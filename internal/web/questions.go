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
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DevOfPie/Mustur/internal/export"
	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/intake"
	"github.com/DevOfPie/Mustur/internal/question"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/session"
	"github.com/DevOfPie/Mustur/internal/store"
)

// Questions serves the decision queue.
type Questions struct {
	counts countCache

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

	// Roles answers whether the viewer owns a held jot's destination project,
	// which decides whether they see it, count it, and may approve it
	// (MUS-D-0189). Nil without accounts, where nobody holds anything.
	Roles Roles
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
	mux.HandleFunc("GET /questions/count", q.count)
	// A reader's held jots, approved or discarded from this surface (MUS-D-0189).
	// Under /intake because that is where they came from; served here because
	// Decisions is where an owner meets what is waiting on them (MUS-Q-0143).
	mux.HandleFunc("POST /intake/held/{id}/approve", q.approveHeld)
	mux.HandleFunc("POST /intake/held/{id}/discard", q.discardHeld)
}

// A heldCard is one reader's jot as an owner reviews it.
type heldCard struct {
	ID   string
	By   string
	When string
	Text string
	// To is the reader's chosen destination, and empty for "Route it for me",
	// which is what the select starts on.
	To string
	// ToName says where the reader pointed it, in words.
	ToName string
	// Guess is where "Route it for me" lands today, so the owner can see it
	// before pressing Approve rather than after.
	Guess string
}

// held builds the cards this viewer may act on, and the destinations each
// card's select offers.
func (q *Questions) held(r *http.Request) ([]heldCard, []destGroup) {
	jots, dests := approvable(r, q.Store, q.Roles, q.Project)
	if len(jots) == 0 {
		return nil, nil
	}
	routing, err := intake.Destinations(r.Context(), q.Store)
	if err != nil {
		return nil, nil
	}
	all := make([]destination, 0, len(routing))
	names := map[string]string{}
	for _, rr := range routing {
		all = append(all, destination{ID: rr.ID, Name: rr.Title, Kind: rr.Kind})
		names[rr.ID] = rr.Title
	}
	cards := make([]heldCard, 0, len(jots))
	for i, h := range jots {
		c := heldCard{ID: h.ID, By: h.Email, When: pacific(h.Created), Text: h.Text, To: h.To,
			ToName: "Route it for me"}
		if c.By == "" {
			c.By = "an account that no longer exists"
		}
		if h.To != "" {
			c.ToName = names[h.To]
			if c.ToName == "" {
				c.ToName = h.To + ", which no longer exists"
			}
		} else {
			c.Guess = dests[i].Name
		}
		cards = append(cards, c)
	}
	return cards, grouped(all)
}

// approveHeld files a held jot through the one filing path, with the reader as
// filer and the owner who pressed Approve recorded beside them (MUS-D-0189).
//
// Only an owner of the project it is filed under may do it. That is checked
// against the destination this press names — the owner may have changed the
// select — resolved the way File will resolve it, so the check and the filing
// cannot disagree about where the jot is going.
func (q *Questions) approveHeld(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		q.redirect(w, r, "", "that did not arrive as a form")
		return
	}
	// Detached for the reason an answer is: a phone that drops the connection
	// after the press must not leave the jot claimed and unfiled.
	ctx := context.WithoutCancel(r.Context())
	id := r.PathValue("id")
	to := strings.TrimSpace(r.PostFormValue("to"))
	h, err := q.Store.HeldJot(ctx, id)
	if err != nil {
		q.redirect(w, r, "", err.Error())
		return
	}
	if to == scratchTo {
		q.redirect(w, r, "", "scratch files nothing; Discard is how a held jot is let go")
		return
	}
	if _, project := heldProject(ctx, q.Store, h.Text, to, q.Project); !mayApprove(r, q.Roles, project, q.Project) {
		http.Error(w, "only an owner of "+project+" may approve a jot filed there", http.StatusForbidden)
		return
	}
	filer := h.Email
	if filer == "" {
		filer = h.AccountID
	}
	approver := q.actor(r)
	if viewer, ok := Viewer(r); ok {
		approver = viewer.Email
	}
	var rec record.Record
	var dest intake.Destination
	err = q.Store.Approve(ctx, id, func(got store.Held) error {
		var ferr error
		rec, dest, ferr = intake.File(ctx, q.Store, intake.Request{
			Project: q.Project, Text: got.Text, Actor: filer, To: to,
			Now: q.now(), Approved: approver,
		})
		return ferr
	})
	if err != nil {
		q.redirect(w, r, "", err.Error())
		return
	}
	q.export(ctx)
	u := "/questions?filed=" + template.URLQueryEscaper(rec.ID)
	if dest.Name != "" {
		u += "&filedto=" + template.URLQueryEscaper(dest.Name)
	}
	http.Redirect(w, r, u, http.StatusSeeOther)
}

