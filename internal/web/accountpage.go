package web

// The account surface: your passkeys, and — if you own the project — everybody
// else's roles.
//
// **Two screens, not one** (MUS-Q-0045). This one is yours: your roles and your
// passkeys, and nothing about anybody else — which a reader sees in full,
// because there is nothing on it they should not (MUS-Q-0046). People and
// invitations is the other, owner-only, and **linked from here**, which the
// first drawing forgot: it had been drawn, described and made unreachable.
//
// **It carries script**, which it did not. Adding a passkey had a page of its
// own so this one could stay server-rendered; a review called that page what it
// was — a heading and one button — so the ceremony opens here instead
// (MUS-Q-0047). The count of scripted surfaces is four, written down rather
// than absorbed. Everything else on this page works without.
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
	_ "embed"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/intake"
	"github.com/DevOfPie/Mustur/internal/store"
)

//go:embed assets/account.js
var accountJS string

// projectName writes a project the way somebody who has never seen this
// repository would need it: "Mustur (MUS)", in full and with the tag.
//
// A review found the surfaces showing a bare "MUS" to people being invited into
// them. The tag is what every identifier uses, so it has to be visible; it is
// just not a name. Where the store cannot say what the tag belongs to, the tag
// is returned on its own rather than a guess.
func projectName(ctx context.Context, s *store.Store, prefix string) string {
	if s == nil || prefix == "" {
		return prefix
	}
	projects, err := s.List(ctx, "project")
	if err != nil {
		return prefix
	}
	// A routing record that names its own prefix wins: that is the mapping
	// stated rather than inferred.
	for _, p := range projects {
		if v, ok := p.Get(intake.PrefixField); ok && strings.EqualFold(strings.TrimSpace(v), prefix) {
			return p.Title + " (" + prefix + ")"
		}
	}
	// Otherwise the project written under this prefix and claiming no other.
	for _, p := range projects {
		if _, named := p.Get(intake.PrefixField); named {
			continue
		}
		if id, err := ident.Parse(p.ID); err == nil && id.Project == prefix {
			return p.Title + " (" + prefix + ")"
		}
	}
	return prefix
}

// Accounts serves the account surface.
type Accounts struct {
	Store *account.Store
	Auth  *Auth
	// Records resolves a project's prefix to its name. Nil means the tag is
	// shown alone, which is what an invited reader should not have to read.
	Records *store.Store
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
	mux.HandleFunc("POST /account/passkey/remove", a.removePasskey)
	// Owners only, and its own screen: an owner was scrolling past their own
	// two passkeys to reach a list of people, and the rows overlapped on a
	// phone when they shared a screen.
	mux.HandleFunc("GET /account/people", a.people)
	mux.HandleFunc("POST /account/invite", a.invite)
	mux.HandleFunc("POST /account/role", a.role)
	mux.HandleFunc("POST /account/disable", a.disable)
	mux.HandleFunc("GET /assets/account.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(accountJS))
	})
}

type passkeyRow struct {
	ID       string
	Label    string
	Added    string
	LastUsed string
	Current  bool
}

type personRow struct {
	Email string
	ID    string
	// Role in this project. A row on this screen is about this project, so the
	// list of every project somebody has a role in belonged on their own page
	// rather than here.
	Role     string
	Passkeys int
	Disabled bool
	Self     bool
}

type accountPage struct {
	Email    string
	Roles    []roleRow
	Passkeys []passkeyRow
	// Owner is whether this account owns the project, which is what puts the
	// People row on this page and lets the other screen be reached at all.
	Owner bool
	// PeopleScreen renders the second screen rather than the first.
	PeopleScreen bool
	People       []personRow
	Project      string
	ProjectName  string
	// Invited is a link shown exactly once, whole, because the secret is never
	// stored and cannot be shown again.
	Invited string
	Error   string
	Said    string
	// LastOwner marks that this account is the only owner left. Nothing on the
	// page announces it any more — the controls refuse and say why at the
	// moment they refuse (MUS-Q-0048) — but the handlers still need to know.
	LastOwner bool
}

