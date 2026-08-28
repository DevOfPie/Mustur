// Package web serves Mustur's human surfaces. Server-rendered HTML, no
// per-project client state: the plan rejects a feature on the grounds that it
// costs client state as it grows, and the intake box is the cheapest surface
// there is to keep that promise on.
package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
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

	// ShowAccount renders the header link to the account surface, which is
	// served only when an origin is configured. Off means the link is absent
	// rather than dead (MUS-Q-0052).
	ShowAccount bool
	// ShowSessions says whether this server serves the session surface, which
	// decides whether the bar offers a tab to it. Off by default: the session
	// surface carries a composer that types into a running agent's stdin, so
	// publishing it is a deliberate act rather than a consequence of deploying
	// something else that happens to be in the same binary.
	ShowSessions bool

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

// scratchTo is the destination that files nothing.
//
// Not a routing identifier and deliberately unlike one: nothing should be able
// to cite a scratch filing, because there is nothing there to cite.
const scratchTo = "scratch"

// MaxJot is the largest body the capture path accepts.
const MaxJot = 64 << 10

type destination struct {
	ID   string
	Name string
	Kind string
}

// A destGroup is one kind of destination, with a heading a reader can use to
// tell two similarly-named things apart.
type destGroup struct {
	Label string
	Items []destination
}

// grouped orders the destinations by kind, projects first.
//
// A project is what a jot usually belongs to; a repository is a tree inside one
// and a machine is where that tree sits. Presented flat they read as four equal
// choices, two of which are the same thing at different altitudes — which is
// exactly what the owner asked about.
func grouped(all []destination) []destGroup {
	order := []struct{ kind, label string }{
		{"project", "Projects"},
		{"repository", "Repositories"},
		{"machine", "Machines"},
	}
	var out []destGroup
	for _, o := range order {
		g := destGroup{Label: o.label}
		for _, d := range all {
			if d.Kind == o.kind {
				g.Items = append(g.Items, d)
			}
		}
		if len(g.Items) > 0 {
			out = append(out, g)
		}
	}
	return out
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
	// Scratch filings, which are not records and have no identifier.
	Scratch []store.Scratch
	// Groups are the destinations, gathered by kind. The owner asked why
	// "DevOfPie/Mustur" and "Mustur" both appear: they are a repository and the
	// project that contains it, which the flat row gave no way to tell.
	Groups      []destGroup
	Cutoff      string
	Project     string
	ShowAccount bool
	// ShowSessions renders the Sessions tab. Off unless the server is actually
	// serving that surface: a tab that goes nowhere is an unbuilt capability
	// described as existing, which is what MUS-D-0041's bar exists to avoid.
	ShowSessions bool

	// OpenQuestions drives the banner, which is **interim**. MUS-D-0041 chose a
	// fixed place the eye already knows to check over a banner on another
	// screen, on the grounds that a banner can be scrolled past — and that is
	// still the decision.
	//
	// The bar is on both surfaces now, carrying the ones that are built and
	// growing as the rest arrive. The banner stays beside it because they do
	// different jobs: the bar is the fixed place the eye knows to check, and the
	// banner is what makes an open decision impossible to miss on the surface
	// the owner happened to open. Owner-confirmed as interim on MUS-Q-0006 and
	// MUS-Q-0012.
	//
	// Until the bar reached here, the banner was the *only* route from intake to
	// the queue — which meant the queue was reachable from intake exactly when
	// it had something to say, and unreachable when it did not. The owner found
	// that by loading the site and seeing only the intake box.
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
		ShowSessions: in.ShowSessions,
		ShowAccount:  in.ShowAccount,
		Filed:        r.URL.Query().Get("filed"),
		Routed:       r.URL.Query().Get("routed"),
		Why:          r.URL.Query().Get("why"),
		Warn:         r.URL.Query().Get("warn"),
		Project:      in.Project,
		Cutoff:       "the last hour",
	}
	if choices, err := in.destinations(r.Context()); err != nil {
		p.Error = err.Error()
	} else {
		p.Destinations = choices
		p.Groups = grouped(choices)
	}
	recent, err := in.recent(r.Context())
	if err != nil {
		p.Error = err.Error()
	}
	p.Recent = recent
	// Swept as the page is drawn rather than on a timer: this surface is the
	// only thing that reads them, so nothing accumulates unseen.
	if _, err := in.Store.SweepScratch(r.Context(), in.now().Add(-store.ScratchLife)); err == nil {
		if left, err := in.Store.Scratches(r.Context()); err == nil {
			p.Scratch = left
		}
	}
	p.OpenQuestions = OpenCount(r.Context(), in.Store)
	render(w, p)
}