// discardHeld removes a held jot and leaves nothing (MUS-Q-0143).
//
// The same owner rule as approving, checked against where the reader pointed
// it: the person who may let a jot into a project is the person who may keep
// it out.
func (q *Questions) discardHeld(w http.ResponseWriter, r *http.Request) {
	ctx := context.WithoutCancel(r.Context())
	id := r.PathValue("id")
	h, err := q.Store.HeldJot(ctx, id)
	if err != nil {
		q.redirect(w, r, "", err.Error())
		return
	}
	if _, project := heldProject(ctx, q.Store, h.Text, h.To, q.Project); !mayApprove(r, q.Roles, project, q.Project) {
		http.Error(w, "only an owner of "+project+" may discard a jot sent there", http.StatusForbidden)
		return
	}
	if err := q.Store.Discard(ctx, id); err != nil {
		q.redirect(w, r, "", err.Error())
		return
	}
	http.Redirect(w, r, "/questions?discarded=1", http.StatusSeeOther)
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
	Label string
	Line  string
	// Detail is rendered as markdown like the body, but the export flattens an
	// option into one table cell, so only inline markdown survives there: a
	// table or a second paragraph written here would not reach the record.
	Detail      template.HTML
	Recommended bool
}

type queued struct {
	ID string
	// Project is whose question this is, named the way the account page names
	// one: "Mustur (MUS)", or the bare prefix when no project record claims it.
	// The artboard drew it as a pill and the first build dropped it while one
	// project existed; the store now holds several, and the owner could not
	// tell whose a decision was without reading the faded identifier at the
	// foot of the card (MUS-F-0150).
	Project  string
	Title    string
	Body     template.HTML
	Blocks   string
	Asked    string
	Needed   bool
	Surfaced bool
	Options  []queuedOption
	// Cites are the records the question's text names, each expanding in
	// place the way a citation does on Records (MUS-D-0198, which extends
	// MUS-D-0040 to this surface on the owner's answer to MUS-Q-0160). The
	// owner met MUS-F-0164 and MUS-F-0027 in a question and its options and
	// had no way to them but typing the address (MUS-F-0168).
	Cites []citation
}

type queuePage struct {
	Project string
	Open    []queued
	OpenN   int
	// Attention is the Records badge (MUS-D-0193); bar.js keeps it true.
	Attention int
	Answered  string
	// What became of the answer once it was written: the same sentence the
	// record keeps, shown to the person who just answered (MUS-F-0070). An
	// answer to a question that named no session is recorded and delivered
	// nowhere, which is correct and was invisible -- the owner answered, waited
	// for a session to resume, and nothing had ever been going to type into
	// one.
	Delivered string
	Error     string
	// ShowSessions renders the Sessions tab. See the note on intake's page.
	ShowSessions bool
	ShowAccount  bool

	// Held is readers' jots this viewer may approve, above the questions
	// (MUS-Q-0143), and HeldGroups the destinations each card's select offers.
	Held       []heldCard
	HeldGroups []destGroup
	// Waiting is the badge: open questions and held jots together.
	Waiting int
	// Filed and FiledTo say a held jot was just approved; Discarded that one
	// was let go.
	Filed     string
	FiledTo   string
	Discarded bool
}

