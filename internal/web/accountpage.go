package web

// The account surface: your passkeys, and — if you own the project — everybody
// else's roles.
//
// **It carries no script**, which took a decision rather than luck. Adding a
// passkey is a WebAuthn ceremony and ceremonies need the browser API, so the
// button here is a link to `/account/passkey`, which is the auth family's own
// page in a third mode. The scripted surfaces stay at three — the session view,
// the composer, and sign-in — rather than becoming four because a management
// page wanted one button.
//
// **Two authorisations, not one.** Anyone signed in may manage their own
// passkeys; only an owner of the project may invite, change a role or disable
// somebody. The guard cannot express that — it knows one project and one
// verb-shaped question — so the guard exempts this subtree from its write check
// and every handler here asks for itself. That exemption is the kind of thing
// that rots, so it is one line in the guard, named, and every handler below
// begins by saying who it is for.

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/account"
)

// Accounts serves the account surface.
type Accounts struct {
	Store *account.Store
	Auth  *Auth
	// Project is the project whose roles this install manages.
	Project string
	Now     func() time.Time
}

func (a *Accounts) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// Routes registers the surface.
func (a *Accounts) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /account", a.show)
	mux.HandleFunc("POST /account/invite", a.invite)
	mux.HandleFunc("POST /account/role", a.role)
	mux.HandleFunc("POST /account/disable", a.disable)
	mux.HandleFunc("POST /account/passkey/remove", a.removePasskey)
}

type passkeyRow struct {
	ID       string
	Label    string
	Added    string
	LastUsed string
	Current  bool
}

type personRow struct {
	Email    string
	ID       string
	Roles    string
	Passkeys int
	Disabled bool
	Self     bool
}

type accountPage struct {
	Email    string
	Roles    []account.Grant
	Passkeys []passkeyRow
	// Owner is whether this account owns the project, which is what unlocks
	// everything below the fold.
	Owner   bool
	People  []personRow
	Project string
	// Invited is a link shown exactly once, because the secret is never stored
	// and cannot be shown again.
	Invited string
	Error   string
	Said    string
	// LastOwner marks that this account is the only owner left, which is why
	// some things below are refused.
	LastOwner bool
}

// who resolves the signed-in account, or sends them to sign in.
func (a *Accounts) who(w http.ResponseWriter, r *http.Request) (account.Account, bool) {
	acct, ok := a.Auth.Whoever(r.Context(), r)
	if !ok {
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return account.Account{}, false
	}
	return acct, true
}

// owns reports whether this account may manage other people.
func (a *Accounts) owns(ctx context.Context, acct account.Account) bool {
	role, ok := a.Store.RoleFor(ctx, acct.ID, a.Project)
	return ok && role == account.Owner
}

// lastOwner reports whether acct is the only owner the project has left.
//
// The lockout the owner named when they chose passkeys, in its other form: not
// a lost device but a role removed from the only person holding it. Nothing
// below is allowed to produce a project nobody can administer.
func (a *Accounts) lastOwner(ctx context.Context, acct account.Account) bool {
	people, err := a.Store.Accounts(ctx)
	if err != nil {
		// Refusing is the safe answer to not knowing.
		return true
	}
	owners := 0
	mine := false
	for _, p := range people {
		if p.Disabled {
			continue
		}
		if role, ok := a.Store.RoleFor(ctx, p.ID, a.Project); ok && role == account.Owner {
			owners++
			if p.ID == acct.ID {
				mine = true
			}
		}
	}
	return mine && owners <= 1
}

func (a *Accounts) show(w http.ResponseWriter, r *http.Request) {
	acct, ok := a.who(w, r)
	if !ok {
		return
	}
	a.render(w, r, acct, accountPage{
		Invited: r.URL.Query().Get("invited"),
		Said:    r.URL.Query().Get("said"),
		Error:   r.URL.Query().Get("error"),
	})
}

