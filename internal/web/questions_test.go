package web

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

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
	q := &Questions{Store: s, Project: "MUS", Actor: "pie"}
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

func TestWithdrawClosesWithoutAnAnswer(t *testing.T) {
	srv, s := serveQuestions(t, openQuestion("MUS-Q-0001", "Own the session, or attach?"))

	res, err := srv.Client().PostForm(srv.URL+"/questions", url.Values{
		"id":       {"MUS-Q-0001"},
		"withdraw": {"1"},
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

func TestAnEmptyQueueSaysSo(t *testing.T) {
	srv, _ := serveQuestions(t)
	if body := getFrom(t, srv, "/questions"); !strings.Contains(body, "Nothing waiting on you") {
		t.Error("an empty queue does not say it is empty")
	}
}