func (q *Questions) open(ctx context.Context) ([]queued, error) {
	records, err := q.Store.List(ctx, "")
	if err != nil {
		return nil, err
	}
	// The project records are already in this listing, so naming every card's
	// project costs no query beyond the one above, rather than one per question.
	names := projectNamesIn(records)
	// Which identifiers resolve is read from the same listing.
	by := make(map[string]record.Record, len(records))
	for _, r := range records {
		by[r.ID] = r
	}
	retired := record.Retire(records)
	var out []queued
	for _, r := range question.Open(records) {
		plain := retired.PlainIn(r.ID)
		item := queued{
			ID:       r.ID,
			Title:    r.Title,
			Body:     markdownPlain(strings.TrimSpace(r.Body), plain),
			Asked:    r.At,
			Needed:   question.Needed(r),
			Surfaced: question.Surfaced(r),
		}
		// The prefix is the routing (MUS-D-0125), so the identifier is the
		// authority on whose question it is. One that does not parse gets no
		// pill rather than a guess.
		if id, err := ident.Parse(r.ID); err == nil {
			item.Project = names.name(id.Project)
		}
		if b, ok := r.Get(question.FieldBlocks); ok {
			item.Blocks = strings.TrimSpace(b)
		}
		for _, o := range question.Options(r) {
			item.Options = append(item.Options, queuedOption{
				Label: o.Label, Line: o.Says(), Detail: markdownPlain(o.Detail, plain),
				Recommended: o.IsRecommended(),
			})
		}
		item.Cites = queueCites(r, item.Blocks, by, plain)
		out = append(out, item)
	}
	return out, nil
}

// queueCites gathers the records a question names, in the order it names them:
// title, what it blocks, the body, then every option's label, line and detail.
// Deduplicated and never the question itself, as on Records.
//
// Every identifier on the card is gathered, the option's label and line
// included, but the row sits outside every option: a <details> inside an
// option's <label> would take the tap meant to choose it, and the whole row is
// the control (docs/ui-surfaces.md, surface 4). The text itself stays text
// everywhere, because a <details> is not allowed inside a paragraph and the
// browser ends the paragraph where one starts.
//
// Only what the store holds is listed. An identifier it does not hold stays
// text in the question and gets no entry, rather than an entry saying there is
// nothing behind it. Nor does one retired where this question stands
// (MUS-D-0197): it is plain text here, as it is on the Records page.
func queueCites(r record.Record, blocks string, by map[string]record.Record, plain map[string]bool) []citation {
	text := r.Title + " " + blocks + " " + r.Body
	for _, o := range question.Options(r) {
		text += " " + o.Label + " " + o.Says() + " " + o.Detail
	}
	seen := map[string]bool{r.ID: true}
	var out []citation
	for _, id := range idInProse(text) {
		if seen[id] || plain[id] {
			continue
		}
		seen[id] = true
		if _, ok := by[id]; ok {
			out = append(out, resolve("", id, by))
		}
	}
	return out
}

