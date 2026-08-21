package web

// The decision queue. Where a blocked agent's question arrives and where the
// owner answers it, from whatever device is to hand.
//
// The interaction that must not fail, from docs/ui-surfaces.md: **an open
// question is visible without hunting for it**, and answering it is one action.
// An agent is blocked until it is answered, so latency here is work stopped —
// which is why the answer box is on the list rather than behind a tap into a
// detail page, and why what each question blocks is shown next to it. The owner
// has to be able to tell a question holding up a milestone from one holding up
// a sentence without opening either.

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/export"
	"github.com/DevOfPie/Mustur/internal/question"
	"github.com/DevOfPie/Mustur/internal/store"
)

// Questions serves the decision queue.
type Questions struct {
	Store   *store.Store
	Project string
	Actor   string
	Now     func() time.Time

	// ExportTo is the tree the store is rendered into after an answer, for the
	// same reason the intake box has one: whoever answers from a phone cannot
	// run `make export`, and an answer that reached the store and not the file
	// the records role is mapped at has only half arrived.
	ExportTo string
}

// Routes registers the queue on an existing mux.
func (q *Questions) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /questions", q.show)
	mux.HandleFunc("POST /questions", q.answer)
}

func (q *Questions) now() time.Time {
	if q.Now != nil {
		return q.Now()
	}
	return time.Now()
}

// actor is who answered. Cloudflare Access puts the authenticated identity in a
// header at the edge; the record says who it was either way rather than
// guessing.
func (q *Questions) actor(r *http.Request) string {
	if who := r.Header.Get("Cf-Access-Authenticated-User-Email"); who != "" {
		return who
	}
	return q.Actor
}

type queued struct {
	ID       string
	Title    string
	Body     string
	Blocks   string
	Raised   string
	Surfaced bool
}

type queuePage struct {
	Project  string
	Open     []queued
	Answered string
	Error    string
}

func (q *Questions) open(ctx context.Context) ([]queued, error) {
	records, err := q.Store.List(ctx, "")
	if err != nil {
		return nil, err
	}
	var out []queued
	for _, r := range question.Open(records) {
		item := queued{
			ID:       r.ID,
			Title:    r.Title,
			Body:     strings.TrimSpace(r.Body),
			Raised:   r.At,
			Surfaced: question.Surfaced(r),
		}
		if b, ok := r.Get(question.FieldBlocks); ok {
			item.Blocks = strings.TrimSpace(b)
		}
		out = append(out, item)
	}
	return out, nil
}

