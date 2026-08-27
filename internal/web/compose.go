package web

// The composer — surface 1, and the reason the phone matters.
//
// Built from its artboard in the seven-surface plan rather than from this
// file's idea of it: a screen of its own, a full-height box, "draft kept" in
// the header, and a destination row above the send button. Milestone 5 first
// built it as a widget inside the session view and amended the brief to say
// that was where it lived. The owner declined that on MUS-Q-0034, for the
// reason they had already given on MUS-Q-0010 — a plan agents route around is
// not a plan.
//
// **Thought first, destination second** (MUS-D-0013). What is being written is
// a thought; where it goes is a separate choice made after it is written. Two
// of that decision's three clauses are here: the box comes before the route
// row, and the route row defaults to the last active session. The third asks
// that the idea inbox be a route like any other — built — **which folds intake
// into the composer** — declined on MUS-Q-0036, so the intake box and the
// session view's reply box both stay and three surfaces can start a message.
//
// **This is the second surface carrying script**, which until now was a rule
// rather than a count. The owner took that deliberately on MUS-Q-0034: a
// composer that loses a draft is not a composer, and a draft cannot survive a
// backgrounded phone without something running in the page. What the script
// does is bounded to that — keep the draft, grow the box, show that it is kept.
// **The form works without it**: the destination is a radio group and Send is a
// submit button, so a browser with no script posts and gets a rendered page
// back. Everything else here is server-rendered.

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/export"
	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/intake"
	"github.com/DevOfPie/Mustur/internal/session"
	"github.com/DevOfPie/Mustur/internal/store"
)

//go:embed assets/compose.js
var composeJS string

// Compose serves the composer.
type Compose struct {
	// ShowAccount renders the header link to the account surface, which is
	// served only when an origin is configured. Off means the link is absent
	// rather than dead (MUS-Q-0052).
	ShowAccount bool

	// Adapter lists the sessions a message can be sent to. Nil means none can
	// be, and the page says so rather than pretending.
	Adapter *session.Adapter
	// Store holds the routing records, which is where the idea inbox comes
	// from. Nil means the composer offers sessions only.
	Store *store.Store
	// Project is the identifier prefix a jot is filed under when the
	// destination does not name its own.
	Project string
	Actor   string
	// ExportTo is the tree the store is rendered into after a jot is filed, for
	// the reason the intake box has one: whoever composes from a phone cannot
	// run `make export`.
	ExportTo string
	Now      func() time.Time
}

func (c *Compose) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Compose) actor(r *http.Request) string {
	if v := r.Header.Get("Cf-Access-Authenticated-User-Email"); v != "" {
		return v
	}
	return c.Actor
}

// Routes registers the surface on an existing mux.
func (c *Compose) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /compose", c.show)
	mux.HandleFunc("POST /compose", c.send)
	mux.HandleFunc("GET /assets/compose.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(composeJS))
	})
}

// A composeTarget is one row in the destination list.
type composeTarget struct {
	// ID is what the form posts back: a project name for a session, or a
	// routing record's identifier for anything else.
	ID    string
	Label string
	// Detail is the second line: how long ago a session was active, or what the
	// inbox is for.
	Detail string
	Chosen bool
	// Jot marks a destination that files a record rather than typing into a
	// session, so the page can say which of the two is about to happen.
	Jot bool
}

type composePage struct {
	Targets []composeTarget
	// ShowAccount renders the header link to the account surface, which is
	// served only when an origin is configured. Off means the link is absent
	// rather than dead (MUS-Q-0052).
	ShowAccount bool
	// None is true when there is nowhere at all to send, which is a different
	// page rather than a form that cannot be submitted.
	None  bool
	Draft string
	Sent  string
	Error string
	// Gone names a destination that was asked for and is no longer there, so
	// the page can say the choice moved instead of moving it quietly.
	Gone string
}

