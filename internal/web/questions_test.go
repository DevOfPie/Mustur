package web

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"net/http/httptest"

	"github.com/DevOfPie/Mustur/internal/question"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

func serveQuestions(t *testing.T, recs ...record.Record) (*httptest.Server, *store.Store) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, r := range recs {
		if err := s.Append(ctx, r, "create", "test"); err != nil {
			t.Fatal(err)
		}
	}
	// Sessions on: this helper stands in for a server serving everything, and
	// the surface-that-is-not-served case has its own test next door.
	q := &Questions{Store: s, Project: "MUS", Actor: "pie", ShowSessions: true}
	mux := http.NewServeMux()
	q.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, s
}

func openQuestion(id, title string) record.Record {
	return record.Record{
		ID: id, Kind: question.Kind, Title: title, At: "2026-08-21",
		Body: "one short paragraph of context",
		Data: []record.Field{
			{Key: question.FieldStatus, Value: question.StatusOpen},
			{Key: question.FieldBlocks, Value: "milestone 3"},
		},
	}
}

func getFrom(t *testing.T, srv *httptest.Server, path string) string {
	t.Helper()
	res, err := srv.Client().Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The interaction that must not fail: the question is visible, what it blocks
// is visible beside it, and the answer box is on the list rather than behind a
// tap into somewhere else.
func TestOpenQuestionIsVisibleAndAnswerableInOneAction(t *testing.T) {
	srv, _ := serveQuestions(t, openQuestion("MUS-Q-0001", "Own the session, or attach?"))
	body := getFrom(t, srv, "/questions")

	for _, want := range []string{"Own the session, or attach?", "milestone 3", `name="answer"`, "MUS-Q-0001"} {
		if !strings.Contains(body, want) {
			t.Errorf("the queue does not show %q", want)
		}
	}
	// One action means the form posts straight from the list.
	if !strings.Contains(body, `action="/questions"`) {
		t.Error("no form on the list itself")
	}
}

func TestAnsweringClosesItAndRecordsWhatWasSaid(t *testing.T) {
	srv, s := serveQuestions(t, openQuestion("MUS-Q-0001", "Own the session, or attach?"))

	res, err := srv.Client().PostForm(srv.URL+"/questions", url.Values{
		"id":     {"MUS-Q-0001"},
		"answer": {"It owns the session."},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	got, err := s.Get(context.Background(), "MUS-Q-0001")
	if err != nil {
		t.Fatal(err)
	}
	if question.Status(got) != question.StatusAnswered {
		t.Errorf("status = %q, want answered", question.Status(got))
	}
	if ans, _ := got.Get(question.FieldAnswer); ans != "It owns the session." {
		t.Errorf("answer = %q", ans)
	}
	if at, _ := got.Get(question.FieldAnswered); strings.TrimSpace(at) == "" {
		t.Error("no answered-at recorded")
	}
	if body := getFrom(t, srv, "/questions"); !strings.Contains(body, "Nothing waiting on you") {
		t.Error("an answered question is still on the queue")
	}
}

// Post/redirect/get, so a phone reloading after a dropped connection does not
// answer twice — and a second answer to a closed question is not an error
// worth alarming anyone about.
func TestAnsweringTwiceIsHarmless(t *testing.T) {
	srv, s := serveQuestions(t, openQuestion("MUS-Q-0001", "Own the session, or attach?"))
	form := url.Values{"id": {"MUS-Q-0001"}, "answer": {"first"}}

	for i := 0; i < 2; i++ {
		res, err := srv.Client().PostForm(srv.URL+"/questions", form)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode >= 500 {
			t.Fatalf("answer %d returned %d", i+1, res.StatusCode)
		}
	}
	got, err := s.Get(context.Background(), "MUS-Q-0001")
	if err != nil {
		t.Fatal(err)
	}
	if ans, _ := got.Get(question.FieldAnswer); ans != "first" {
		t.Errorf("the second answer overwrote the first: %q", ans)
	}
}

func TestEmptyAnswerIsRefusedAndTheQuestionStaysOpen(t *testing.T) {
	srv, s := serveQuestions(t, openQuestion("MUS-Q-0001", "Own the session, or attach?"))

	res, err := srv.Client().PostForm(srv.URL+"/questions", url.Values{
		"id":     {"MUS-Q-0001"},
		"answer": {"   "},
	})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	got, err := s.Get(context.Background(), "MUS-Q-0001")
	if err != nil {
		t.Fatal(err)
	}
	if !question.IsOpen(got) {
		t.Error("an empty answer closed the question")
	}
}

// The tick is carried here too. Withdrawing without it is refused, which
// TestWithdrawNeedsTheTickBesideIt holds; this test is about what a deliberate
// withdrawal does, and it needs the same form the surface sends.
func TestWithdrawClosesWithoutAnAnswer(t *testing.T) {
	srv, s := serveQuestions(t, openQuestion("MUS-Q-0001", "Own the session, or attach?"))

	res, err := srv.Client().PostForm(srv.URL+"/questions", url.Values{
		"id":       {"MUS-Q-0001"},
		"withdraw": {"1"},
		"sure":     {"1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	got, err := s.Get(context.Background(), "MUS-Q-0001")
	if err != nil {
		t.Fatal(err)
	}
	if question.Status(got) != question.StatusWithdrawn {
		t.Errorf("status = %q, want withdrawn", question.Status(got))
	}
	if ans, ok := got.Get(question.FieldAnswer); ok && strings.TrimSpace(ans) != "" {
		t.Errorf("a withdrawn question carries an answer: %q", ans)
	}
}

// Cloudflare Access sets the identity at the edge. Whoever answered is who the
// record says answered.
func TestTheAnswererIsWhoAccessSays(t *testing.T) {
	srv, s := serveQuestions(t, openQuestion("MUS-Q-0001", "Own the session, or attach?"))

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/questions",
		strings.NewReader(url.Values{"id": {"MUS-Q-0001"}, "answer": {"yes"}}.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cf-Access-Authenticated-User-Email", "dev@killerofpie.com")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	events, err := s.History(context.Background(), "MUS-Q-0001")
	if err != nil {
		t.Skipf("no history to read: %v", err)
	}
	var sawActor bool
	for _, e := range events {
		if strings.Contains(e.Actor, "dev@killerofpie.com") {
			sawActor = true
		}
	}
	if !sawActor {
		t.Error("the answer was not attributed to the Access identity")
	}
}

func withOptions(id, title string, opts ...string) record.Record {
	r := openQuestion(id, title)
	for _, o := range opts {
		r.Data = append(r.Data, record.Field{Key: question.FieldOption, Value: o})
	}
	return r
}

// The artboard's central idea: answers are options with what each one costs,
// not a text box that makes the owner reconstruct the list the asker had.
func TestOptionsRenderWithTheirLineAndDetail(t *testing.T) {
	srv, _ := serveQuestions(t, withOptions("MUS-Q-0001", "Where does the audit run?",
		"Check StrucGu out in CI :: Recommended · conformance runs on every push :: The catalog is fetched per run.",
		"Vendor a pinned copy :: Runs offline · a stale copy proves nothing :: A pinned specification goes stale silently."))
	body := getFrom(t, srv, "/questions")

	for _, want := range []string{
		"Check StrucGu out in CI",
		// The line without its marker. This read "Recommended · conformance
		// runs on every push" until MUS-F-0072: the owner asked for the
		// recommendation to be a mark on the row rather than the first words of
		// the description, and the prefix is still what the record carries.
		"conformance runs on every push",
		"The catalog is fetched per run.",
		"Vendor a pinned copy",
		"A pinned specification goes stale silently.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the queue does not show %q", want)
		}
	}
	if strings.Contains(body, "Recommended · conformance") {
		t.Error("the marker is still text in the description")
	}
	// Expansion in place, and no script anywhere on the page.
	if !strings.Contains(body, "<details") {
		t.Error("options do not expand in place")
	}
	// Only the bar's script. The queue's own design is that it works without
	// one, and the owner's answer on MUS-Q-0078 added exactly the badge and
	// nothing else — so what is worth asserting is that nothing followed it in.
	if got := scriptsIn(body); len(got) != 1 || got[0] != "/assets/bar.js" {
		t.Errorf("the queue loads %v, want only the bar's script", got)
	}
}

// What is blocked comes first, above the question. That is what separates a
// milestone-stopping question from a sentence-stopping one.
func TestWhatIsBlockedComesBeforeTheQuestion(t *testing.T) {
	srv, _ := serveQuestions(t, openQuestion("MUS-Q-0001", "Where does the audit run?"))
	body := getFrom(t, srv, "/questions")

	blocks := strings.Index(body, "blocks milestone 3")
	title := strings.Index(body, "Where does the audit run?")
	if blocks < 0 || title < 0 {
		t.Fatalf("missing blocks=%d title=%d", blocks, title)
	}
	if blocks > title {
		t.Error("what is blocked is rendered after the question")
	}
}

func TestChoosingAnOptionAnswersTheQuestion(t *testing.T) {
	srv, s := serveQuestions(t, withOptions("MUS-Q-0001", "Where does the audit run?",
		"Check StrucGu out in CI :: Recommended :: detail"))

	res, err := srv.Client().PostForm(srv.URL+"/questions", url.Values{
		"id":     {"MUS-Q-0001"},
		"option": {"Check StrucGu out in CI"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	got, err := s.Get(context.Background(), "MUS-Q-0001")
	if err != nil {
		t.Fatal(err)
	}
	if question.Status(got) != question.StatusAnswered {
		t.Fatalf("status = %q", question.Status(got))
	}
	if ans, _ := got.Get(question.FieldAnswer); ans != "Check StrucGu out in CI" {
		t.Errorf("answer = %q, want the chosen option", ans)
	}
}

// Free text beside a chosen option is a note on that choice.
//
// This test held the opposite until MUS-Q-0068: MUS-D-0055 put free text
// beneath the options and let it beat a choice, so picking an option and adding
// a remark meant retyping the label into the box, and the record then said only
// what was typed. The owner asked for both (MUS-F-0071) and chose to keep the
// choice as the answer. The clause that survives is the one below it: text with
// no choice is still the answer itself.
func TestANoteRidesAlongsideTheChosenOption(t *testing.T) {
	srv, s := serveQuestions(t, withOptions("MUS-Q-0001", "Where does the audit run?",
		"Check StrucGu out in CI :: Recommended :: detail"))

	res, err := srv.Client().PostForm(srv.URL+"/questions", url.Values{
		"id":     {"MUS-Q-0001"},
		"option": {"Check StrucGu out in CI"},
		"answer": {"but only once the catalog is pinned"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	got, err := s.Get(context.Background(), "MUS-Q-0001")
	if err != nil {
		t.Fatal(err)
	}
	// Verbatim, so it can still be matched back to the option it names.
	if ans, _ := got.Get(question.FieldAnswer); ans != "Check StrucGu out in CI" {
		t.Errorf("answer = %q, want the chosen option unchanged", ans)
	}
	if note := question.NoteOf(got); note != "but only once the catalog is pinned" {
		t.Errorf("note = %q, want the remark kept beside the choice", note)
	}
}

// And text with no choice is the answer, which is MUS-D-0055's case for what
// the list does not contain. That half is unchanged.
func TestFreeTextWithNoChoiceIsStillTheAnswer(t *testing.T) {
	srv, s := serveQuestions(t, withOptions("MUS-Q-0001", "Where does the audit run?",
		"Check StrucGu out in CI :: Recommended :: detail"))

	res, err := srv.Client().PostForm(srv.URL+"/questions", url.Values{
		"id":     {"MUS-Q-0001"},
		"answer": {"Neither. Ask me again after milestone 4."},
	})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	got, err := s.Get(context.Background(), "MUS-Q-0001")
	if err != nil {
		t.Fatal(err)
	}
	if ans, _ := got.Get(question.FieldAnswer); ans != "Neither. Ask me again after milestone 4." {
		t.Errorf("answer = %q, want the typed text", ans)
	}
	// No choice was made, so there is nothing for a note to be a note on.
	if note := question.NoteOf(got); note != "" {
		t.Errorf("note = %q, want none: the text was the answer", note)
	}
}

// A waiting session is told the choice and the note, not the choice alone.
func TestTheNoteTravelsWithTheAnswerIntoTheSession(t *testing.T) {
	rec := withOptions("MUS-Q-0001", "Where does the audit run?",
		"Check StrucGu out in CI :: Recommended :: detail")
	rec.Data = append(rec.Data, record.Field{Key: question.FieldProject, Value: "Mustur"})

	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Append(ctx, rec, "create", "test"); err != nil {
		t.Fatal(err)
	}

	spy := &recordingSender{}
	q := &Questions{Store: s, Project: "MUS", Actor: "pie", Sessions: spy}
	mux := http.NewServeMux()
	q.Routes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := srv.Client().PostForm(srv.URL+"/questions", url.Values{
		"id":     {"MUS-Q-0001"},
		"option": {"Check StrucGu out in CI"},
		"answer": {"but only once the catalog is pinned"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if !strings.Contains(spy.sent, "Check StrucGu out in CI") {
		t.Errorf("the session was not told the choice: %q", spy.sent)
	}
	if !strings.Contains(spy.sent, "but only once the catalog is pinned") {
		t.Errorf("the session was told the choice and not the note, so it would act on an option the owner qualified: %q", spy.sent)
	}
}

// A sender that keeps what it was handed.
type recordingSender struct{ sent string }

func (recordingSender) Alive(context.Context, string) (bool, error) { return true, nil }
func (r *recordingSender) Send(_ context.Context, _, text string) error {
	r.sent = text
	return nil
}

// The surface marks the recommended option. The first build computed the flag
// and never rendered it, so an option read as recommended only because the
// asker happened to type the word.
func TestTheRecommendedOptionIsMarkedBySurfaceNotByText(t *testing.T) {
	srv, _ := serveQuestions(t, withOptions("MUS-Q-0001", "Where does the audit run?",
		"Check StrucGu out in CI :: Recommended · runs on every push :: detail",
		"Vendor a pinned copy :: a stale copy proves nothing :: detail"))
	body := getFrom(t, srv, "/questions")

	if !strings.Contains(body, `class="rec"`) {
		t.Error("the recommended option carries no mark of its own")
	}
	if n := strings.Count(body, `class="rec"`); n != 1 {
		t.Errorf("%d options marked recommended, want 1", n)
	}
}

// An option with no detail has no disclosure, so hiding the radio inside one
// made it unpickable without a blind tap. Selection is the row now.
func TestABareOptionIsPickableWithoutExpandingAnything(t *testing.T) {
	srv, _ := serveQuestions(t, withOptions("MUS-Q-0001", "Ship it?",
		"Yes", "No"))
	body := getFrom(t, srv, "/questions")

	if strings.Contains(body, "<details") {
		t.Error("a question whose options carry no detail renders a disclosure")
	}
	for _, want := range []string{`value="Yes"`, `value="No"`} {
		if !strings.Contains(body, want) {
			t.Errorf("no radio for %s", want)
		}
	}
	// The radio must not be inside a details element for any option.
	if i := strings.Index(body, "<details"); i >= 0 {
		if j := strings.Index(body[i:], `type="radio"`); j >= 0 {
			t.Error("a radio is nested inside a disclosure")
		}
	}
}

// The queue has to be reachable from intake when the queue is empty. Before
// this, the only route was the banner — which renders when something is open —
// so the queue could be reached from intake exactly when it had nothing to say.
func TestTheQueueIsReachableFromIntakeWhenNothingIsOpen(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	in := &Intake{Store: s, Project: "MUS", Actor: "pie"}
	srv := httptest.NewServer(in.Handler())
	defer srv.Close()

	body := getFrom(t, srv, "/intake")
	if strings.Contains(body, "waiting on you") {
		t.Fatal("the banner rendered with no open questions; this test proves nothing")
	}
	if !strings.Contains(body, `href="/questions"`) {
		t.Error("no route from intake to the decision queue")
	}
}

// A tab that goes nowhere is an unbuilt capability described as existing.
//
// The list of built surfaces moves as milestones land, so this asserts the rule
// rather than a snapshot: everything built and served has a tab, and nothing
// else does. Sessions was on the unbuilt side until milestone 4b and had to
// move; it is now built but served only on request, which is why this server is
// constructed with it switched on.
func TestOnlyBuiltSurfacesGetATab(t *testing.T) {
	srv, _ := serveQuestions(t, openQuestion("MUS-Q-0001", "Where does the audit run?"))
	body := getFrom(t, srv, "/questions")

	for _, built := range []string{"/intake", "/sessions", "/records"} {
		if !strings.Contains(body, `href="`+built+`"`) {
			t.Errorf("no tab for %s, which is built", built)
		}
	}
	// Records arrived at milestone 5b and moved from the list below to the one
	// above, which is what this test is shaped to survive.
	for _, unbuilt := range []string{"/routing", "/audit"} {
		if strings.Contains(body, `href="`+unbuilt+`"`) {
			t.Errorf("a tab points at %s, which is not built", unbuilt)
		}
	}
}

// slowSender blocks in Send until its context is done, which is what an
// unresponsive tmux looks like from here.
type slowSender struct{ called chan struct{} }

func (s *slowSender) Alive(context.Context, string) (bool, error) { return true, nil }

func (s *slowSender) Send(ctx context.Context, _, _ string) error {
	close(s.called)
	<-ctx.Done()
	return ctx.Err()
}

// The answer is the owner's the moment it reaches the server. A phone that
// drops the connection while tmux is being shelled out to must not unmake it —
// a review reproduced exactly that, and the question came back still open with
// no answer and no reason.
func TestAnAnswerSurvivesTheClientDisconnecting(t *testing.T) {
	rec := openQuestion("MUS-Q-0001", "Where does the audit run?")
	rec.Data = append(rec.Data, record.Field{Key: question.FieldProject, Value: "Mustur"})

	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Append(ctx, rec, "create", "test"); err != nil {
		t.Fatal(err)
	}

	slow := &slowSender{called: make(chan struct{})}
	q := &Questions{Store: s, Project: "MUS", Actor: "pie", Sessions: slow, DeliverTimeout: 300 * time.Millisecond}
	mux := http.NewServeMux()
	q.Routes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	reqCtx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, srv.URL+"/questions",
		strings.NewReader(url.Values{"id": {"MUS-Q-0001"}, "answer": {"CI, with a real catalog"}}.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	done := make(chan struct{})
	go func() {
		defer close(done)
		if res, err := srv.Client().Do(req); err == nil {
			res.Body.Close()
		}
	}()

	<-slow.called // The handler is inside the delivery.
	cancel()      // The phone goes away.
	<-done

	// The delivery is bounded by its own timeout, so give the handler room to
	// finish writing after the request context died.
	deadline := time.Now().Add(5 * time.Second)
	var got record.Record
	for time.Now().Before(deadline) {
		got, err = s.Get(ctx, "MUS-Q-0001")
		if err == nil && !question.IsOpen(got) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	if question.IsOpen(got) {
		t.Fatal("the answer was lost when the client disconnected")
	}
	if ans, _ := got.Get(question.FieldAnswer); ans != "CI, with a real catalog" {
		t.Errorf("answer = %q", ans)
	}
	if d, _ := got.Get(question.FieldDelivered); !strings.Contains(d, "not delivered") {
		t.Errorf("delivery outcome = %q, want a not-delivered reason", d)
	}
}

func TestAnEmptyQueueSaysSo(t *testing.T) {
	srv, _ := serveQuestions(t)
	if body := getFrom(t, srv, "/questions"); !strings.Contains(body, "Nothing waiting on you") {
		t.Error("an empty queue does not say it is empty")
	}
}

// The person answering learns what became of the answer.
//
// MUS-F-0070: the owner answered a question that named no session, waited for a
// session to resume, and nothing had ever been going to type into one. Deliver
// already returned the sentence and it went only into the record, which is the
// one place the person who just answered was not looking. MUS-D-0065 stands --
// an undelivered answer is still an answer -- and saying so is not the same as
// refusing it.
func TestTheAnswererIsToldWhereTheAnswerWent(t *testing.T) {
	// No FieldProject: a question raised outside a session Mustur started,
	// which is the common case and the one that surprised the owner. Sessions
	// is set, because the surprise needs a server that could have delivered --
	// with the flag off there is no Sessions tab either and nobody is waiting
	// for a session to resume.
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Append(ctx, openQuestion("MUS-Q-0001", "Where does the audit run?"), "create", "test"); err != nil {
		t.Fatal(err)
	}
	q := &Questions{Store: s, Project: "MUS", Actor: "pie", Sessions: liveSender{}, ShowSessions: true}
	mux := http.NewServeMux()
	q.Routes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := srv.Client().PostForm(srv.URL+"/questions",
		url.Values{"id": {"MUS-Q-0001"}, "answer": {"CI, with a real catalog"}})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	// The client follows the redirect, so this is the page the owner lands on.
	if !strings.Contains(string(body), "Answered <code>MUS-Q-0001</code>") {
		t.Fatal("the queue does not confirm the answer at all")
	}
	if !strings.Contains(string(body), "not delivered") {
		t.Errorf("the page says nothing about delivery, so an answer that reached no session looks like one that did:\n%s", string(body))
	}
	if !strings.Contains(string(body), "names no session") {
		t.Error("the reason is not carried back, only the fact")
	}
}

// And a delivery that worked says so, rather than saying nothing.
func TestADeliveredAnswerSaysWhereItWentToo(t *testing.T) {
	rec := openQuestion("MUS-Q-0001", "Where does the audit run?")
	rec.Data = append(rec.Data, record.Field{Key: question.FieldProject, Value: "Mustur"})

	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Append(ctx, rec, "create", "test"); err != nil {
		t.Fatal(err)
	}

	q := &Questions{Store: s, Project: "MUS", Actor: "pie", Sessions: liveSender{}}
	mux := http.NewServeMux()
	q.Routes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := srv.Client().PostForm(srv.URL+"/questions",
		url.Values{"id": {"MUS-Q-0001"}, "answer": {"CI, with a real catalog"}})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "typed into") {
		t.Errorf("a delivered answer does not say so:\n%s", string(body))
	}
}

// A sender whose session is alive and takes what it is given.
type liveSender struct{}

func (liveSender) Alive(context.Context, string) (bool, error) { return true, nil }
func (liveSender) Send(context.Context, string, string) error  { return nil }

// The answer box cannot submit on Enter.
//
// MUS-F-0076: it was a single-line input, and Enter in one submits the form.
// The owner pressed it mid-sentence while writing a note and the half they had
// typed was recorded as the answer. A textarea takes Enter as a newline, so the
// Answer button is the only way out.
func TestTheAnswerBoxIsMultiLineSoEnterCannotSubmitIt(t *testing.T) {
	srv, _ := serveQuestions(t, withOptions("MUS-Q-0001", "Where does the audit run?",
		"Check StrucGu out in CI :: Recommended :: detail"))
	body := getFrom(t, srv, "/questions")

	if !strings.Contains(body, `<textarea name="answer"`) {
		t.Error("the answer box is not a textarea, so Enter still submits mid-sentence")
	}
	if strings.Contains(body, `<input type="text" name="answer"`) {
		t.Error("the single-line input is still there")
	}
	// It is the same box that carries a note, so it is spell-checked like the
	// other place prose is written.
	if !strings.Contains(body, `spellcheck="true"`) {
		t.Error("the box is not spell-checked")
	}
}

// Withdraw says what it does before it does it.
//
// MUS-F-0077: the button said only "Withdraw" and closed the question with no
// answer on one press. The owner pressed it not knowing, and MUS-Q-0060 closed.
func TestWithdrawNeedsTheTickBesideIt(t *testing.T) {
	srv, s := serveQuestions(t, openQuestion("MUS-Q-0001", "Where does the audit run?"))

	body := getFrom(t, srv, "/questions")
	if !strings.Contains(body, `name="sure"`) {
		t.Fatal("nothing beside the button says what it does")
	}
	if !strings.Contains(body, "close it with no answer") {
		t.Error("the tick does not say what withdrawing is")
	}

	// Unticked: refused, and the question is untouched.
	res, err := srv.Client().PostForm(srv.URL+"/questions",
		url.Values{"id": {"MUS-Q-0001"}, "withdraw": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	page, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(context.Background(), "MUS-Q-0001")
	if err != nil {
		t.Fatal(err)
	}
	if !question.IsOpen(got) {
		t.Fatalf("an unticked withdraw closed the question: status %q", question.Status(got))
	}
	if !strings.Contains(string(page), "Tick the box") {
		t.Errorf("the refusal does not say how to withdraw on purpose:\n%s", string(page))
	}

	// Ticked: it withdraws, because saying no to the owner twice is not the point.
	res, err = srv.Client().PostForm(srv.URL+"/questions",
		url.Values{"id": {"MUS-Q-0001"}, "withdraw": {"1"}, "sure": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	got, err = s.Get(context.Background(), "MUS-Q-0001")
	if err != nil {
		t.Fatal(err)
	}
	if question.Status(got) != question.StatusWithdrawn {
		t.Errorf("a ticked withdraw did not withdraw: status %q", question.Status(got))
	}
}

// Answer reads as unavailable until there is something to answer with.
//
// MUS-Q-0071. Done in CSS rather than with script or a required attribute:
// the first would make this the seventh scripted surface and the second would
// retire MUS-D-0055's clause that text alone can answer a question that offers
// options.
func TestAnswerIsDimmedUntilThereIsSomethingToAnswerWith(t *testing.T) {
	srv, _ := serveQuestions(t, withOptions("MUS-Q-0001", "Where does the audit run?",
		"Check StrucGu out in CI :: Recommended :: detail"))
	body := getFrom(t, srv, "/questions")

	if !strings.Contains(body, "input[type=radio]:checked") {
		t.Error("nothing asks whether an option is chosen")
	}
	if !strings.Contains(body, "textarea:not(:placeholder-shown)") {
		t.Error("nothing asks whether the box has text, so text alone would not enable it")
	}
	// The dimming is CSS and stays CSS. The bar's script is the only one here,
	// and it touches the badge and nothing else — if the button ever becomes
	// script's job, this is where it shows up.
	if got := scriptsIn(body); len(got) != 1 || got[0] != "/assets/bar.js" {
		t.Errorf("the queue loads %v; the dimming is meant to be CSS", got)
	}
	// Withdraw has to work when nothing is chosen -- that is what it is for.
	if strings.Contains(body, "button { opacity: .45") {
		t.Error("the rule is not scoped to the primary button, so Withdraw is dimmed too")
	}
}

// The row carries a mark and the description does not carry the word.
func TestTheRecommendationIsAMarkNotAWordInTheDescription(t *testing.T) {
	srv, _ := serveQuestions(t, withOptions("MUS-Q-0001", "Where does the audit run?",
		"Check StrucGu out in CI :: Recommended. It is the only place with a real catalog :: detail"))
	body := getFrom(t, srv, "/questions")

	if !strings.Contains(body, `class="rec"`) {
		t.Fatal("nothing marks the recommended option")
	}
	if !strings.Contains(body, "&#9733;") {
		t.Error("the mark is not the star")
	}
	if strings.Contains(body, ">recommended<") {
		t.Error("the word is still rendered as the mark")
	}
	if !strings.Contains(body, "It is the only place with a real catalog") {
		t.Error("the description lost its sentence")
	}
	if strings.Contains(body, "Recommended. It is the only place") {
		t.Error("the marker is still text in the description, which is what MUS-F-0072 reported")
	}
}