func (q *Questions) show(w http.ResponseWriter, r *http.Request) {
	openQs, err := q.open(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := queuePage{
		Project:  q.Project,
		Open:     openQs,
		Answered: r.URL.Query().Get("answered"),
		Error:    r.URL.Query().Get("error"),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := queueTmpl.Execute(w, page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// answer records what the owner said and redirects — post/redirect/get, so a
// phone reloading after a dropped connection does not answer twice.
func (q *Questions) answer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		q.redirect(w, r, "", "that did not arrive as a form")
		return
	}
	id := strings.TrimSpace(r.PostFormValue("id"))
	text := strings.TrimSpace(r.PostFormValue("answer"))
	withdraw := r.PostFormValue("withdraw") != ""
	if id == "" {
		q.redirect(w, r, "", "no question named")
		return
	}
	if text == "" && !withdraw {
		q.redirect(w, r, "", "an empty answer is not an answer")
		return
	}

	ctx := r.Context()
	rec, err := q.Store.Get(ctx, id)
	if err != nil {
		q.redirect(w, r, "", err.Error())
		return
	}
	if rec.Kind != question.Kind {
		q.redirect(w, r, "", fmt.Sprintf("%s is a %s, not a question", rec.ID, rec.Kind))
		return
	}
	if !question.IsOpen(rec) {
		// Not an error worth a scary message: the likely cause is two devices,
		// or a reload that outran the redirect.
		q.redirect(w, r, rec.ID, "")
		return
	}

	at := q.now().Format("2006-01-02 15:04")
	if withdraw {
		question.Withdraw(&rec, at)
	} else {
		question.Answer(&rec, text, at)
	}
	if err := q.Store.Append(ctx, rec, "amend", q.actor(r)); err != nil {
		q.redirect(w, r, "", err.Error())
		return
	}
	q.export(ctx)
	q.redirect(w, r, rec.ID, "")
}

// export renders the store after an answer. A failure is not surfaced to the
// answerer: the answer is in the store, which is the record, and the export is
// a rendering of it that the next run will fix.
func (q *Questions) export(ctx context.Context) {
	if q.ExportTo == "" {
		return
	}
	records, err := q.Store.List(ctx, "")
	if err != nil {
		return
	}
	_ = export.Write(q.ExportTo, records)
}

func (q *Questions) redirect(w http.ResponseWriter, r *http.Request, answered, problem string) {
	u := "/questions"
	switch {
	case problem != "":
		u += "?error=" + template.URLQueryEscaper(problem)
	case answered != "":
		u += "?answered=" + template.URLQueryEscaper(answered)
	}
	http.Redirect(w, r, u, http.StatusSeeOther)
}

// OpenCount is how many questions are waiting, for the banner the intake box
// carries. A queue nobody is looking at is the failure this milestone names, so
// the count travels to whatever surface the owner did open.
func OpenCount(ctx context.Context, s *store.Store) int {
	records, err := s.List(ctx, "")
	if err != nil {
		return 0
	}
	return len(question.Open(records))
}

// Same rules as the intake box: no stylesheet, no script, no font, no image.
// Everything a phone off the home network has to fetch is another thing between
// the owner and an answer, and an unanswered question is an agent stopped.
var queueTmpl = template.Must(template.New("questions").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mustur — decisions</title>
<style>
  :root { color-scheme: light dark; --edge: #8884; }
  body { font: 17px/1.5 system-ui, sans-serif; margin: 0; padding: 1rem;
         max-width: 40rem; margin-inline: auto; }
  h1 { font-size: 1rem; font-weight: 600; margin: 0 0 .75rem; opacity: .7; }
  .said { margin: .9rem 0; padding: .6rem .8rem; border-left: 3px solid var(--edge); }
  .q { border-top: 1px solid var(--edge); padding: 1rem 0; }
  .q h2 { font-size: 1.05rem; font-weight: 600; margin: 0 0 .35rem; }
  .ctx { opacity: .75; font-size: .92em; margin: 0 0 .5rem; }
  /* What it blocks sits above the answer box, not below it: the owner decides
     how much care a question deserves before they start typing, not after. */
  .blocks { font-size: .85em; opacity: .8; margin: 0 0 .5rem; }
  .id { font-size: .8em; opacity: .55; }
  input[type=text] { width: 100%; font: inherit; padding: .6rem;
             border: 1px solid var(--edge); border-radius: .5rem;
             background: transparent; color: inherit; box-sizing: border-box; }
  button { font: inherit; padding: .6rem 1.2rem; margin-top: .5rem;
           border: 1px solid var(--edge); border-radius: .5rem;
           background: transparent; color: inherit; }
  button.wide { width: 100%; }
  .drop { opacity: .6; font-size: .85em; margin-top: .4rem; }
  .none { opacity: .6; margin-top: 1.5rem; }
  .unsurfaced { font-size: .8em; opacity: .7; }
  nav { margin-top: 2rem; font-size: .9em; opacity: .7; }
</style>
</head>
<body>
<h1>Mustur — {{.Project}} — decisions</h1>
{{if .Error}}<p class="said">{{.Error}}</p>{{end}}
{{if .Answered}}<p class="said">Answered <code>{{.Answered}}</code>.</p>{{end}}
{{if .Open}}
{{range .Open}}
<div class="q">
  <h2>{{.Title}}</h2>
  {{if .Body}}<p class="ctx">{{.Body}}</p>{{end}}
  {{if .Blocks}}<p class="blocks">Blocks: {{.Blocks}}</p>{{end}}
  <form method="post" action="/questions">
    <input type="hidden" name="id" value="{{.ID}}">
    <input type="text" name="answer" placeholder="Your answer" autocomplete="off">
    <button class="wide" type="submit">Answer</button>
    <div class="drop">
      <span class="id">{{.ID}} · raised {{.Raised}}{{if not .Surfaced}} · <span class="unsurfaced">never surfaced as a prompt</span>{{end}}</span>
      <button type="submit" name="withdraw" value="1">Withdraw</button>
    </div>
  </form>
</div>
{{end}}
{{else}}
<p class="none">Nothing waiting on you.</p>
{{end}}
<nav><a href="/intake">Intake</a></nav>
</body>
</html>
`))