// targets is every destination, last-active first, with the idea inbox after
// the sessions.
//
// The order is the decision: a session that just printed something is what the
// owner is most likely answering, and MUS-D-0013 asks for that to cost one tap.
func (c *Compose) targets(ctx context.Context, chosen string) []composeTarget {
	var out []composeTarget
	if c.Adapter != nil {
		live, err := c.Adapter.List(ctx)
		if err == nil {
			for _, s := range session.ByActivity(live) {
				out = append(out, composeTarget{
					ID:     s.Project,
					Label:  s.Project,
					Detail: lastActive(s.Activity, c.now()),
				})
			}
		}
	}
	if c.Store != nil {
		// The inbox is not special-cased by name: it is whichever routing
		// record marks itself the intake default, which is the same record the
		// intake box falls back to. A store that names none offers sessions
		// only.
		if routes, err := intake.Destinations(ctx, c.Store); err == nil {
			for _, r := range routes {
				if v, ok := r.Get(intake.DefaultField); !ok ||
					!strings.EqualFold(strings.TrimSpace(v), intake.DefaultValue) {
					continue
				}
				out = append(out, composeTarget{
					ID:     r.ID,
					Label:  r.Title,
					Detail: "files a record; no session is typed into",
					Jot:    true,
				})
				break
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	// The default is the first, which is the last active session — or the inbox
	// when nothing is running, so a thought still has somewhere to go.
	picked := false
	for i := range out {
		if out[i].ID == chosen {
			out[i].Chosen, picked = true, true
		}
	}
	if !picked {
		out[0].Chosen = true
	}
	return out
}

// gone reports a destination that was asked for and is not on offer.
//
// It used to be silent: the pill moved to the last active session while the
// error text still named the old one, so the next Send delivered somewhere the
// owner had not chosen. MUS-D-0013's accepted cost is that the routing step
// must never misfire silently, and quietly changing the target is exactly that.
func gone(targets []composeTarget, chosen string) string {
	if strings.TrimSpace(chosen) == "" {
		return ""
	}
	for _, t := range targets {
		if t.ID == chosen {
			return ""
		}
	}
	return chosen
}

// lastActive renders how long ago a session did anything. Coarse, because the
// number is there to order the list and to reassure, not to be acted on.
func lastActive(at, now time.Time) string {
	if at.IsZero() {
		return "no activity reported"
	}
	d := now.Sub(at)
	switch {
	case d < 0:
		return "active just now"
	case d < time.Minute:
		return "active just now"
	case d < time.Hour:
		return fmt.Sprintf("active %dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("active %dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("active %dd ago", int(d.Hours()/24))
	}
}

func (c *Compose) show(w http.ResponseWriter, r *http.Request) {
	want := r.URL.Query().Get("to")
	targets := c.targets(r.Context(), want)
	c.render(w, composePage{
		Targets: targets,
		None:    len(targets) == 0,
		Sent:    r.URL.Query().Get("sent"),
		Error:   r.URL.Query().Get("error"),
		Gone:    gone(targets, want),
	})
}

func (c *Compose) send(w http.ResponseWriter, r *http.Request) {
	// The same check the session socket makes, for the same reason: this path
	// types into a running agent, browsers send Origin on a cross-site form
	// post, and Access authenticates the person rather than the page they
	// happened to be on. A request with no Origin at all is a form post from a
	// browser that did not send one, or something that is not a browser; it is
	// refused here as it is there.
	if !sameOrigin(r) {
		http.Error(w, "cross-origin post refused", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		c.redirect(w, r, "", "", "that form did not arrive intact: "+err.Error())
		return
	}
	text := strings.TrimSpace(r.FormValue("text"))
	to := r.FormValue("to")
	if text == "" {
		c.redirect(w, r, to, "", "nothing to send")
		return
	}

	// A send outlives the request that started it, for the reason the answer
	// path does: a phone that navigates away mid-post used to cancel the write
	// through the request's context, and the thing being written is the whole
	// point of the surface.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), session.DeliverTimeout)
	defer cancel()

	sent, err := c.deliver(ctx, to, text, c.actor(r))
	if err != nil {
		// The failure page is rendered rather than redirected to, which is what
		// the intake box already does. The draft used to travel in the redirect
		// URL — where it lands in browser history and in the edge's logs, while
		// this same package argues the prose must not sit in a tmux buffer
		// anything on the machine could read.
		targets := c.targets(r.Context(), to)
		c.render(w, composePage{
			Targets: targets,
			None:    len(targets) == 0,
			Draft:   text,
			Error:   err.Error(),
			Gone:    gone(targets, to),
		})
		return
	}
	c.redirect(w, r, to, sent, "")
}

// deliver routes one composed message and says what happened, in a sentence fit
// to show the person who wrote it.
func (c *Compose) deliver(ctx context.Context, to, text, actor string) (string, error) {
	if to == "" {
		return "", errors.New("no destination chosen")
	}
	// A record if the destination parses as an identifier, a session otherwise.
	//
	// This was `strings.Contains(to, "-")`, and project names admit hyphens:
	// a session called TradeShop-Support was read as a routing identifier,
	// refused by the registry, and could never be sent to at all. The
	// identifier scheme is the thing that knows what an identifier looks like.
	if ident.Valid(to) && c.Store != nil {
		r, dest, err := intake.File(ctx, c.Store, intake.Request{
			Project: c.Project, Text: text, Actor: actor, Now: c.now(), To: to,
		})
		if err != nil {
			return "", err
		}
		if c.ExportTo != "" {
			// Best effort, and said so: the record is written either way, and a
			// failed export is not a reason to tell the owner their thought was
			// lost.
			if all, err := c.Store.List(ctx, ""); err == nil {
				_ = export.Write(c.ExportTo, all)
			}
		}
		return fmt.Sprintf("filed %s to %s", r.ID, dest.Name), nil
	}
	if c.Adapter == nil {
		return "", errors.New("no adapter, so no session can be typed into")
	}
	if err := c.Adapter.Send(ctx, to, text); err != nil {
		return "", err
	}
	return "sent to " + to, nil
}

func (c *Compose) redirect(w http.ResponseWriter, r *http.Request, to, sent, msg string) {
	q := url.Values{}
	if to != "" {
		q.Set("to", to)
	}
	if sent != "" {
		q.Set("sent", sent)
	}
	if msg != "" {
		q.Set("error", msg)
	}
	http.Redirect(w, r, "/compose?"+q.Encode(), http.StatusSeeOther)
}

func (c *Compose) render(w http.ResponseWriter, p composePage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// A capture surface a browser caches is one that shows a stale draft.
	w.Header().Set("Cache-Control", "no-store")
	p.ShowAccount = c.ShowAccount
	if err := composeTmpl.Execute(w, p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var composeTmpl = template.Must(template.New("compose").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mustur — compose</title>
<style>
  :root { color-scheme: light dark; --edge: #8884; --accent: #6a8fd8;
          --accent-soft: #6a8fd820; }
  body { font: 17px/1.5 system-ui, sans-serif; margin: 0; max-width: 46rem;
         margin-inline: auto; display: flex; flex-direction: column;
         min-height: 100vh; }
  /* MUS-Q-0052: the account surface is reached from here rather than from
     a fifth tab, so MUS-D-0041's four stand. Rendered only when the server
     actually serves it — a link that goes nowhere is the failure the bar
     itself was written to avoid. */
  .acct { font-size: .82em; opacity: .6; text-decoration: none;
          color: inherit; margin-left: auto; }
  header { display: flex; align-items: center; gap: .6rem; padding: .75rem 1rem;
           border-bottom: 1.4px solid var(--edge); white-space: nowrap; }
  header a { color: inherit; text-decoration: none; opacity: .7; }
  header .kept { margin-left: auto; opacity: .65; font-size: .82em;
                 font-style: italic; }
  /* The box is the page. Thought first: it is what the screen is for, and the
     destination sits under it rather than above. */
  .box { flex: 1; display: flex; padding: 1rem; }
  textarea { flex: 1; width: 100%; font: inherit; padding: .7rem;
             border: 1px solid var(--edge); border-radius: .5rem;
             background: transparent; color: inherit; resize: none;
             box-sizing: border-box; line-height: 1.45; }
  .foot { padding: .8rem 1rem; border-top: 1.4px solid var(--edge);
          display: flex; flex-direction: column; gap: .7rem; }
  .to { display: flex; align-items: baseline; gap: .5rem; font-size: .85em; }
  .to .lead { opacity: .6; }
  .routes { display: flex; gap: .4rem; overflow-x: auto; padding-bottom: .2rem;
            white-space: nowrap; }
  .routes label { flex: 0 0 auto; border: 1px solid var(--edge);
                  border-radius: 999px; padding: .25rem .75rem; font-size: .85em;
                  opacity: .65; cursor: pointer; }
  .routes label.on { opacity: 1; border-color: var(--accent);
                     background: var(--accent-soft); }
  .routes input { position: absolute; opacity: 0; pointer-events: none; }
  .routes .detail { display: block; font-size: .78em; opacity: .7; }
  button { font: inherit; width: 100%; padding: .7rem 1rem;
           border: 1px solid var(--accent); border-radius: .5rem;
           background: var(--accent-soft); color: inherit;
           transition: background-color .12s ease, border-color .12s ease; }
  button:hover { border-color: var(--accent); background: #6a8fd833; }
  button:active { background: #6a8fd855; transition-duration: 0s; }
  button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .said { margin: .8rem 1rem 0; padding: .6rem .8rem;
          border-left: 3px solid var(--edge); font-size: .9em; }
  .none { opacity: .6; padding: 2rem 1rem; text-align: center; }
` + shellCSS + `
</style>
</head>
<body>
<header><a href="/sessions" aria-label="Back">←</a><strong>Compose</strong>
  <span class="kept" id="kept" hidden>draft kept</span>{{if .ShowAccount}}<a class="acct" href="/account">Account</a>{{end}}</header>
{{if .Sent}}<p class="said">{{.Sent}}</p>{{end}}
{{if .Error}}<p class="said">Not sent: {{.Error}}</p>{{end}}
{{if .Gone}}<p class="said">{{.Gone}} is no longer a destination, so nothing is chosen for you. Pick one below.</p>{{end}}
{{if .None}}
<p class="none">Nowhere to send.<br>
<small>Mustur has no session running and no idea inbox to file to.</small></p>
{{else}}
<form method="post" action="/compose">
  <div class="box">
    <textarea id="text" name="text" placeholder="Write it. Choose where it goes after."
              spellcheck="true" autocapitalize="sentences" autocorrect="on"
              autocomplete="off" autofocus>{{.Draft}}</textarea>
  </div>
  <div class="foot">
    <div class="to"><span class="lead">Send to</span></div>
    <div class="routes">
      {{range .Targets}}<label{{if .Chosen}} class="on"{{end}}>
        <input type="radio" name="to" value="{{.ID}}"{{if .Chosen}} checked{{end}}>
        {{.Label}}<span class="detail">{{.Detail}}</span>
      </label>{{end}}
    </div>
    <button type="submit">Send</button>
  </div>
</form>
{{end}}
<nav>
  <a href="/sessions" aria-label="Sessions"><i class="ic ic-sess"></i><span>Sessions</span></a>
  <a href="/questions" aria-label="Decisions"><i class="ic ic-dec">?</i><span>Decisions</span></a>
  <a href="/intake" aria-label="Intake"><i class="ic ic-in"></i><span>Intake</span></a>
  <a href="/records" aria-label="Records"><i class="ic ic-rec"></i><span>Records</span></a>
  {{if .ShowAccount}}<a class="me" href="/account" title="Account" aria-label="Account"><i class="ic ic-acc"></i></a>{{end}}
</nav>
{{if not .None}}<script src="/assets/compose.js"></script>{{end}}
</body>
</html>
`))