// A roleRow is one project an account may do something in, written out with its
// tag because an invited reader has never seen the tag.
type roleRow struct {
	Project string
	Role    string
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

// people is the owner-only second screen.
func (a *Accounts) people(w http.ResponseWriter, r *http.Request) {
	acct, ok := a.who(w, r)
	if !ok {
		return
	}
	if !a.owns(r.Context(), acct) {
		// A reader has no such screen. Refused rather than rendered empty: an
		// empty list would suggest there is nobody rather than that this is not
		// theirs to see.
		http.Error(w, "only an owner of this project can see who has access", http.StatusForbidden)
		return
	}
	a.render(w, r, acct, accountPage{
		PeopleScreen: true,
		Invited:      r.URL.Query().Get("invited"),
		Said:         r.URL.Query().Get("said"),
		Error:        r.URL.Query().Get("error"),
	})
}

// render fills in everything the page shows about whoever is asking.
func (a *Accounts) render(w http.ResponseWriter, r *http.Request, acct account.Account, p accountPage) {
	ctx := r.Context()
	p.Email = acct.Email
	p.Project = a.Project
	p.ProjectName = projectName(ctx, a.Records, a.Project)
	p.Owner = a.owns(ctx, acct)
	p.LastOwner = a.lastOwner(ctx, acct)

	if grants, err := a.Store.Grants(ctx, acct.ID); err == nil {
		for _, g := range grants {
			p.Roles = append(p.Roles, roleRow{
				Project: projectName(ctx, a.Records, g.Project),
				Role:    string(g.Role),
			})
		}
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
				role, _ := a.Store.RoleFor(ctx, person.ID, a.Project)
				creds, _ := a.Store.Credentials(ctx, person.ID)
				p.People = append(p.People, personRow{
					Email: person.Email, ID: person.ID,
					Role: string(role), Passkeys: len(creds),
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

// back returns to the screen the action came from, which for anything about
// other people is the people screen.
func (a *Accounts) back(w http.ResponseWriter, r *http.Request, said, errMsg, invited string) {
	to := "/account"
	if strings.HasPrefix(r.URL.Path, "/account/invite") ||
		strings.HasPrefix(r.URL.Path, "/account/role") ||
		strings.HasPrefix(r.URL.Path, "/account/disable") {
		to = "/account/people"
	}
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
	http.Redirect(w, r, to+"?"+q.Encode(), http.StatusSeeOther)
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
	// Shown once, and whole. The secret was never stored, so a truncated link
	// is a secret destroyed and the only recovery is issuing another.
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
<title>Mustur — {{if .PeopleScreen}}people{{else}}account{{end}}</title>
<style>
  :root { color-scheme: light dark; --edge: #8884; --accent: #6a8fd8;
          --accent-soft: #6a8fd820; }
  body { font: 17px/1.5 system-ui, sans-serif; margin: 0; padding: 0 0 1rem;
         max-width: 40rem; margin-inline: auto; }
  header { display: flex; align-items: baseline; gap: .6rem; padding: .75rem 1rem;
           border-bottom: 1.4px solid var(--edge); }
  header a { color: inherit; text-decoration: none; opacity: .65; }
  header .who { margin-left: auto; opacity: .65; font-size: .82em; }
  h1 { font-size: 1rem; font-weight: 600; margin: 0; }
  h2 { font-size: .78rem; font-weight: 600; margin: 1.3rem 1rem .3rem;
       opacity: .55; text-transform: uppercase; letter-spacing: .04em; }
  ul { list-style: none; padding: 0; margin: .2rem 1rem; }
  li { padding: .5rem 0; border-bottom: 1px solid var(--edge); font-size: .92em; }
  li.row { display: flex; align-items: baseline; gap: .6rem; }
  li .grow { flex: 1; }
  li small { opacity: .6; }
  /* A person is three stacked lines rather than one crowded row: the row
     wrapped into itself on a phone, which a review saw as overlapping text. */
  li.person { display: flex; flex-direction: column; gap: .35rem; }
  li.person .controls { display: flex; gap: .5rem; align-items: center; }
  p { margin: .4rem 1rem; }
  .said { margin: .8rem 1rem; padding: .6rem .8rem;
          border-left: 3px solid var(--edge); font-size: .9em; }
  /* Whole, and wrapping. A one-time secret shown with an ellipsis is a secret
     destroyed, since it is never stored and cannot be shown again. */
  .link { font-size: .8em; word-break: break-all; line-height: 1.4;
          margin: .4rem 0; }
  form.inline { display: inline; }
  button, .btn { font: inherit; font-size: .85em; padding: .3rem .7rem;
           border: 1px solid var(--edge); border-radius: .5rem;
           background: transparent; color: inherit; text-decoration: none;
           transition: background-color .12s ease; }
  button:hover, .btn:hover { background: #8881; }
  button:active { background: #8883; transition-duration: 0s; }
  button:focus-visible, .btn:focus-visible { outline: 2px solid var(--accent);
           outline-offset: 2px; }
  .primary { border-color: var(--accent); background: var(--accent-soft); }
  fieldset { margin: .2rem 1rem; border: 1px solid var(--edge);
             border-radius: .5rem; display: flex; flex-direction: column;
             gap: .7rem; }
  label { display: flex; flex-direction: column; gap: .2rem; font-size: .85em; }
  label span { opacity: .6; font-size: .88em; }
  input, select { font: inherit; font-size: .92em; padding: .4rem;
                  border: 1px solid var(--edge); border-radius: .4rem;
                  background: transparent; color: inherit; }
  .pair { display: flex; gap: .6rem; }
  .pair label { flex: 1; }
  #ceremony { margin: .6rem 0; padding: .7rem .8rem; border: 1px solid var(--edge);
              border-radius: .5rem; font-size: .9em; }
  nav { display: flex; border-top: 1.4px solid var(--edge); margin-top: 1.5rem; }
  nav a { flex: 1; padding: .7rem .25rem; text-align: center; font-size: .85em;
          text-decoration: none; color: inherit; opacity: .6; }
</style>
</head>
<body>

{{if .PeopleScreen}}
<header><a href="/account" aria-label="Back">←</a><h1>People</h1>
  <span class="who">{{.ProjectName}}</span></header>

{{if .Said}}<p class="said">{{.Said}}</p>{{end}}
{{if .Error}}<p class="said">{{.Error}}</p>{{end}}
{{if .Invited}}<p class="said"><strong>Invitation created.</strong> Shown once and never stored.
<span class="link" id="invite-link">{{.Invited}}</span>
<button type="button" class="btn primary" id="copy" data-link="{{.Invited}}">Copy link</button></p>{{end}}

<h2>Invite somebody</h2>
<form method="post" action="/account/invite">
  <fieldset>
    <label><span>Email address</span>
      <input type="email" name="email" placeholder="them@example.com" required></label>
    <div class="pair">
      <label><span>Role</span>
        <select name="role"><option value="reader">reader</option><option value="owner">owner</option></select></label>
      <label><span>Project</span>
        <select name="project"><option value="{{.Project}}">{{.ProjectName}}</option></select></label>
    </div>
    <button type="submit" class="primary" style="align-self:flex-start">Invite</button>
  </fieldset>
</form>

<h2>People</h2>
<ul>{{range .People}}<li class="person">
  <span>{{.Email}}{{if .Self}} <small>(you)</small>{{end}}{{if .Disabled}} <small>disabled</small>{{end}}</span>
  <small>{{if .Role}}{{.Role}}{{else}}no role here{{end}} · {{.Passkeys}} passkey(s)</small>
  <span class="controls">
    <form class="inline" method="post" action="/account/role">
      <input type="hidden" name="id" value="{{.ID}}">
      <select name="role" data-save><option value="reader"{{if eq .Role "reader"}} selected{{end}}>reader</option><option value="owner"{{if eq .Role "owner"}} selected{{end}}>owner</option></select>
      <noscript><button type="submit">Save</button></noscript>
    </form>
    <form class="inline" method="post" action="/account/disable">
      <input type="hidden" name="id" value="{{.ID}}">
      {{if .Disabled}}<input type="hidden" name="undo" value="1">{{end}}
      <button type="submit">{{if .Disabled}}Enable{{else}}Disable{{end}}</button>
    </form>
  </span>
</li>{{end}}</ul>

{{else}}
<header><h1>Account</h1><span class="who">{{.Email}}</span></header>

{{if .Said}}<p class="said">{{.Said}}</p>{{end}}
{{if .Error}}<p class="said">{{.Error}}</p>{{end}}

<h2>What you may do</h2>
<ul>{{range .Roles}}<li class="row"><span class="grow">{{.Project}}</span><small>{{.Role}}</small></li>{{else}}
<li class="row"><span class="grow">no roles on any project</span></li>{{end}}</ul>

<h2>Passkeys</h2>
<ul>{{range .Passkeys}}<li class="row">
  <span class="grow">{{.Label}}</span>
  <small>added {{.Added}}</small>
  <form class="inline" method="post" action="/account/passkey/remove">
    <input type="hidden" name="id" value="{{.ID}}">
    <button type="submit">Remove</button>
  </form>
</li>{{else}}<li class="row"><span class="grow">none, which should not be possible while you are signed in</span></li>{{end}}</ul>
<p><button type="button" class="primary" id="addkey">Add a passkey</button></p>
<div id="ceremony" hidden></div>

{{if .Owner}}
<h2>This project</h2>
<ul><li class="row">
  <a class="grow" href="/account/people" style="color:inherit">People and invitations</a>
  <small>{{len .People}} people</small><span>›</span>
</li></ul>
{{end}}

<h2>This browser</h2>
<form method="post" action="/signout"><button type="submit">Sign out</button></form>
{{end}}

<nav>
  <a href="/records">Records</a>
  <a href="/questions">Decisions</a>
  <a href="/intake">Intake</a>
</nav>
<script src="/assets/auth.js"></script>
<script src="/assets/account.js"></script>
</body>
</html>
`))