func (in *Intake) file(w http.ResponseWriter, r *http.Request) {
	// A capture box on the public side of an ingress is the obvious place to
	// post a gigabyte at. The limit is generous for a jot and finite, which is
	// the whole requirement.
	//
	// An image raises the ceiling but does not remove it: MaxJot for the words,
	// plus room for one picture and the multipart framing around it.
	r.Body = http.MaxBytesReader(w, r.Body, MaxJot+store.MaxAttachment+(1<<16))
	if err := r.ParseMultipartForm(1 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		render(w, page{Error: "that form did not arrive intact: " + err.Error(), Project: in.Project, ShowSessions: in.ShowSessions, ShowAccount: in.ShowAccount})
		return
	}
	text := r.PostFormValue("jot")
	// The words keep their own limit. Raising the body cap to make room for a
	// picture would otherwise have raised the ceiling on the text with it, and
	// TestAnOversizedJotIsRefusedRatherThanStored said so immediately.
	if len(text) > MaxJot {
		render(w, page{
			Error:   "that is longer than this box takes; it is for a line, not a document",
			Project: in.Project, ShowSessions: in.ShowSessions, ShowAccount: in.ShowAccount,
		})
		return
	}

	// Read the image before the record is written, so a picture this refuses
	// does not leave a jot behind claiming to have one.
	image, imageErr := readImage(r)
	if imageErr != nil {
		render(w, page{Error: imageErr.Error(), Project: in.Project, Jot: text, ShowSessions: in.ShowSessions, ShowAccount: in.ShowAccount})
		return
	}
	// Scratch: filed beside the records rather than among them. It takes no
	// identifier and never enters the log, which is the whole point — testing
	// the box twice cost two permanent identifiers in the idea warehouse.
	if r.PostFormValue("to") == scratchTo {
		sc, err := in.Store.Scratched(r.Context(), text, in.actor(r))
		if err != nil {
			render(w, page{Error: err.Error(), Project: in.Project, Jot: text,
				ShowSessions: in.ShowSessions, ShowAccount: in.ShowAccount})
			return
		}
		if len(image) > 0 {
			// Attached to the scratch row's own id, so the sweep takes the
			// picture with the note rather than leaving it unreachable.
			if _, err := in.Store.Attach(r.Context(), sc.ID, image, in.actor(r)); err != nil {
				render(w, page{Error: "kept the note, but not the image: " + err.Error(),
					Project: in.Project, ShowSessions: in.ShowSessions, ShowAccount: in.ShowAccount})
				return
			}
		}
		http.Redirect(w, r, "/intake?warn="+template.URLQueryEscaper(
			"filed to scratch: no identifier, not exported, gone on restart"), http.StatusSeeOther)
		return
	}

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
		render(w, page{Error: err.Error(), Project: in.Project, Jot: text, ShowSessions: in.ShowSessions, ShowAccount: in.ShowAccount})
		return
	}
	if len(image) > 0 {
		if _, err := in.Store.Attach(r.Context(), rec.ID, image, in.actor(r)); err != nil {
			// The jot is already filed and is worth more than the picture. Say
			// what happened rather than losing the words to a failed image.
			render(w, page{
				Error:   "filed " + rec.ID + ", but the image was not stored: " + err.Error(),
				Project: in.Project, ShowSessions: in.ShowSessions, ShowAccount: in.ShowAccount,
			})
			return
		}
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

// readImage takes the one picture a jot may carry.
//
// Nothing about the upload is trusted: not its name, which is never stored, not
// its Content-Type, which the sender also chose, and not its length, which is
// bounded by the reader rather than believed from a header. What comes back is
// bytes, and store.Attach decides whether they are an image.
func readImage(r *http.Request) ([]byte, error) {
	if r.MultipartForm == nil {
		return nil, nil
	}
	f, _, err := r.FormFile("image")
	if errors.Is(err, http.ErrMissingFile) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("that image did not arrive intact: %w", err)
	}
	defer f.Close()

	// One byte past the limit is enough to know it is too big, and stops a
	// large upload being read into memory in full only to be refused.
	data, err := io.ReadAll(io.LimitReader(f, store.MaxAttachment+1))
	if err != nil {
		return nil, fmt.Errorf("that image did not arrive intact: %w", err)
	}
	if len(data) > store.MaxAttachment {
		return nil, store.ErrTooLarge
	}
	// Decided here, before the record is written. Leaving it to Attach meant a
	// refused picture had already left a jot behind claiming to have one.
	if _, err := store.ImageType(data); err != nil {
		return nil, err
	}
	return data, nil
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
  :root { color-scheme: light dark; --edge: #8884; --accent: #6a8fd8; }
  /* border-box, because this body carries its own padding and the shell caps
     its width. Without it the padding lands outside the cap and this is the
     one surface that reaches the right edge while the rest keep a gutter. */
  body { font: 17px/1.5 system-ui, sans-serif; margin: 0; padding: 1rem;
         box-sizing: border-box; max-width: 40rem; margin-inline: auto; }
  h1 { font-size: 1rem; font-weight: 600; margin: 0 0 .75rem; opacity: .7; }
  textarea { width: 100%; min-height: 8rem; font: inherit; padding: .6rem;
             border: 1px solid var(--edge); border-radius: .5rem;
             background: transparent; color: inherit; box-sizing: border-box; }
  button { font: inherit; padding: .6rem 1.2rem; margin-top: .6rem;
           border: 1px solid var(--edge); border-radius: .5rem;
           background: transparent; color: inherit; width: 100%;
           /* MUS-F-0024, filed from a phone: the button looked the same before
              and after a tap, so there was no way to tell one had registered.
              A phone has no hover, so :active is the half that matters — it is
              first, and the transition is short enough to survive a quick tap
              rather than animating past it. */
           transition: background-color .12s ease, border-color .12s ease; }
  button:hover { border-color: #888a; background: #8881; }
  button:active { border-color: #8888; background: #8883;
                  transition-duration: 0s; }
  button:focus-visible { outline: 2px solid #888a; outline-offset: 2px; }
  .said { margin: .9rem 0; padding: .6rem .8rem; border-left: 3px solid var(--edge); }
  /* Pinned above the box, not below it. A blocked agent is work stopped, and
     the owner should see that before they start typing something else. This is
     the interim placement; MUS-D-0041's fixed place is the tab bar, which the
     queue now carries and intake does not. */
  .waiting { margin: 0 0 .75rem; padding: .55rem .8rem; border: 1px solid var(--edge);
             border-radius: .5rem; font-size: .95em; }
  .waiting a { color: inherit; }
  .said code { font-size: .95em; }
  .why { opacity: .7; font-size: .9em; }
  ul { list-style: none; padding: 0; margin: 1.5rem 0 0; }
  li { padding: .5rem 0; border-top: 1px solid var(--edge); }
  /* An identifier is the thing you want next: it goes to the record rather
     than sitting there as text you have to retype somewhere else. */
  /* A scratch filing looks like what it is: no identifier to follow, and a
     standing note that it is going. */
  .scratch li { opacity: .75; }
  .tmp { font-size: .78em; border: 1px solid var(--edge); border-radius: 999px;
         padding: .05rem .5rem; margin-right: .4rem; opacity: .8; }
  .pic { display: flex; flex-direction: column; gap: .3rem; margin-top: .6rem;
         font-size: .88em; opacity: .8; }
  .pic small { opacity: .7; font-size: .85em; }
  .rec { color: inherit; text-decoration: none; border-bottom: 1px solid var(--edge); }
  .rec:hover, .rec:focus-visible { border-bottom-color: var(--accent, currentColor); }
  li .to { opacity: .7; font-size: .85em; display: block; }
  .none { opacity: .6; font-size: .9em; margin-top: 1.5rem; }
  /* A list rather than a row of chips.

     The chips were one line that scrolled sideways, on the reasoning that a
     clipped repository name is a wrong destination picked by accident. They
     produced exactly that: 741px of choices in a 640px row, so the last one —
     the idea inbox — sat off the edge with nothing saying it was there
     (MUS-F-0036). Adding scratch made a sixth.

     A native select has no hidden end, groups its options by kind so two
     similarly-named destinations can be told apart, becomes the system picker
     on a phone, and takes type-ahead on a desktop for free. It is still a form
     control, so this surface still works with script blocked. */
  .to { display: flex; flex-direction: column; gap: .25rem; margin-top: .6rem; }
  .to span { font-size: .85em; opacity: .6; }
  .to select { font: inherit; font-size: .95em; padding: .5rem;
               border: 1px solid var(--edge); border-radius: .5rem;
               background: transparent; color: inherit; width: 100%; }
  /* The bar MUS-D-0041 chose, carrying the surfaces that exist. Without it the
     only route from here to the queue was the banner, which renders when
     something is open — so the queue was reachable from intake exactly when it
     had nothing to say. The owner found that by loading the site. */
  /* MUS-Q-0052: the account surface is reached from here rather than from
     a fifth tab, so MUS-D-0041's four stand. Rendered only when the server
     actually serves it — a link that goes nowhere is the failure the bar
     itself was written to avoid. */
  .acct { font-size: .82em; opacity: .6; text-decoration: none;
          color: inherit; margin-left: auto; }
  h1 { display: flex; align-items: baseline; }
` + shellCSS + `
</style>
</head>
<body>
<h1>Mustur — {{.Project}}{{if .ShowAccount}}<a class="acct" href="/account">Account</a>{{end}}</h1>
{{if .OpenQuestions}}<p class="waiting"><a href="/questions">{{.OpenQuestions}} decision{{if ne .OpenQuestions 1}}s{{end}} waiting on you</a></p>{{end}}
{{if .Error}}<p class="said">Not filed: {{.Error}}</p>{{end}}
{{if .Filed}}<p class="said">Filed <a class="rec" href="/records/{{.Filed}}"><code>{{.Filed}}</code></a>{{if .Routed}} → {{.Routed}}{{end}}<br>
<span class="why">{{.Why}}</span></p>{{end}}
{{if .Warn}}<p class="said">{{.Warn}}</p>{{end}}
<form method="post" action="/intake" enctype="multipart/form-data">
  <textarea name="jot" autofocus placeholder="A line. Nothing to decide.">{{.Jot}}</textarea>
  <label class="pic">A picture, if a picture says it faster
    <input type="file" name="image" accept="image/png,image/jpeg,image/gif,image/webp">
    <small>Held privately. The record carries what an agent reads in it, never the picture.</small>
  </label>
  <label class="to"><span>Where</span>
    <select name="to">
      <option value="" selected>Route it for me</option>
      {{range .Groups}}<optgroup label="{{.Label}}">
        {{range .Items}}<option value="{{.ID}}">{{.Name}}</option>{{end}}
      </optgroup>{{end}}
      <option value="scratch">Scratch &mdash; not kept, not counted</option>
    </select>
  </label>
  <button type="submit">File it</button>
</form>
{{if .Scratch}}<ul class="scratch">
{{range .Scratch}}<li><span class="tmp">scratch</span> {{.Text}}<span class="to">goes on restart</span></li>{{end}}
</ul>{{end}}
{{if .Recent}}<ul>
{{range .Recent}}<li><a class="rec" href="/records/{{.ID}}"><code>{{.ID}}</code></a> {{.Title}}<span class="to">{{.Routed}}</span></li>{{end}}
</ul>{{else}}<p class="none">Nothing filed in {{.Cutoff}}.</p>{{end}}
<nav>
  {{if .ShowSessions}}<a href="/sessions" aria-label="Sessions"><i class="ic ic-sess"></i><span>Sessions</span></a>{{end}}
  <a href="/questions" aria-label="Decisions"><i class="ic ic-dec">?</i><span>Decisions</span>{{if .OpenQuestions}}<em class="cnt">{{.OpenQuestions}}</em>{{end}}</a>
  <a href="/intake" class="here" aria-label="Intake"><i class="ic ic-in"><b></b></i><span>Intake</span></a>
  <a href="/records" aria-label="Records"><i class="ic ic-rec"></i><span>Records</span></a>
  {{if .ShowAccount}}<a class="me" href="/account" title="Account" aria-label="Account"><i class="ic ic-acc"></i></a>{{end}}
</nav>
</body>
</html>
`))
