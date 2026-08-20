// Package web serves Mustur's human surfaces. Server-rendered HTML, no
// per-project client state: the plan rejects a feature on the grounds that it
// costs client state as it grows, and the intake box is the cheapest surface
// there is to keep that promise on.
package web

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/intake"
	"github.com/DevOfPie/Mustur/internal/store"
)

// Recent is how far back the intake surface looks. The owner's call, recorded:
// a phone surface shows the recent and nothing else, and history moves off the
// working surface rather than being lost.
const Recent = time.Hour

// Intake serves the capture box.
type Intake struct {
	Store   *store.Store
	Project string
	Actor   string
	Now     func() time.Time
}

// Handler routes the two methods the surface needs. GET renders the box, POST
// files and redirects — post/redirect/get, so that a phone reloading the page
// after a dropped connection does not file the same jot twice.
func (in *Intake) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /intake", in.show)
	mux.HandleFunc("POST /intake", in.file)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/intake", http.StatusSeeOther)
	})
	return mux
}

// MaxJot is the largest body the capture path accepts.
const MaxJot = 64 << 10

type page struct {
	Jot     string // What was typed, when it has to come back.
	Filed   string
	Routed  string
	Why     string
	Error   string
	Recent  []recentJot
	Cutoff  string
	Project string
}

type recentJot struct {
	ID     string
	Title  string
	Routed string
}

func (in *Intake) now() time.Time {
	if in.Now != nil {
		return in.Now()
	}
	return time.Now()
}

func (in *Intake) show(w http.ResponseWriter, r *http.Request) {
	p := page{
		Filed:   r.URL.Query().Get("filed"),
		Routed:  r.URL.Query().Get("routed"),
		Why:     r.URL.Query().Get("why"),
		Project: in.Project,
		Cutoff:  "the last hour",
	}
	recent, err := in.recent(r.Context())
	if err != nil {
		p.Error = err.Error()
	}
	p.Recent = recent
	render(w, p)
}

func (in *Intake) file(w http.ResponseWriter, r *http.Request) {
	// A capture box on the public side of an ingress is the obvious place to
	// post a gigabyte at. The limit is generous for a jot and finite, which is
	// the whole requirement.
	r.Body = http.MaxBytesReader(w, r.Body, MaxJot)
	if err := r.ParseForm(); err != nil {
		render(w, page{Error: "that form did not arrive intact: " + err.Error(), Project: in.Project})
		return
	}
	text := r.PostFormValue("jot")
	rec, to, err := intake.File(r.Context(), in.Store, in.Project, text, in.actor(r), in.now())
	if err != nil {
		// Rendered rather than redirected, and carrying the text back with it.
		// The comment here used to say that losing what was typed is the one
		// failure this surface cannot have, above code that dropped it: the
		// page had no field for it and the textarea came back empty. On a
		// phone that is a thumb-typed paragraph gone.
		render(w, page{Error: err.Error(), Project: in.Project, Jot: text})
		return
	}
	q := fmt.Sprintf("/intake?filed=%s&routed=%s&why=%s",
		template.URLQueryEscaper(rec.ID), template.URLQueryEscaper(to.Name), template.URLQueryEscaper(to.Why))
	http.Redirect(w, r, q, http.StatusSeeOther)
}

// actor is who filed a jot. Cloudflare Access puts the authenticated identity
// in a header at the edge; until that is in front of this, the configured actor
// is who it is, and the record says so either way rather than guessing.
func (in *Intake) actor(r *http.Request) string {
	if who := r.Header.Get("Cf-Access-Authenticated-User-Email"); who != "" {
		return who
	}
	return in.Actor
}

func (in *Intake) recent(ctx context.Context) ([]recentJot, error) {
	// The log's own timestamps, not the records' dates. A record carries the
	// date its content was true, to the day; "what did I file in the last hour"
	// is a different question and only the log can answer it.
	findings, err := in.Store.Since(ctx, "finding", in.now().Add(-Recent))
	if err != nil {
		return nil, err
	}
	out := make([]recentJot, 0, len(findings))
	for _, f := range findings {
		routed, _ := f.Get("Routed to")
		out = append(out, recentJot{ID: f.ID, Title: f.Title, Routed: routed})
	}
	return out, nil
}

func render(w http.ResponseWriter, p page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// A capture surface that a browser caches is one that shows a stale list
	// after a jot lands.
	w.Header().Set("Cache-Control", "no-store")
	if err := tmpl.Execute(w, p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// tmpl is the whole surface. One box, a button, and what just happened.
//
// No stylesheet, no script, no font, no image: everything a phone off the home
// network has to fetch is another thing between a thought and it being filed,
// and the target is fifteen seconds including the network.
var tmpl = template.Must(template.New("intake").Funcs(template.FuncMap{
	"trim": strings.TrimSpace,
}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mustur — intake</title>
<style>
  :root { color-scheme: light dark; --edge: #8884; }
  body { font: 17px/1.5 system-ui, sans-serif; margin: 0; padding: 1rem;
         max-width: 40rem; margin-inline: auto; }
  h1 { font-size: 1rem; font-weight: 600; margin: 0 0 .75rem; opacity: .7; }
  textarea { width: 100%; min-height: 8rem; font: inherit; padding: .6rem;
             border: 1px solid var(--edge); border-radius: .5rem;
             background: transparent; color: inherit; box-sizing: border-box; }
  button { font: inherit; padding: .6rem 1.2rem; margin-top: .6rem;
           border: 1px solid var(--edge); border-radius: .5rem;
           background: transparent; color: inherit; width: 100%; }
  .said { margin: .9rem 0; padding: .6rem .8rem; border-left: 3px solid var(--edge); }
  .said code { font-size: .95em; }
  .why { opacity: .7; font-size: .9em; }
  ul { list-style: none; padding: 0; margin: 1.5rem 0 0; }
  li { padding: .5rem 0; border-top: 1px solid var(--edge); }
  li .to { opacity: .7; font-size: .85em; display: block; }
  .none { opacity: .6; font-size: .9em; margin-top: 1.5rem; }
</style>
</head>
<body>
<h1>Mustur — {{.Project}}</h1>
{{if .Error}}<p class="said">Not filed: {{.Error}}</p>{{end}}
{{if .Filed}}<p class="said">Filed <code>{{.Filed}}</code>{{if .Routed}} → {{.Routed}}{{end}}<br>
<span class="why">{{.Why}}</span></p>{{end}}
<form method="post" action="/intake">
  <textarea name="jot" autofocus placeholder="A line. Nothing to decide.">{{.Jot}}</textarea>
  <button type="submit">File it</button>
</form>
{{if .Recent}}<ul>
{{range .Recent}}<li><code>{{.ID}}</code> {{.Title}}<span class="to">{{.Routed}}</span></li>{{end}}
</ul>{{else}}<p class="none">Nothing filed in {{.Cutoff}}.</p>{{end}}
</body>
</html>
`))
