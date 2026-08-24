package web

// Records — the reading surface, and the fourth tab.
//
// **A document, not a graph** (MUS-D-0040). Identifiers here are dense and
// cross-referential and the graph reading was real; what the owner chose is a
// thing to read, where a citation expands in place with no round trip and no
// new tab. The counts at the top are the only navigation.
//
// Expansion is a `<details>` element, which is why this page carries no script:
// the browser already knows how to open and close a thing, and the decision
// queue reached the same answer for the same reason.
//
// **Routing lives inside it**, because repositories, machines and projects are
// record kinds like any other and a separate page would be a second surface to
// keep true. What makes routing different is that its rows are claims about
// this machine — so the surface **verifies rather than repeats**: a checkout
// that moved, or a contract file that is gone, reads as stale on the row
// itself. That is the whole reason it is a surface and not a printed table, and
// it is the same posture the dispatcher contract takes, which verifies before
// entering rather than trusting a row.

import (
	"context"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

// Records serves the records document.
type Records struct {
	Store   *store.Store
	Project string
	// Home expands a leading ~ in a checkout path. Empty means the running
	// user's home, and it is injectable so the verification can be tested
	// without depending on whose machine the test runs on.
	Home string
	Now  func() time.Time
}

func (rr *Records) now() time.Time {
	if rr.Now != nil {
		return rr.Now()
	}
	return time.Now()
}

// Routes registers the surface.
func (rr *Records) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /records", rr.index)
	mux.HandleFunc("GET /records/{id}", rr.one)
}

// kinds is the order the document presents, which is the order the export
// already uses. Routing last, because it is the part about this machine rather
// than about the work.
var kinds = []struct {
	Kind string
	One  string
	Many string
}{
	{"milestone", "milestone", "milestones"},
	{"work-unit", "work unit", "work units"},
	{"question", "question", "questions"},
	{"decision", "decision", "decisions"},
	{"finding", "finding", "findings"},
	{"investigation", "investigation", "investigations"},
	// Both spellings are written out rather than derived. Trimming an "s"
	// turned "repositories" into "1 repositorie" on the first render against
	// the real store, which is what a rule about English usually does.
	{"repository", "repository", "repositories"},
	{"machine", "machine", "machines"},
	{"project", "project", "projects"},
}

type citation struct {
	Key   string
	ID    string
	Kind  string
	Title string
	At    string
	// Known is false for an identifier the store does not hold, which is a
	// dangling citation and says so rather than rendering an empty box.
	Known bool
	// Plain marks a ref whose value is not an identifier at all — a path, a
	// name — which is shown as written rather than looked up and reported
	// missing.
	Plain bool
}

type recordView struct {
	ID    string
	Kind  string
	Title string
	At    string
	Body  string
	Data  []record.Field
	Refs  []citation
	// Cites are identifiers found in the prose, which is where most of this
	// tree's cross-references actually live.
	Cites []citation
	// State is the verification, for a routing record. Empty for everything
	// else: a decision cannot be stale in this sense.
	State string
	Stale bool
}

type kindView struct {
	Kind string
	// Label is what the count says — singular when there is one of a thing.
	Label string
	// Heading is always the plural, because a section heading names a kind
	// rather than counting it.
	Heading string
	Count   int
	Records []recordView
}

type recordsPage struct {
	Project string
	Kinds   []kindView
	Total   int
	// One is set when a single record was asked for by identifier.
	One     *recordView
	Missing string
	Checked string
}

// idInProse finds identifiers written in a record's text.
var idInProse = regexp.MustCompile(`\b[A-Z]{3}-[A-Z]-[0-9]{4}\b`)

func (rr *Records) load(ctx context.Context) (map[string]record.Record, []record.Record, error) {
	all, err := rr.Store.List(ctx, "")
	if err != nil {
		return nil, nil, err
	}
	by := make(map[string]record.Record, len(all))
	for _, r := range all {
		by[r.ID] = r
	}
	return by, all, nil
}

// view builds what the page shows for one record, including its citations
// resolved so they can expand without another request.
func (rr *Records) view(r record.Record, by map[string]record.Record) recordView {
	v := recordView{ID: r.ID, Kind: r.Kind, Title: r.Title, At: r.At, Body: r.Body, Data: r.Data}

	// A ref field may name several records — "Decided by: MUS-D-0002,
	// MUS-D-0008, MUS-D-0027" is one field and three citations. Looking the
	// whole value up as one identifier rendered eleven perfectly good
	// citations as dangling on the first run against the real store, which is
	// the sort of thing that reads as a finding about the tree until somebody
	// looks.
	for _, ref := range r.Refs {
		found := idInProse.FindAllString(ref.Value, -1)
		if len(found) == 0 {
			// Not an identifier at all: some refs name a file or a person.
			v.Refs = append(v.Refs, citation{Key: ref.Key, ID: ref.Value, Plain: true})
			continue
		}
		for _, id := range found {
			v.Refs = append(v.Refs, resolve(ref.Key, id, by))
		}
	}

	// Identifiers in the prose and in field values, which is where most
	// citations in this tree live. Deduplicated, and never the record itself.
	seen := map[string]bool{r.ID: true}
	for _, ref := range r.Refs {
		seen[ref.Value] = true
	}
	text := r.Body
	for _, f := range r.Data {
		text += " " + f.Value
	}
	for _, found := range idInProse.FindAllString(text, -1) {
		if seen[found] {
			continue
		}
		seen[found] = true
		v.Cites = append(v.Cites, resolve("", found, by))
	}
	return v
}

