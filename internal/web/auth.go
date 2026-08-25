package web

// Sign in, and the surface that creates an account from an invitation.
//
// **These pages carry script, and unlike the other surfaces they have no choice
// at all.** WebAuthn is a browser API: `navigator.credentials.create` and
// `.get` cannot be reached from a server-rendered form, so choosing passkeys
// (MUS-D-0104) chose a client layer here.
//
// Adding a passkey used to have a page of its own for that reason, keeping the
// account page script-free. A review called it what it was — a page holding a
// heading and one button — so the ceremony opens on the account page instead
// and that page carries script too (MUS-Q-0047). Four surfaces now, counted
// rather than absorbed.
//
// What the script does is the ceremony and nothing else: ask the browser to
// make or use a passkey, and post what it returns. No state, no rendering, no
// framework.
//
// The verification is not this repository's own. Parsing CBOR attestation,
// checking a relying-party hash and verifying an ES256 signature is exactly the
// code that looks right and is not (MUS-D-0105).
//
// The wiring around it is this repository's, and `auth_test.go` drives it with
// a virtual authenticator: invited, registered, signed out, recognised, plus a
// spent challenge, a passkey signed for another site, and a device replaced
// after a loss. What that does not test is a real authenticator, which is the
// owner's to try.

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/store"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

//go:embed assets/auth.js
var authJS string

// SessionCookie is the name of the cookie that says who is signed in.
const SessionCookie = "mustur_session"

// ceremonyCookie points at the half-finished ceremony, and lives only as long
// as one.
const ceremonyCookie = "mustur_ceremony"

// Auth serves sign-in, and registration from an invitation.
type Auth struct {
	Accounts *account.Store
	// Origin is the site as a browser sees it — "https://mustur.devofpie.com".
	// WebAuthn binds every credential to it, which is what makes a passkey
	// unphishable: a look-alike site cannot ask for one.
	Origin string
	// Secure marks the session cookie secure. Off only for a plain-HTTP local
	// server, which is the one place a browser will do WebAuthn without TLS.
	Secure bool
	// Records resolves a project's prefix to its name, so an invitation names
	// the project rather than showing a tag the invited person has never seen.
	// Nil means the tag is shown alone.
	Records *store.Store
	Now     func() time.Time
}

func (a *Auth) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// relying builds the WebAuthn configuration from the origin.
//
// The RP ID is the origin's hostname with no port and no scheme, which is what
// a credential is scoped to. Getting it wrong does not fail loudly: it makes
// every existing passkey silently unusable, because the browser will not offer
// a credential registered under a different id.
func (a *Auth) relying() (*webauthn.WebAuthn, error) {
	origin := strings.TrimRight(strings.TrimSpace(a.Origin), "/")
	if origin == "" {
		return nil, errors.New("no origin configured, so no passkey can be scoped to this site")
	}
	host := origin
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	return webauthn.New(&webauthn.Config{
		RPID:          host,
		RPDisplayName: "Mustur",
		RPOrigins:     []string{origin},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			// Discoverable, required rather than preferred.
			//
			// Sign-in here asks the browser for any passkey scoped to this site
			// and lets the person choose from what it offers. An authenticator
			// that made a non-discoverable credential would have made one this
			// server can never ask for by name — the account would exist, hold a
			// passkey, and be unreachable. Left unset, that is the authenticator's
			// choice to make.
			ResidentKey: protocol.ResidentKeyRequirementRequired,
			// Preferred rather than required: the passkey is the credential, and
			// refusing an authenticator that cannot do a second factor would
			// refuse hardware the owner may be holding.
			UserVerification: protocol.VerificationPreferred,
		},
	})
}

// A person is what the library needs to know about whoever is registering or
// signing in.
type person struct {
	id    []byte
	email string
	creds []webauthn.Credential
}

func (p *person) WebAuthnID() []byte                         { return p.id }
func (p *person) WebAuthnName() string                       { return p.email }
func (p *person) WebAuthnDisplayName() string                { return p.email }
func (p *person) WebAuthnCredentials() []webauthn.Credential { return p.creds }

