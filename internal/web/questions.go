package web

// The decision queue, built from its artboard in
// https://plan.agent-native.com/plans/plan-4827b50a72674a22 rather than from
// the brief in docs/ui-surfaces.md. The first version was written from the
// brief, which is the route intake took and the thing publishing the plan was
// meant to stop; the owner's answer on MUS-Q-0010 was to rebuild it from the
// drawing.
//
// What the drawing settles, and the brief did not:
//
//   - **What is blocked comes first**, above the question, because that is what
//     tells a milestone-stopping question from a sentence-stopping one.
//   - **Answers are options**, not a text box. A well-put decision arrives as a
//     short list with what each one costs; a box made the owner reconstruct the
//     options the asker already had. One may be marked recommended.
//   - **Each option's detail expands in place** — one line up front, the
//     paragraph behind it only when asked. That is MUS-D-0017's rule about what
//     detail costs, applied to a surface rather than to a session. (An earlier
//     version of this comment cited MUS-D-0043, which is "The audit is a page"
//     and says nothing about detail. The citation was wrong and is corrected
//     here rather than quietly dropped, because a `.go` comment is the one
//     place check-links cannot see a wrong identifier.)
//   - **Selection is the row.** Choosing an option is one tap; the disclosure
//     is a separate control beneath it, so an option with no detail is still
//     pickable and still looks pickable.
//
// The expansion is a <details> element, so it costs no script — the constraint
// every surface inherits survives the redesign intact.
//
// Where this departs from the drawing, and why, is listed in
// docs/ui-surfaces.md under surface 4. The list is longer than the first
// version of that file admitted.

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/export"
	"github.com/DevOfPie/Mustur/internal/question"
	"github.com/DevOfPie/Mustur/internal/session"
	"github.com/DevOfPie/Mustur/internal/store"
)

// Questions serves the decision queue.
type Questions struct {
	Store   *store.Store
	Project string
	Actor   string
	Now     func() time.Time

	// ShowSessions says whether this server serves the session surface, which
	// decides whether the bar offers a tab to it. Off by default: the session
	// surface carries a composer that types into a running agent's stdin, so
	// publishing it is a deliberate act rather than a consequence of deploying
	// something else that happens to be in the same binary.
	ShowSessions bool

	// ExportTo is the tree the store is rendered into after an answer, for the
	// same reason the intake box has one: whoever answers from a phone cannot
	// run `make export`, and an answer that reached the store and not the file
	// the records role is mapped at has only half arrived.
	ExportTo string

	// Sessions carries an answer back into the session that raised it. Nil
	// means answers are recorded and not delivered, which is what milestone 3
	// shipped and what a Mustur running somewhere without tmux still does.
	Sessions session.Sender

	// DeliverTimeout bounds one delivery. Zero means session.DeliverTimeout.
	DeliverTimeout time.Duration
}