func resolve(key, id string, by map[string]record.Record) citation {
	c := citation{Key: key, ID: id}
	if cited, ok := by[id]; ok {
		c.Known, c.Kind, c.Title, c.At = true, cited.Kind, cited.Title, cited.At
	}
	return c
}

// verify checks a routing record against the machine it describes.
//
// Only what can be checked cheaply and locally: that a checkout is where the
// row says, and that the contract file it names is in it. A row about another
// machine is not verifiable from here and says so rather than guessing.
func (rr *Records) verify(r record.Record) (string, bool) {
	if r.Kind != "repository" {
		return "", false
	}
	var path, contract string
	for _, f := range r.Data {
		switch {
		case strings.HasPrefix(f.Key, "Checkout on"):
			path = strings.TrimSpace(f.Value)
		case f.Key == "Contract":
			contract = strings.TrimSpace(f.Value)
		}
	}
	if path == "" {
		return "no checkout named", true
	}
	full := rr.expand(path)
	info, err := os.Stat(full)
	if err != nil || !info.IsDir() {
		return "stale — nothing at " + path, true
	}
	if contract != "" {
		if _, err := os.Stat(filepath.Join(full, contract)); err != nil {
			return "stale — no " + contract, true
		}
	}
	return "there", false
}

func (rr *Records) expand(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home := rr.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

func (rr *Records) index(w http.ResponseWriter, r *http.Request) {
	by, all, err := rr.load(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := recordsPage{Project: rr.Project, Total: len(all), Checked: rr.now().Format("15:04")}

	grouped := map[string][]record.Record{}
	for _, rec := range all {
		grouped[rec.Kind] = append(grouped[rec.Kind], rec)
	}
	for _, k := range kinds {
		recs := grouped[k.Kind]
		if len(recs) == 0 {
			continue
		}
		sort.Slice(recs, func(i, j int) bool { return less(recs[i].ID, recs[j].ID) })
		label := k.Many
		if len(recs) == 1 {
			label = k.One
		}
		kv := kindView{Kind: k.Kind, Label: label, Heading: k.Many, Count: len(recs)}
		for _, rec := range recs {
			v := rr.view(rec, by)
			v.State, v.Stale = rr.verify(rec)
			kv.Records = append(kv.Records, v)
		}
		page.Kinds = append(page.Kinds, kv)
	}
	rr.render(w, page)
}

// one is the canonical URL for a single record, which is what "every record
// addressable by identifier" means: a bare identifier can be pasted into a bar.
func (rr *Records) one(w http.ResponseWriter, r *http.Request) {
	id := strings.ToUpper(strings.TrimSpace(r.PathValue("id")))
	by, _, err := rr.load(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rec, ok := by[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		rr.render(w, recordsPage{Project: rr.Project, Missing: id})
		return
	}
	v := rr.view(rec, by)
	v.State, v.Stale = rr.verify(rec)
	rr.render(w, recordsPage{Project: rr.Project, One: &v, Checked: rr.now().Format("15:04")})
}

// less orders identifiers by role then serial, which is how every listing here
// sorts.
func less(a, b string) bool {
	ia, erra := ident.Parse(a)
	ib, errb := ident.Parse(b)
	if erra != nil || errb != nil {
		return a < b
	}
	return ident.Less(ia, ib)
}

func (rr *Records) render(w http.ResponseWriter, p recordsPage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := recordsTmpl.Execute(w, p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var recordsTmpl = template.Must(template.New("records").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mustur — records</title>
<style>
  :root { color-scheme: light dark; --edge: #8884; --accent: #6a8fd8;
          --accent-soft: #6a8fd820; }
  body { font: 17px/1.5 system-ui, sans-serif; margin: 0; padding: 0 0 1rem;
         max-width: 46rem; margin-inline: auto; }
  header { display: flex; align-items: baseline; gap: .6rem; padding: .75rem 1rem;
           border-bottom: 1.4px solid var(--edge); }
  header strong { font-size: 1rem; }
  header .who { margin-left: auto; opacity: .6; font-size: .82em; }
  /* The counts are the only navigation, which is the decision this page rests
     on: no tree, no filter, no search box. */
  .counts { display: flex; gap: .4rem; flex-wrap: wrap; padding: .6rem 1rem;
            border-bottom: 1.4px solid var(--edge); font-size: .82em; }
  .counts a { border: 1px solid var(--edge); border-radius: 999px;
              padding: .15rem .6rem; text-decoration: none; color: inherit;
              opacity: .7; }
  .counts a:hover { opacity: 1; border-color: var(--accent); }
  h2 { font-size: .85rem; font-weight: 600; text-transform: uppercase;
       letter-spacing: .04em; opacity: .55; margin: 1.6rem 1rem .3rem; }
  article { padding: .7rem 1rem; border-bottom: 1px solid var(--edge); }
  article .line { display: flex; align-items: baseline; gap: .5rem;
                  flex-wrap: wrap; font-size: .8em; opacity: .65; }
  article .line a { color: inherit; }
  article h3 { font-size: .98rem; font-weight: 600; margin: .15rem 0 .3rem; }
  article p { margin: .3rem 0; font-size: .93em; }
  .fields { font-size: .86em; margin: .4rem 0 0; }
  .fields div { display: flex; gap: .5rem; padding: .1rem 0; }
  .fields .k { opacity: .55; flex: 0 0 9rem; }
  .fields .v { flex: 1; }
  details { margin: .25rem 0; font-size: .88em; }
  summary { cursor: pointer; opacity: .8; }
  details .inner { margin: .3rem 0 .5rem 1rem; padding-left: .6rem;
                   border-left: 2px solid var(--edge); }
  .cites { display: flex; gap: .35rem; flex-wrap: wrap; margin-top: .4rem; }
  .badge { font-size: .78em; border: 1px solid var(--edge); border-radius: 999px;
           padding: .05rem .5rem; opacity: .75; }
  .badge.stale { border-color: #c2703a; opacity: 1; }
  .none { opacity: .6; padding: 2rem 1rem; text-align: center; }
  nav { display: flex; border-top: 1.4px solid var(--edge); margin-top: 1.5rem; }
  nav a { flex: 1; padding: .7rem .25rem; text-align: center; font-size: .85em;
          text-decoration: none; color: inherit; opacity: .6; }
  nav a.here { opacity: 1; font-weight: 600; }
</style>
</head>
<body>
<header><strong>{{if .One}}{{.One.ID}}{{else}}Records{{end}}</strong>
  {{if .One}}<a href="/records" style="font-size:.82em">all records</a>{{end}}
  <span class="who">{{.Project}}</span></header>

{{if .Missing}}
<p class="none">No record called {{.Missing}}.<br>
<small>An identifier that is not here is either a typo or a citation to something never written.</small></p>
{{else if .One}}
{{template "record" .One}}
{{else}}
<div class="counts">
  {{range .Kinds}}<a href="#{{.Kind}}">{{.Count}} {{.Label}}</a>{{end}}
</div>
{{range .Kinds}}
<h2 id="{{.Kind}}">{{.Heading}}</h2>
{{range .Records}}{{template "record" .}}{{end}}
{{end}}
{{end}}

<nav>
  <a href="/records" class="here">Records</a>
  <a href="/questions">Decisions</a>
  <a href="/intake">Intake</a>
  <a href="/account">Account</a>
</nav>
</body>
</html>

{{define "record"}}
<article id="{{.ID}}">
  <div class="line">
    <a href="/records/{{.ID}}">{{.ID}}</a>
    <span>{{.Kind}}</span>
    <span>{{.At}}</span>
    {{if .State}}<span class="badge{{if .Stale}} stale{{end}}">{{.State}}</span>{{end}}
  </div>
  <h3>{{.Title}}</h3>
  {{if .Body}}<p>{{.Body}}</p>{{end}}
  {{if .Data}}<div class="fields">
    {{range .Data}}<div><span class="k">{{.Key}}</span><span class="v">{{.Value}}</span></div>{{end}}
  </div>{{end}}
  {{range .Refs}}{{if .Plain}}<div class="fields"><div><span class="k">{{.Key}}</span><span class="v">{{.ID}}</span></div></div>{{else}}<details>
    <summary>{{if .Key}}{{.Key}}: {{end}}{{.ID}}{{if .Known}} · {{.Kind}}{{end}}</summary>
    <div class="inner">{{if .Known}}<strong>{{.Title}}</strong><br><small>{{.At}} · <a href="/records/{{.ID}}">open on its own</a></small>{{else}}Nothing in the store has this identifier.{{end}}</div>
  </details>{{end}}{{end}}
  {{if .Cites}}<div class="cites">
    {{range .Cites}}<details>
      <summary class="badge">{{.ID}}</summary>
      <div class="inner">{{if .Known}}<strong>{{.Title}}</strong><br><small>{{.Kind}} · {{.At}} · <a href="/records/{{.ID}}">open on its own</a></small>{{else}}Nothing in the store has this identifier.{{end}}</div>
    </details>{{end}}
  </div>{{end}}
</article>
{{end}}
`))