// render fills in everything the page shows about whoever is asking.
func (a *Accounts) render(w http.ResponseWriter, r *http.Request, acct account.Account, p accountPage) {
	ctx := r.Context()
	p.Email = acct.Email
	p.Project = a.Project
	p.Owner = a.owns(ctx, acct)
	p.LastOwner = a.lastOwner(ctx, acct)

	if grants, err := a.Store.Grants(ctx, acct.ID); err == nil {
		p.Roles = grants
	}
	// Which passkey this browser is using cannot be known — a session cookie
	// does not remember which credential started it — so none is marked
	// current. Saying "this one" wrongly would be worse than saying nothing.
	if creds, err := a.Store.Credentials(ctx, acct.ID); err == nil {
		for _, c := range creds {
			row := passkeyRow{
				ID:    url.QueryEscape(string(c.ID)),
				Label: c.Label,
				Added: c.Created.Format("2006-01-02"),
			}
			if !c.LastUsed.IsZero() {
				row.LastUsed = c.LastUsed.Format("2006-01-02")
			} else {
				row.LastUsed = "not since it was added"
			}
			p.Passkeys = append(p.Passkeys, row)
		}
	}

	if p.Owner {
		people, err := a.Store.Accounts(ctx)
		if err == nil {
			for _, person := range people {
				grants, _ := a.Store.Grants(ctx, person.ID)
				var roles []string
				for _, g := range grants {
					roles = append(roles, g.Project+": "+string(g.Role))
				}
				creds, _ := a.Store.Credentials(ctx, person.ID)
				p.People = append(p.People, personRow{
					Email: person.Email, ID: person.ID,
					Roles: strings.Join(roles, ", "), Passkeys: len(creds),
					Disabled: person.Disabled, Self: person.ID == acct.ID,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := accountTmpl.Execute(w, p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *Accounts) back(w http.ResponseWriter, r *http.Request, said, errMsg, invited string) {
	q := url.Values{}
	if said != "" {
		q.Set("said", said)
	}
	if errMsg != "" {
		q.Set("error", errMsg)
	}
	if invited != "" {
		q.Set("invited", invited)
	}
	http.Redirect(w, r, "/account?"+q.Encode(), http.StatusSeeOther)
}

// invite — owners only.
func (a *Accounts) invite(w http.ResponseWriter, r *http.Request) {
	acct, ok := a.who(w, r)
	if !ok {
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin post refused", http.StatusForbidden)
		return
	}
	if !a.owns(r.Context(), acct) {
		http.Error(w, "only an owner of this project can invite", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		a.back(w, r, "", "that form did not arrive intact", "")
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	role := account.Role(r.FormValue("role"))
	project := strings.TrimSpace(r.FormValue("project"))
	if project == "" {
		project = a.Project
	}
	secret, err := a.Store.Invite(r.Context(), email, project, role, acct.Email)
	if err != nil {
		a.back(w, r, "", err.Error(), "")
		return
	}
	// Shown once. The secret was never stored, so there is nothing to come back
	// for — which the page says, because somebody who closes the tab will
	// otherwise go looking.
	a.back(w, r, "", "", "/invite/"+secret)
}

// role — owners only, and never the last one demoting themselves.
func (a *Accounts) role(w http.ResponseWriter, r *http.Request) {
	acct, ok := a.who(w, r)
	if !ok {
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin post refused", http.StatusForbidden)
		return
	}
	ctx := r.Context()
	if !a.owns(ctx, acct) {
		http.Error(w, "only an owner of this project can change a role", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		a.back(w, r, "", "that form did not arrive intact", "")
		return
	}
	target := r.FormValue("id")
	role := account.Role(r.FormValue("role"))
	if !role.Valid() {
		a.back(w, r, "", "that is not a role", "")
		return
	}
	if target == acct.ID && role != account.Owner && a.lastOwner(ctx, acct) {
		a.back(w, r, "", "you are the only owner left; make somebody else an owner first", "")
		return
	}
	if err := a.Store.Grant(ctx, target, a.Project, role, acct.Email); err != nil {
		a.back(w, r, "", err.Error(), "")
		return
	}
	a.back(w, r, "role changed", "", "")
}

// disable — owners only, and never the last owner.
func (a *Accounts) disable(w http.ResponseWriter, r *http.Request) {
	acct, ok := a.who(w, r)
	if !ok {
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin post refused", http.StatusForbidden)
		return
	}
	ctx := r.Context()
	if !a.owns(ctx, acct) {
		http.Error(w, "only an owner of this project can disable an account", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		a.back(w, r, "", "that form did not arrive intact", "")
		return
	}
	target := r.FormValue("id")
	if target == acct.ID && a.lastOwner(ctx, acct) {
		a.back(w, r, "", "you are the only owner left; that would leave nobody able to administer this", "")
		return
	}
	if err := a.Store.Disable(ctx, target, r.FormValue("undo") == "1"); err != nil {
		a.back(w, r, "", err.Error(), "")
		return
	}
	a.back(w, r, "account updated", "", "")
}

// removePasskey — your own, and never your last one.
func (a *Accounts) removePasskey(w http.ResponseWriter, r *http.Request) {
	acct, ok := a.who(w, r)
	if !ok {
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin post refused", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		a.back(w, r, "", "that form did not arrive intact", "")
		return
	}
	raw, err := url.QueryUnescape(r.FormValue("id"))
	if err != nil {
		a.back(w, r, "", "that passkey is not one of yours", "")
		return
	}
	// Ownership is checked by the store, not here: removing somebody else's
	// passkey is the same request with a different id, and a check the caller
	// can skip is not a check.
	if err := a.Store.RemoveCredential(r.Context(), acct.ID, []byte(raw)); err != nil {
		if errors.Is(err, account.ErrLastPasskey) {
			a.back(w, r, "", "that is your only passkey; add another first or you will be locked out", "")
			return
		}
		a.back(w, r, "", "that passkey is not one of yours", "")
		return
	}
	a.back(w, r, "passkey removed", "", "")
}

var accountTmpl = template.Must(template.New("account").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mustur — account</title>
<style>
  :root { color-scheme: light dark; --edge: #8884; --accent: #6a8fd8;
          --accent-soft: #6a8fd820; }
  body { font: 17px/1.5 system-ui, sans-serif; margin: 0; padding: 0 0 1rem;
         max-width: 40rem; margin-inline: auto; }
  header { display: flex; align-items: baseline; gap: .6rem; padding: .75rem 1rem;
           border-bottom: 1.4px solid var(--edge); }
  header .who { margin-left: auto; opacity: .65; font-size: .82em; }
  h1 { font-size: 1rem; font-weight: 600; margin: 0; }
  h2 { font-size: .9rem; font-weight: 600; margin: 1.4rem 1rem .4rem; opacity: .75; }
  p, ul { margin: .4rem 1rem; }
  ul { list-style: none; padding: 0; }
  li { display: flex; align-items: baseline; gap: .6rem; padding: .45rem 0;
       border-bottom: 1px solid var(--edge); font-size: .92em; }
  li .grow { flex: 1; }
  li small { opacity: .6; }
  .said { margin: .8rem 1rem; padding: .6rem .8rem;
          border-left: 3px solid var(--edge); font-size: .9em; }
  .said code { word-break: break-all; }
  form.inline { display: inline; }
  button, .btn { font: inherit; font-size: .85em; padding: .3rem .7rem;
           border: 1px solid var(--edge); border-radius: .5rem;
           background: transparent; color: inherit; text-decoration: none;
           transition: background-color .12s ease; }
  button:hover, .btn:hover { background: #8881; }
  button:active { background: #8883; transition-duration: 0s; }
  button:focus-visible, .btn:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .primary { border-color: var(--accent); background: var(--accent-soft); }
  fieldset { margin: .4rem 1rem; border: 1px solid var(--edge); border-radius: .5rem;
             display: flex; gap: .5rem; flex-wrap: wrap; align-items: center; }
  input, select { font: inherit; font-size: .9em; padding: .35rem;
                  border: 1px solid var(--edge); border-radius: .4rem;
                  background: transparent; color: inherit; }
  nav { display: flex; border-top: 1.4px solid var(--edge); margin-top: 1.5rem; }
  nav a { flex: 1; padding: .7rem .25rem; text-align: center; font-size: .85em;
          text-decoration: none; color: inherit; opacity: .6; }
</style>
</head>
<body>
<header><h1>Account</h1><span class="who">{{.Email}}</span></header>

{{if .Said}}<p class="said">{{.Said}}</p>{{end}}
{{if .Error}}<p class="said">{{.Error}}</p>{{end}}
{{if .Invited}}<p class="said">Invitation created. <strong>This link is shown once and is not stored</strong> — copy it now.<br>
<code>{{.Invited}}</code></p>{{end}}

<h2>What you may do</h2>
<ul>{{range .Roles}}<li><span class="grow">{{.Project}}</span><small>{{.Role}}</small></li>{{else}}
<li><span class="grow">no roles on any project</span></li>{{end}}</ul>
{{if .LastOwner}}<p class="said">You are the only owner of {{.Project}}. Some things below are refused, so that this cannot end up with nobody able to administer it.</p>{{end}}

<h2>Passkeys</h2>
<ul>{{range .Passkeys}}<li>
  <span class="grow">{{.Label}}</span>
  <small>added {{.Added}} · used {{.LastUsed}}</small>
  <form class="inline" method="post" action="/account/passkey/remove">
    <input type="hidden" name="id" value="{{.ID}}">
    <button type="submit">Remove</button>
  </form>
</li>{{else}}<li><span class="grow">none, which should not be possible while you are signed in</span></li>{{end}}</ul>
<p><a class="btn primary" href="/account/passkey">Add a passkey</a>
<small>— a second device is how you get back in if you lose this one.</small></p>

{{if .Owner}}
<h2>Invite somebody</h2>
<form method="post" action="/account/invite">
  <fieldset>
    <input type="email" name="email" placeholder="them@example.com" required>
    <select name="role"><option value="reader">reader</option><option value="owner">owner</option></select>
    <input type="text" name="project" value="{{.Project}}" size="6">
    <button type="submit" class="primary">Invite</button>
  </fieldset>
</form>

<h2>People</h2>
<ul>{{range .People}}<li>
  <span class="grow">{{.Email}}{{if .Self}} <small>(you)</small>{{end}}{{if .Disabled}} <small>disabled</small>{{end}}</span>
  <small>{{.Roles}} · {{.Passkeys}} passkey(s)</small>
  <form class="inline" method="post" action="/account/role">
    <input type="hidden" name="id" value="{{.ID}}">
    <select name="role"><option value="reader">reader</option><option value="owner">owner</option></select>
    <button type="submit">Set</button>
  </form>
  <form class="inline" method="post" action="/account/disable">
    <input type="hidden" name="id" value="{{.ID}}">
    {{if .Disabled}}<input type="hidden" name="undo" value="1">{{end}}
    <button type="submit">{{if .Disabled}}Enable{{else}}Disable{{end}}</button>
  </form>
</li>{{end}}</ul>
{{end}}

<h2>This browser</h2>
<form method="post" action="/signout"><button type="submit">Sign out</button></form>

<nav>
  <a href="/records">Records</a>
  <a href="/questions">Decisions</a>
  <a href="/intake">Intake</a>
</nav>
</body>
</html>
`))