func (q *Questions) show(w http.ResponseWriter, r *http.Request) {
	openQs, err := q.open(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := queuePage{
		ShowSessions: q.ShowSessions && CanWrite(r),
		ShowAccount:  q.ShowAccount,
		Project:      q.Project,
		Open:         openQs,
		OpenN:        len(openQs),
		Attention:    intake.AttentionCount(r.Context(), q.Store),
		Answered:     r.URL.Query().Get("answered"),
		Delivered:    r.URL.Query().Get("sent"),
		Error:        r.URL.Query().Get("error"),
		Filed:        r.URL.Query().Get("filed"),
		FiledTo:      r.URL.Query().Get("filedto"),
		Discarded:    r.URL.Query().Get("discarded") == "1",
	}
	page.Held, page.HeldGroups = q.held(r)
	page.Waiting = page.OpenN + len(page.Held)
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
	// Withdrawing closes a question with no answer, and the button said only
	// "Withdraw" — the owner pressed it not knowing what it did and lost
	// MUS-Q-0060 (MUS-F-0077). The tick beside it is what says so, and it is a
	// checkbox rather than a confirmation page because this surface has no
	// script and a second page would be a second surface.
	if withdraw && r.PostFormValue("sure") == "" {
		q.redirect(w, r, "", "Withdraw closes a question with no answer. Tick the box beside it if that is what you meant.")
		return
	}
	// A chosen option answers the question. Free text beside it is a note on
	// that choice; free text with no choice is the answer itself, which is
	// MUS-D-0055's case for what the list does not contain.
	//
	// It used to override the choice, so picking an option and adding a remark
	// meant retyping the option's label into the box -- and the record then
	// said only what was typed, never which option it named (MUS-F-0071).
	text := strings.TrimSpace(r.PostFormValue("option"))
	note := strings.TrimSpace(r.PostFormValue("answer"))
	if text == "" {
		text, note = note, ""
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
	var sent string
	if withdraw {
		question.Withdraw(&rec, at)
	} else {
		question.AnswerWithNote(&rec, text, note, at)
		// Carried back before the record is written, so what happened to the
		// delivery is part of the same event rather than a second one that
		// could fail on its own.
		//
		// Bounded, because an unresponsive tmux would otherwise hold the answer
		// unwritten for as long as it liked. On timeout the delivery is what
		// fails; the answer is written with the reason.
		if q.Sessions != nil {
			dctx, cancel := context.WithTimeout(ctx, q.deliverTimeout())
			// The note travels with the choice: an agent told only the label
			// would act on an option the owner qualified.
			sent = session.Deliver(dctx, q.Sessions, question.ProjectOf(rec), rec.ID,
				question.Said(text, note))
			question.Set(&rec, question.FieldDelivered, sent)
			cancel()
		}
	}
	if err := q.Store.Append(ctx, rec, "amend", q.actor(r)); err != nil {
		q.redirect(w, r, "", err.Error())
		return
	}
	q.export(ctx)
	q.redirectSent(w, r, rec.ID, sent, "")
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
	q.redirectSent(w, r, answered, "", problem)
}

// redirectSent carries the delivery's own sentence back to the page, so the
// person who answered learns what happened to the answer at the moment they
// answered rather than by opening the record later.
func (q *Questions) redirectSent(w http.ResponseWriter, r *http.Request, answered, sent, problem string) {
	u := "/questions"
	switch {
	case problem != "":
		u += "?error=" + template.URLQueryEscaper(problem)
	case answered != "":
		u += "?answered=" + template.URLQueryEscaper(answered)
		if sent != "" {
			u += "&sent=" + template.URLQueryEscaper(sent)
		}
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
  /* MUS-Q-0052: the account surface is reached from here rather than from
     a fifth tab, so MUS-D-0041's four stand. Rendered only when the server
     actually serves it — a link that goes nowhere is the failure the bar
     itself was written to avoid. */
  .acct { font-size: .82em; opacity: .6; text-decoration: none;
          color: inherit; margin-left: auto; }
  header { display: flex; align-items: baseline; gap: .6rem;
           padding: .75rem 1rem; border-bottom: 1.4px solid var(--edge);
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
  .ctx p { margin: 0 0 .6rem; }
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
  /* The recommendation is a mark on the option's own row, not a word in its
     description (MUS-F-0072). A star rather than a drawing: the tab icons are
     CSS because the plan tool refuses SVG (MUS-F-0048), and a star is a
     character that needs neither. The word it replaces is still in the record,
     where a reader with no surface can see it -- Says() takes it off here and
     nowhere else. */
  .rec { color: var(--accent); font-size: 1em; line-height: 1;
         vertical-align: .05em; white-space: nowrap; }
  .opt details { border-top: 1px solid var(--edge); }
  .opt details > summary { cursor: pointer; padding: .5rem 0; opacity: .6;
                           font-size: .85em; }
  .opt details .md { margin: 0 0 .7rem; font-size: .92em; }
  .opt details p { margin: 0 0 .5rem; }
  /* A textarea rather than a text input, because Enter in a single-line input
     submits the form: the owner pressed it mid-sentence while writing a note
     and the half they had typed was recorded as the answer (MUS-F-0076). Here
     Enter is a newline and the Answer button is the only way out, which is the
     same lesson MUS-F-0067 taught the session composer arriving at the second
     surface. It grows a little rather than scrolling, because a note about a
     choice is a sentence or two. */
  textarea { width: 100%; font: inherit; padding: .6rem; min-height: 4.2rem;
             border: 1px solid var(--edge); border-radius: .5rem; resize: vertical;
             background: transparent; color: inherit; box-sizing: border-box; }
  button { font: inherit; padding: .65rem 1.2rem; border: 1px solid var(--edge);
           border-radius: .5rem; background: transparent; color: inherit; }
  /* Answering is one tap, above the bar rather than inside it. */
  button.primary { width: 100%; margin-top: .8rem; border-color: var(--accent);
                   background: var(--accent-soft); }
  /* The owner asked for Answer to be unavailable until there is something to
     answer with -- a chosen option, or text in the box (MUS-Q-0071). The three
     options put to them all cost something: making the radios required retires
     MUS-D-0055's clause that text alone can answer, and a script makes this one
     more scripted surface, of which there are eight (MUS-D-0199). Neither is needed. :has() asks the form whether
     anything is checked and :placeholder-shown asks whether the box is empty,
     both live, both CSS.

     Each question is its own form, so this scopes to one question rather than
     to the page. A question with no options has no radio to check, so it turns
     on text alone, which is what it should do.

     It is an affordance, not a guard. pointer-events: none stops a pointer and
     not a keyboard, and a browser without :has() applies none of this and gets
     today's button. The refusal that actually holds is the server's, which has
     always been there and stays. Withdraw is deliberately untouched: closing a
     question with no answer is the one thing that must work when nothing is
     chosen. */
  form:not(.held):not(:has(input[type=radio]:checked)):not(:has(textarea:not(:placeholder-shown)))
    button.primary { opacity: .45; pointer-events: none; }
  .drop { display: flex; align-items: center; gap: .6rem; margin-top: .6rem;
          flex-wrap: wrap; }
  .id { opacity: .5; font-size: .78em; }
  .drop button { font-size: .85em; padding: .35rem .8rem; }
  /* The tick sits between the identifier and the button, and takes the space
     the button used to push itself over with, so the button cannot be reached
     without passing the sentence that says what it does. */
  .sure { margin-left: auto; display: flex; align-items: center; gap: .4rem;
          font-size: .8em; opacity: .75; }
  .none { opacity: .6; padding: 2rem 0; text-align: center; }
  hr { border: 0; border-top: 1.4px solid var(--edge); margin: 1.6rem 0; }
  form > .cites { margin: 0 0 1rem; }
  /* Readers' jots waiting for approval, above the questions (MUS-Q-0143).
     Each is its own card and its own form, stacked rather than laid in a row,
     so the reader, the time and the destination never share a line with a
     control that could overlap them at phone width. The text keeps its line
     breaks and wraps anywhere, so a pasted link cannot push the page sideways. */
  h2.sect { font-size: .95rem; font-weight: 600; opacity: .75; margin: 0 0 .7rem; }
  .held { border: 1px solid var(--edge); border-radius: .5rem; padding: .7rem .8rem;
          margin-bottom: .8rem; display: flex; flex-direction: column; gap: .5rem; }
  .held .meta { opacity: .65; font-size: .82em; overflow-wrap: anywhere; }
  .held .txt { white-space: pre-line; overflow-wrap: anywhere; }
  .held label { display: flex; flex-direction: column; gap: .25rem; font-size: .85em; }
  .held select { font: inherit; font-size: 1rem; padding: .45rem; width: 100%;
                 max-width: 100%; border: 1px solid var(--edge); border-radius: .5rem;
                 background: transparent; color: inherit; }
  .held .acts { display: flex; gap: .6rem; flex-wrap: wrap; }
  .held .acts button { flex: 1 1 auto; }
  .held .acts button.primary { width: auto; margin-top: 0; }
` + markdownCSS + citesCSS + shellCSS + `
</style>
</head>
<body>
<header><strong>Decisions</strong><span class="n">{{if .OpenN}}{{.OpenN}} open{{else}}nothing open{{end}}</span>{{if .ShowAccount}}<a class="acct" href="/account">Account</a>{{end}}</header>
<main>
{{if .Error}}<p class="said">{{.Error}}</p>{{end}}
{{if .Answered}}<p class="said">Answered <code>{{.Answered}}</code>.{{if .Delivered}} {{.Delivered}}.{{end}}</p>{{end}}
{{if .Filed}}<p class="said">Filed <code>{{.Filed}}</code>{{if .FiledTo}} → {{.FiledTo}}{{end}}.</p>{{end}}
{{if .Discarded}}<p class="said">Discarded. Nothing was kept.</p>{{end}}
{{if .Held}}<section aria-labelledby="held-h">
<h2 class="sect" id="held-h">Jots waiting for approval</h2>
{{range .Held}}<form class="held" method="post" action="/intake/held/{{.ID}}/approve">
  <span class="meta">{{.By}} · {{.When}} · {{.ToName}}</span>
  <span class="txt">{{.Text}}</span>
  <label>File to
    <select name="to">
      <option value=""{{if not .To}} selected{{end}}>Route it for me{{if .Guess}} ({{.Guess}}){{end}}</option>
      {{$to := .To}}{{range $.HeldGroups}}<optgroup label="{{.Label}}">
        {{range .Items}}<option value="{{.ID}}"{{if eq .ID $to}} selected{{end}}>{{.Name}}</option>{{end}}
      </optgroup>{{end}}
    </select>
  </label>
  <div class="acts">
    <button class="primary" type="submit">Approve and file</button>
    <button type="submit" formaction="/intake/held/{{.ID}}/discard">Discard</button>
  </div>
</form>
{{end}}</section>
{{if .Open}}<hr><h2 class="sect">Questions</h2>{{end}}
{{end}}
{{if .Open}}
{{range $i, $q := .Open}}
{{if $i}}<hr>{{end}}
<form method="post" action="/questions">
  <input type="hidden" name="id" value="{{$q.ID}}">
  <div class="pills">
    {{if $q.Project}}<span class="pill">{{$q.Project}}</span>{{end}}
    {{if $q.Blocks}}<span class="pill accent">blocks {{$q.Blocks}}</span>{{end}}
    {{if $q.Needed}}<span class="pill">answer needed to proceed</span>{{end}}
    {{if not $q.Surfaced}}<span class="pill">never surfaced</span>{{end}}
  </div>
  <h2>{{$q.Title}}</h2>
  <small class="asked">Asked {{$q.Asked}}</small>
  {{if $q.Body}}<div class="ctx md">{{$q.Body}}</div>{{end}}
  {{template "cites" $q.Cites}}
  {{range $q.Options}}
  <div class="opt">
    <label class="pick">
      <input type="radio" name="option" value="{{.Label}}">
      <span class="lbl"><strong>{{.Label}}</strong>{{if .Recommended}} <span class="rec" title="Recommended" aria-label="Recommended">&#9733;</span>{{end}}
        {{if .Line}}<span class="line">{{.Line}}</span>{{end}}</span>
    </label>
    {{if .Detail}}<details><summary>more</summary><div class="md">{{.Detail}}</div></details>{{end}}
  </div>
  {{end}}
  <textarea name="answer" rows="2" spellcheck="true" autocapitalize="sentences"
            autocomplete="off"
            placeholder="{{if $q.Options}}A note on your choice, or something else entirely{{else}}Your answer{{end}}"></textarea>
  <button class="primary" type="submit">Answer</button>
  <div class="drop">
    <span class="id">{{$q.ID}}</span>
    <label class="sure"><input type="checkbox" name="sure" value="1">close it with no answer</label>
    <button type="submit" name="withdraw" value="1">Withdraw</button>
  </div>
</form>
{{end}}
{{else if not .Held}}
<p class="none">Nothing waiting on you.</p>
{{end}}
</main>
<nav>
  {{if .ShowSessions}}<a href="/sessions" aria-label="Sessions"><i class="ic ic-sess"></i><span>Sessions</span></a>{{end}}
  <a href="/questions" class="here" aria-label="Decisions"><i class="ic ic-dec">?</i><span>Decisions</span>{{if .Waiting}}<em class="cnt">{{.Waiting}}</em>{{end}}</a>
  <a href="/intake" aria-label="Intake"><i class="ic ic-in"><b></b></i><span>Intake</span></a>
  <a href="/records" aria-label="Records"><i class="ic ic-rec"></i><span>Records</span>{{if .Attention}}<em class="cnt att">{{.Attention}}</em>{{end}}</a>
  {{if .ShowAccount}}<a class="me" href="/account" title="Account" aria-label="Account"><i class="ic ic-acc"></i></a>{{end}}
</nav>
<script src="/assets/bar.js"></script>
</body>
</html>
` + citesTmpl))

// countCache holds the answer for a moment so a handful of open tabs polling
// a badge cost one count between them rather than one each. There is one per
// badge, and each is handed the count it holds.
//
// OpenCount and intake.AttentionCount each list every record in the store and
// filter, which is fine once and wasteful per tab per tick. Two seconds is
// short enough that nobody sees a stale number and long enough that a page
// full of tabs is one query.
type countCache struct {
	mu   sync.Mutex
	at   time.Time
	n    int
	have bool
}

// forget drops the held answer, for a handler that has just changed what it
// counts. Without it the page such a handler redirects to renders the new
// count, and bar.js's first poll on load overwrites it with the one held from
// before the change: seen in a browser after Move, where the badge went back
// from 1 to 2.
func (c *countCache) forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.have = false
}

// get answers from the cache or counts. The lock is held across the count on
// purpose: a forget called while a count is in flight waits for it to be
// stored and then drops it, so a count taken before a write is never what a
// poll after the write is answered with. Counting outside the lock would need a
// generation number to keep that true; holding it costs one count's wait for
// a poll that arrives during another, which the two-second window makes rare.
func (c *countCache) get(ctx context.Context, s *store.Store, now func() time.Time, count func(context.Context, *store.Store) int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.have && now().Sub(c.at) < 2*time.Second {
		return c.n
	}
	c.n = count(ctx, s)
	c.at = now()
	c.have = true
	return c.n
}

// count answers the badge's poll. A number and nothing else: it says how many
// decisions are open and never which, so a surface that may not read questions
// still learns that something is waiting without learning what.
func (q *Questions) count(w http.ResponseWriter, r *http.Request) {
	n := 0
	if q.Store != nil {
		n = q.counts.get(r.Context(), q.Store, q.now, OpenCount)
		// Held jots are counted per viewer, outside the shared cache: which
		// ones a person may approve depends on who is asking (MUS-Q-0143).
		// An empty held table costs one indexed read and nothing more.
		held, _ := approvable(r, q.Store, q.Roles, q.Project)
		n += len(held)
	}
	writeCount(w, n)
}

// writeCount is a badge's answer, the same shape for every badge so bar.js
// reads them all one way.
func writeCount(w http.ResponseWriter, n int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Waiting int `json:"waiting"`
	}{n})
}
