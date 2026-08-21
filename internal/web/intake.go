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

	"github.com/DevOfPie/Mustur/internal/export"
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

	// ExportTo is the tree the store is rendered into after a jot is filed.
	// Empty means no export, which is the safe default for a server that is
	// not sitting on a checkout.
	//
	// It exists because a jot filed from a phone reached the store and nothing
	// else. The findings role is mapped at the exported file, so until this
	// ran, "lands in Mustur's findings-queue" was true of the database and not
	// of the thing the audit reads. Whoever files from a phone cannot run
	// `make export`, which is what makes this the surface's problem rather
	// than the operator's.
	ExportTo string
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

type destination struct {
	ID   string
	Name string
	Kind string
}

type page struct {
	Jot          string // What was typed, when it has to come back.
	Warn         string
	Destinations []destination
	Filed        string
	Routed       string
	Why          string
	Error        string
	Recent       []recentJot
	Cutoff       string
	Project      string

	// OpenQuestions is why the banner exists. A decision queue nobody opens is
	// the failure milestone 3 is named after, so the count travels to the
	// surface the owner did open rather than waiting to be visited.
	OpenQuestions int
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
		Warn:    r.URL.Query().Get("warn"),
		Project: in.Project,
		Cutoff:  "the last hour",
	}
	if choices, err := in.destinations(r.Context()); err != nil {
		p.Error = err.Error()
	} else {
		p.Destinations = choices
	}
	recent, err := in.recent(r.Context())
	if err != nil {
		p.Error = err.Error()
	}
	p.Recent = recent
	p.OpenQuestions = OpenCount(r.Context(), in.Store)
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
	rec, to, err := intake.File(r.Context(), in.Store, intake.Request{
		Project: in.Project,
		Text:    text,
		Actor:   in.actor(r),
		To:      r.PostFormValue("to"),
		Now:     in.now(),
	})
	if err != nil {
		// Rendered rather than redirected, and carrying the text back with it.
		// The comment here used to say that losing what was typed is the one
		// failure this surface cannot have, above code that dropped it: the
		// page had no field for it and the textarea came back empty. On a
		// phone that is a thumb-typed paragraph gone.
		render(w, page{Error: err.Error(), Project: in.Project, Jot: text})
		return
	}
	// The record is already in the store, so an export that fails has not lost
	// the jot — but saying nothing would leave the exported tree quietly behind
	// the store, which is the drift this repository has no gate for.
	exported := ""
	if err := in.export(r.Context()); err != nil {
		exported = err.Error()
	}
	q := fmt.Sprintf("/intake?filed=%s&routed=%s&why=%s&warn=%s",
		template.URLQueryEscaper(rec.ID), template.URLQueryEscaper(to.Name),
		template.URLQueryEscaper(to.Why), template.URLQueryEscaper(exported))
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

// destinations are the routing records a filer may pick instead of leaving it
// to the guess.
func (in *Intake) destinations(ctx context.Context) ([]destination, error) {
	routing, err := intake.Destinations(ctx, in.Store)
	if err != nil {
		return nil, err
	}
	out := make([]destination, 0, len(routing))
	for _, r := range routing {
		out = append(out, destination{ID: r.ID, Name: r.Title, Kind: r.Kind})
	}
	return out, nil
}

// export renders the store into the configured tree. A no-op when none is
// configured.
func (in *Intake) export(ctx context.Context) error {
	if in.ExportTo == "" {
		return nil
	}
	records, err := in.Store.List(ctx, "")
	if err != nil {
		return fmt.Errorf("the jot is filed; reading the store to export it failed: %w", err)
	}
	if err := export.Write(in.ExportTo, records); err != nil {
		return fmt.Errorf("the jot is filed; exporting it to %s failed: %w", in.ExportTo, err)
	}
	return nil
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
  /* Pinned above the box, not below it. A blocked agent is work stopped, and
     the owner should see that before they start typing something else. */
  .waiting { margin: 0 0 .75rem; padding: .55rem .8rem; border: 1px solid var(--edge);
             border-radius: .5rem; font-size: .95em; }
  .waiting a { color: inherit; }
  .said code { font-size: .95em; }
  .why { opacity: .7; font-size: .9em; }
  ul { list-style: none; padding: 0; margin: 1.5rem 0 0; }
  li { padding: .5rem 0; border-top: 1px solid var(--edge); }
  li .to { opacity: .7; font-size: .85em; display: block; }
  .none { opacity: .6; font-size: .9em; margin-top: 1.5rem; }
  /* One line that scrolls, never a block that wraps. Each choice sizes to its
     own text: a clipped repository name is a wrong destination picked by
     accident. */
  .dests { display: flex; gap: .4rem; margin-top: .6rem; overflow-x: auto;
           white-space: nowrap; padding-bottom: .25rem; }
  .dests label { flex: 0 0 auto; border: 1px solid var(--edge);
                 border-radius: 999px; padding: .35rem .7rem; font-size: .9em; }
</style>
</head>
<body>
<h1>Mustur — {{.Project}}</h1>
{{if .OpenQuestions}}<p class="waiting"><a href="/questions">{{.OpenQuestions}} decision{{if ne .OpenQuestions 1}}s{{end}} waiting on you</a></p>{{end}}
{{if .Error}}<p class="said">Not filed: {{.Error}}</p>{{end}}
{{if .Filed}}<p class="said">Filed <code>{{.Filed}}</code>{{if .Routed}} → {{.Routed}}{{end}}<br>
<span class="why">{{.Why}}</span></p>{{end}}
{{if .Warn}}<p class="said">{{.Warn}}</p>{{end}}
<form method="post" action="/intake">
  <textarea name="jot" autofocus placeholder="A line. Nothing to decide.">{{.Jot}}</textarea>
  {{if .Destinations}}<div class="dests">
    <label><input type="radio" name="to" value="" checked> Route it for me</label>
    {{range .Destinations}}<label><input type="radio" name="to" value="{{.ID}}"> {{.Name}}</label>{{end}}
  </div>{{end}}
  <button type="submit">File it</button>
</form>
{{if .Recent}}<ul>
{{range .Recent}}<li><code>{{.ID}}</code> {{.Title}}<span class="to">{{.Routed}}</span></li>{{end}}
</ul>{{else}}<p class="none">Nothing filed in {{.Cutoff}}.</p>{{end}}
</body>
</html>
`))