// Routes registers the surface.
func (a *Auth) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /signin", a.signinPage)
	mux.HandleFunc("POST /signin/begin", a.signinBegin)
	mux.HandleFunc("POST /signin/finish", a.signinFinish)
	mux.HandleFunc("POST /signout", a.signout)
	mux.HandleFunc("POST /account/passkey/begin", a.addBegin)
	mux.HandleFunc("POST /account/passkey/finish", a.addFinish)
	mux.HandleFunc("GET /invite/{token}", a.invitePage)
	mux.HandleFunc("POST /invite/{token}/begin", a.registerBegin)
	mux.HandleFunc("POST /invite/{token}/finish", a.registerFinish)
	mux.HandleFunc("GET /assets/auth.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(authJS))
	})
}

type authPage struct {
	Invite  bool
	Email   string
	Project string
	Role    string
	Error   string
	Empty   bool
	// ProjectName is the project written out with its tag — "Mustur (MUS)" —
	// because an invited reader has never seen the tag, and the tag is what
	// every identifier in the records uses.
	ProjectName string
}

func (a *Auth) signinPage(w http.ResponseWriter, r *http.Request) {
	empty, _ := a.Accounts.Empty(r.Context())
	a.render(w, authPage{Empty: empty})
}

func (a *Auth) invitePage(w http.ResponseWriter, r *http.Request) {
	inv, err := a.Accounts.Invitation(r.Context(), r.PathValue("token"))
	if err != nil {
		// Says that it cannot be used and not why. A page that distinguished
		// expired from unknown would tell somebody guessing tokens when they
		// were close.
		a.render(w, authPage{Error: "That invitation cannot be used. Ask for another."})
		return
	}
	a.render(w, authPage{
		Invite: true, Email: inv.Email, Project: inv.Project, Role: string(inv.Role),
		ProjectName: projectName(r.Context(), a.Records, inv.Project),
	})
}