func (q *Questions) deliverTimeout() time.Duration {
	if q.DeliverTimeout > 0 {
		return q.DeliverTimeout
	}
	return session.DeliverTimeout
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

type queuedOption struct {
	Label       string
	Line        string
	Detail      string
	Recommended bool
}

type queued struct {
	ID       string
	Title    string
	Body     string
	Blocks   string
	Asked    string
	Needed   bool
	Surfaced bool
	Options  []queuedOption
}

type queuePage struct {
	Project  string
	Open     []queued
	OpenN    int
	Answered string
	Error    string
	// ShowSessions renders the Sessions tab. See the note on intake's page.
	ShowSessions bool
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
			Asked:    r.At,
			Needed:   question.Needed(r),
			Surfaced: question.Surfaced(r),
		}
		if b, ok := r.Get(question.FieldBlocks); ok {
			item.Blocks = strings.TrimSpace(b)
		}
		for _, o := range question.Options(r) {
			item.Options = append(item.Options, queuedOption{
				Label: o.Label, Line: o.Line, Detail: o.Detail,
				Recommended: o.IsRecommended(),
			})
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
		ShowSessions: q.ShowSessions,
		Project:      q.Project,
		Open:         openQs,
		OpenN:        len(openQs),
		Answered:     r.URL.Query().Get("answered"),
		Error:        r.URL.Query().Get("error"),
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
	withdraw := r.PostFormValue("withdraw") != ""
	// A chosen option answers the question; free text answers one that offered
	// none, and overrides a choice when the owner wants to say something else.
	text := strings.TrimSpace(r.PostFormValue("answer"))
	if text == "" {
		text = strings.TrimSpace(r.PostFormValue("option"))
	}
	if id == "" {
		q.redirect(w, r, "", "no question named")
		return
	}
	if text == "" && !withdraw {
		q.redirect(w, r, "", "pick an option or write an answer")
		return
	}

	// Detached from the request. A review reproduced the alternative: with the
	// request's own context, a phone that dropped the connection while tmux was
	// being shelled out to cancelled the write as well, and the answer was lost
	// entirely — still `open`, no Answer, no reason. An answer that reached the
	// server is the owner's, and a link that died afterwards must not unmake it.
	// The timeout is the delivery's, not the answer's; see below.
	ctx := context.WithoutCancel(r.Context())
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
		// Carried back before the record is written, so what happened to the
		// delivery is part of the same event rather than a second one that
		// could fail on its own.
		//
		// Bounded, because an unresponsive tmux would otherwise hold the answer
		// unwritten for as long as it liked. On timeout the delivery is what
		// fails; the answer is written with the reason.
		if q.Sessions != nil {
			dctx, cancel := context.WithTimeout(ctx, q.deliverTimeout())
			question.Set(&rec, question.FieldDelivered,
				session.Deliver(dctx, q.Sessions, question.ProjectOf(rec), rec.ID, text))
			cancel()
		}
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
// carries. The tab's own count comes from queuePage.OpenN, which the queue
// already has in hand.
func OpenCount(ctx context.Context, s *store.Store) int {
	records, err := s.List(ctx, "")
	if err != nil {
		return 0
	}
	return len(question.Open(records))
}

// The tab bar. MUS-D-0041 is owner-set: the phone bar has four tabs, decided
// against a recommendation of three, and it still stands. A tab that goes
// nowhere would be an unbuilt capability described as existing, so the bar
// renders the built ones and grows as the rest arrive. Owner-confirmed as the
// interim on MUS-Q-0012, which also corrected MUS-D-0053's claim that no bar
// exists before milestone 4 — a pointer an earlier edit dropped, leaving the
// superseded decision with nothing pointing at what superseded it.
//
// "Grows as the rest arrive" is a promise that has to be kept in every template
// at once. Sessions arrived at milestone 4b and this bar did not grow, which
// left three surfaces in one binary showing two, two and three tabs. A test
// asserts they match.
//
// The count is spelled out rather than shown as a badge: a badge holding one
// character reads as an unexplained dot at this size. That is the drawing's own
// note, and it applies to the two-tab version exactly as much.
var queueTmpl = template.Must(template.New("questions").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mustur — decisions</title>
<style>
  :root { color-scheme: light dark; --edge: #8884; --accent: #6a8fd8;
          --accent-soft: #6a8fd820; }
  body { font: 17px/1.5 system-ui, sans-serif; margin: 0;
         max-width: 40rem; margin-inline: auto;
         display: flex; flex-direction: column; min-height: 100vh; }
  header { padding: .75rem 1rem; border-bottom: 1.4px solid var(--edge);
           display: flex; align-items: center; gap: .5rem; white-space: nowrap; }
  header strong { font-size: 1rem; }
  header .n { margin-left: auto; opacity: .65; font-size: .85em; }
  main { flex: 1; padding: 1rem; }
  .said { margin: 0 0 .9rem; padding: .6rem .8rem; border-left: 3px solid var(--edge); }
  /* What is blocked comes first, above the question. It is what separates a
     milestone-stopping question from a sentence-stopping one. */
  .pills { display: flex; gap: .4rem; overflow-x: auto; white-space: nowrap;
           margin-bottom: .6rem; }
  .pill { flex: 0 0 auto; border: 1px solid var(--edge); border-radius: 999px;
          padding: .2rem .6rem; font-size: .8em; }
  .pill.accent { border-color: var(--accent); background: var(--accent-soft); }
  h2 { margin: 0 0 .25rem; font-size: 1.15rem; }
  .asked { display: block; opacity: .6; font-size: .82em; margin-bottom: .8rem; }
  .ctx { margin: 0 0 1rem; opacity: .85; font-size: .95em; }
  /* One line up front, the paragraph behind it only when asked for. <details>
     does that with no script, which keeps the constraint every surface
     inherits. */
  /* Selection is the whole row, so choosing is one tap. An earlier build hid
     the radio inside the disclosure, which made an option with no detail
     unpickable without a blind tap on a row that showed no sign of opening. */
  .opt { border: 1px solid var(--edge); border-radius: .5rem; padding: .1rem .7rem;
         margin-bottom: .6rem; }
  .opt:has(input:checked) { border-color: var(--accent); background: var(--accent-soft); }
  .pick { display: flex; gap: .6rem; align-items: flex-start; padding: .7rem 0;
          cursor: pointer; }
  .opt .lbl { flex: 1; min-width: 0; }
  .opt .line { display: block; opacity: .7; font-size: .85em; }
  .rec { font-size: .72em; text-transform: uppercase; letter-spacing: .04em;
         border: 1px solid var(--accent); border-radius: 999px;
         padding: .05rem .45rem; vertical-align: .1em; white-space: nowrap; }
  .opt details { border-top: 1px solid var(--edge); }
  .opt details > summary { cursor: pointer; padding: .5rem 0; opacity: .6;
                           font-size: .85em; }
  .opt details p { margin: 0 0 .7rem; font-size: .92em; }
  input[type=text] { width: 100%; font: inherit; padding: .6rem;
             border: 1px solid var(--edge); border-radius: .5rem;
             background: transparent; color: inherit; box-sizing: border-box; }
  button { font: inherit; padding: .65rem 1.2rem; border: 1px solid var(--edge);
           border-radius: .5rem; background: transparent; color: inherit; }
  /* Answering is one tap, above the bar rather than inside it. */
  button.primary { width: 100%; margin-top: .8rem; border-color: var(--accent);
                   background: var(--accent-soft); }
  .drop { display: flex; align-items: center; gap: .6rem; margin-top: .6rem; }
  .id { opacity: .5; font-size: .78em; }
  .drop button { font-size: .85em; padding: .35rem .8rem; margin-left: auto; }
  .none { opacity: .6; padding: 2rem 0; text-align: center; }
  hr { border: 0; border-top: 1.4px solid var(--edge); margin: 1.6rem 0; }
  nav { display: flex; border-top: 1.4px solid var(--edge); white-space: nowrap; }
  nav a { flex: 1; padding: .7rem .25rem; text-align: center; font-size: .85em;
          text-decoration: none; color: inherit; opacity: .6; }
  nav a.here { opacity: 1; font-weight: 600; }
</style>
</head>
<body>
<header><strong>Decisions</strong><span class="n">{{if .OpenN}}{{.OpenN}} open{{else}}nothing open{{end}}</span></header>
<main>
{{if .Error}}<p class="said">{{.Error}}</p>{{end}}
{{if .Answered}}<p class="said">Answered <code>{{.Answered}}</code>.</p>{{end}}
{{if .Open}}
{{range $i, $q := .Open}}
{{if $i}}<hr>{{end}}
<form method="post" action="/questions">
  <input type="hidden" name="id" value="{{$q.ID}}">
  <div class="pills">
    {{if $q.Blocks}}<span class="pill accent">blocks {{$q.Blocks}}</span>{{end}}
    {{if $q.Needed}}<span class="pill">answer needed to proceed</span>{{end}}
    {{if not $q.Surfaced}}<span class="pill">never surfaced</span>{{end}}
  </div>
  <h2>{{$q.Title}}</h2>
  <small class="asked">Asked {{$q.Asked}}</small>
  {{if $q.Body}}<p class="ctx">{{$q.Body}}</p>{{end}}
  {{range $q.Options}}
  <div class="opt">
    <label class="pick">
      <input type="radio" name="option" value="{{.Label}}">
      <span class="lbl"><strong>{{.Label}}</strong>{{if .Recommended}} <span class="rec">recommended</span>{{end}}
        {{if .Line}}<span class="line">{{.Line}}</span>{{end}}</span>
    </label>
    {{if .Detail}}<details><summary>more</summary><p>{{.Detail}}</p></details>{{end}}
  </div>
  {{end}}
  <input type="text" name="answer" autocomplete="off"
         placeholder="{{if $q.Options}}Or say something else{{else}}Your answer{{end}}">
  <button class="primary" type="submit">Answer</button>
  <div class="drop">
    <span class="id">{{$q.ID}}</span>
    <button type="submit" name="withdraw" value="1">Withdraw</button>
  </div>
</form>
{{end}}
{{else}}
<p class="none">Nothing waiting on you.</p>
{{end}}
</main>
<nav>
  {{if .ShowSessions}}<a href="/sessions">Sessions</a>{{end}}
  <a href="/questions" class="here">Decisions{{if .OpenN}} · {{.OpenN}}{{end}}</a>
  <a href="/intake">Intake</a>
</nav>
</body>
</html>
`))