// addBegin issues a challenge for a passkey on an account that already exists.
//
// Unlike registration from an invitation there is nothing to redeem and no
// handle to invent: the account is known, so the ceremony commits to its
// identifier and the new passkey lands beside the others.
func (a *Auth) addBegin(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	acct, ok := a.Whoever(r.Context(), r)
	if !ok {
		http.Error(w, "not signed in", http.StatusForbidden)
		return
	}
	rp, err := a.relying()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	who := &person{id: []byte(acct.ID), email: acct.Email}
	creds, err := a.Accounts.Credentials(r.Context(), acct.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Excluded, so a device that already holds one for this account offers to
	// replace rather than silently making a duplicate.
	for _, c := range creds {
		who.creds = append(who.creds, webauthn.Credential{ID: c.ID, PublicKey: c.PublicKey})
	}
	creation, session, err := rp.BeginRegistration(who)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.Marshal(session)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id, err := a.Accounts.BeginCeremony(r.Context(), account.Ceremony{
		Purpose: "register", Data: data, Handle: []byte(acct.ID),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.setCeremony(w, id)
	writeJSON(w, creation)
}

func (a *Auth) addFinish(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	acct, ok := a.Whoever(r.Context(), r)
	if !ok {
		http.Error(w, "not signed in", http.StatusForbidden)
		return
	}
	c, err := a.take(r, "register")
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	// A ceremony begun for somebody else cannot be finished as this account:
	// the handle it committed to is the account it was for.
	if string(c.Handle) != acct.ID || c.Secret != "" {
		http.Error(w, "that attempt does not belong to this account", http.StatusForbidden)
		return
	}
	rp, err := a.relying()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(c.Data, &session); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cred, err := rp.FinishRegistration(&person{id: []byte(acct.ID), email: acct.Email}, session, r)
	if err != nil {
		http.Error(w, "that passkey could not be verified", http.StatusForbidden)
		return
	}
	if err := a.Accounts.AddCredential(r.Context(), acct.ID, account.Credential{
		ID:        cred.ID,
		PublicKey: cred.PublicKey,
		SignCount: cred.Authenticator.SignCount,
		Label:     labelFor(r),
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"ok": "1", "to": "/account"})
}

// registerBegin issues the challenge the authenticator will sign.
func (a *Auth) registerBegin(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	secret := r.PathValue("token")
	inv, err := a.Accounts.Invitation(r.Context(), secret)
	if err != nil {
		http.Error(w, "that invitation cannot be used", http.StatusForbidden)
		return
	}
	rp, err := a.relying()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// The user handle is committed to now, before the account exists, because
	// the browser is told who it is registering for at the start. An address
	// that already has an account reuses its identifier — that is what makes a
	// replacement passkey land on the account it is replacing a key for, rather
	// than on a second account with the same email.
	who := &person{email: inv.Email}
	if existing, ok := a.Accounts.AccountByEmail(r.Context(), inv.Email); ok {
		who.id = []byte(existing.ID)
		creds, err := a.Accounts.Credentials(r.Context(), existing.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, c := range creds {
			who.creds = append(who.creds, webauthn.Credential{ID: c.ID, PublicKey: c.PublicKey})
		}
	} else {
		id, err := newHandle()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		who.id = id
	}

	creation, session, err := rp.BeginRegistration(who)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.Marshal(session)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id, err := a.Accounts.BeginCeremony(r.Context(), account.Ceremony{
		Purpose: "register", Data: data, Handle: who.id, Secret: secret,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.setCeremony(w, id)
	writeJSON(w, creation)
}

// registerFinish verifies what the authenticator signed, spends the invitation
// and signs the person in.
func (a *Auth) registerFinish(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	c, err := a.take(r, "register")
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	rp, err := a.relying()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(c.Data, &session); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	inv, err := a.Accounts.Invitation(r.Context(), c.Secret)
	if err != nil {
		http.Error(w, "that invitation cannot be used", http.StatusForbidden)
		return
	}

	who := &person{id: c.Handle, email: inv.Email}
	cred, err := rp.FinishRegistration(who, session, r)
	if err != nil {
		// The library's message names the check that failed, which is useful in
		// a log and not to a browser.
		http.Error(w, "that passkey could not be verified", http.StatusForbidden)
		return
	}

	// Only now is the invitation spent: an authenticator that refused, or a
	// person who changed their mind, leaves the invitation usable.
	acct, _, err := a.Accounts.Redeem(r.Context(), c.Secret, string(c.Handle))
	if err != nil {
		http.Error(w, "that invitation cannot be used", http.StatusForbidden)
		return
	}
	if err := a.Accounts.AddCredential(r.Context(), acct.ID, account.Credential{
		ID:        cred.ID,
		PublicKey: cred.PublicKey,
		SignCount: cred.Authenticator.SignCount,
		Label:     labelFor(r),
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.signIn(w, r, acct.ID)
	writeJSON(w, map[string]string{"ok": "1", "to": "/records"})
}

func (a *Auth) signinBegin(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	rp, err := a.relying()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Discoverable: the browser offers whichever passkey is scoped to this
	// site, so nothing is typed and no username is disclosed to a page that has
	// not authenticated anybody yet.
	assertion, session, err := rp.BeginDiscoverableLogin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.Marshal(session)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id, err := a.Accounts.BeginCeremony(r.Context(), account.Ceremony{Purpose: "signin", Data: data})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.setCeremony(w, id)
	writeJSON(w, assertion)
}

func (a *Auth) signinFinish(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	c, err := a.take(r, "signin")
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	rp, err := a.relying()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(c.Data, &session); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	parsed, err := protocol.ParseCredentialRequestResponse(r)
	if err != nil {
		http.Error(w, "that passkey was not recognised", http.StatusForbidden)
		return
	}

	ctx := r.Context()
	var signedIn account.Account
	// The credential identifies the account. The user handle the browser sends
	// is not trusted for that: it is whatever was stored at registration, and
	// the credential id is the thing this server issued and can look up.
	cred, err := rp.ValidateDiscoverableLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		acct, stored, err := a.Accounts.ByCredential(ctx, rawID)
		if err != nil {
			return nil, err
		}
		signedIn = acct
		return &person{
			id:    userHandle,
			email: acct.Email,
			creds: []webauthn.Credential{{
				ID:        stored.ID,
				PublicKey: stored.PublicKey,
				Authenticator: webauthn.Authenticator{
					SignCount: stored.SignCount,
				},
			}},
		}, nil
	}, session, parsed)
	if err != nil || signedIn.ID == "" {
		http.Error(w, "that passkey was not recognised", http.StatusForbidden)
		return
	}

	// The signature counter is the one anti-cloning signal WebAuthn offers, and
	// many authenticators never move it. Recorded where it means something and
	// not treated as an attack when it stays at zero.
	_ = a.Accounts.UsedCredential(ctx, cred.ID, cred.Authenticator.SignCount)
	a.signIn(w, r, signedIn.ID)
	writeJSON(w, map[string]string{"ok": "1", "to": "/records"})
}

func (a *Auth) signout(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	if c, err := r.Cookie(SessionCookie); err == nil {
		_ = a.Accounts.EndSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/signin", http.StatusSeeOther)
}

func (a *Auth) signIn(w http.ResponseWriter, r *http.Request, accountID string) {
	secret, expires, err := a.Accounts.StartSession(r.Context(), accountID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    secret,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true, // Never readable by script: it is not the script's.
		Secure:   a.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *Auth) setCeremony(w http.ResponseWriter, id string) {
	http.SetCookie(w, &http.Cookie{
		Name: ceremonyCookie, Value: id, Path: "/",
		MaxAge:   int(account.CeremonyLife.Seconds()),
		HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteLaxMode,
	})
}

func (a *Auth) take(r *http.Request, purpose string) (account.Ceremony, error) {
	c, err := r.Cookie(ceremonyCookie)
	if err != nil {
		return account.Ceremony{}, account.ErrNoCeremony
	}
	return a.Accounts.TakeCeremony(r.Context(), c.Value, purpose)
}

// labelFor names a passkey so a list of them is readable. The user agent is a
// weak guess and it is the only thing available; the label is cosmetic and
// nothing depends on it.
func labelFor(r *http.Request) string {
	ua := r.UserAgent()
	switch {
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"):
		return "iPhone or iPad"
	case strings.Contains(ua, "Android"):
		return "Android"
	case strings.Contains(ua, "Macintosh"):
		return "Mac"
	case strings.Contains(ua, "Windows"):
		return "Windows"
	case ua == "":
		return "passkey"
	default:
		return "passkey"
	}
}

func newHandle() ([]byte, error) {
	// 32 bytes, base64 to a printable identifier, because it doubles as the
	// account's primary key when the account is created.
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return []byte(base64.RawURLEncoding.EncodeToString(b)), nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Whoever returns the account a request is signed in as.
func (a *Auth) Whoever(ctx context.Context, r *http.Request) (account.Account, bool) {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return account.Account{}, false
	}
	acct, err := a.Accounts.Session(ctx, c.Value)
	if err != nil {
		return account.Account{}, false
	}
	return acct, true
}

func (a *Auth) render(w http.ResponseWriter, p authPage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := authTmpl.Execute(w, p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var authTmpl = template.Must(template.New("auth").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mustur — {{if .Invite}}accept an invitation{{else}}sign in{{end}}</title>
<style>
  :root { color-scheme: light dark; --edge: #8884; --accent: #6a8fd8;
          --accent-soft: #6a8fd820; }
  body { font: 17px/1.5 system-ui, sans-serif; margin: 0; padding: 1rem;
         max-width: 26rem; margin-inline: auto; min-height: 100vh;
         display: flex; flex-direction: column; justify-content: center; }
  h1 { font-size: 1.1rem; font-weight: 600; margin: 0 0 .3rem; }
  p { margin: .3rem 0 1rem; opacity: .75; font-size: .92em; }
  button { font: inherit; width: 100%; padding: .8rem 1rem;
           border: 1px solid var(--accent); border-radius: .5rem;
           background: var(--accent-soft); color: inherit;
           transition: background-color .12s ease; }
  button:hover { background: #6a8fd833; }
  button:active { background: #6a8fd855; transition-duration: 0s; }
  button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .said { margin: .9rem 0 0; padding: .6rem .8rem;
          border-left: 3px solid var(--edge); font-size: .9em; }
  .who { font-weight: 600; }
  .foot { margin-top: 1.2rem; font-size: .85em; opacity: .6; }
</style>
</head>
<body data-invite="{{if .Invite}}1{{end}}">
{{if .Error}}
<h1>Not usable</h1>
<p>{{.Error}}</p>
{{else if .Invite}}
<h1>Accept this invitation</h1>
<p><span class="who">{{.Email}}</span> — {{.Role}} on {{.ProjectName}}.</p>
<button id="go">Create a passkey</button>
{{else}}
<h1>Sign in</h1>
{{if .Empty}}
<p>Mustur has no accounts yet. The first one is made on the machine:
<code>mustur account invite --email you@example.com --role owner</code></p>
{{else}}
<button id="go">Sign in</button>
<p class="foot">No account here? An invitation comes from somebody who already
has one.</p>
{{end}}
{{end}}
<p class="said" id="said" hidden></p>
<script src="/assets/auth.js"></script>
</body>
</html>
`))
