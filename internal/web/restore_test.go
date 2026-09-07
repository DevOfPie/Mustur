package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DevOfPie/Mustur/internal/session"
	"github.com/DevOfPie/Mustur/internal/store"
)

// appearing is a tmux that reports nothing until a session has been created,
// which is what Start's check after creating one needs to find. The web
// package's fakeRunner answers the same listing forever, so a session started
// through it never settles.
type appearing struct {
	mu      sync.Mutex
	listing string
	after   string
	calls   []string
}

func (a *appearing) Run(_ context.Context, _ string, args ...string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, strings.Join(args, " "))
	if len(args) > 0 && args[0] == "new-session" {
		a.listing = a.after
	}
	if len(args) > 0 && args[0] == "list-sessions" {
		return a.listing, nil
	}
	return "", nil
}

func (a *appearing) ran(sub string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, c := range a.calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

// transcript writes a file standing in for a conversation the CLI has kept, and
// returns its path. A conversation is only offered back when the file is
// actually there.
func transcript(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+".jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func restoreServer(t *testing.T, run session.Runner) (*httptest.Server, *store.Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "restore.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	a := &session.Adapter{Run: run, Stat: func(string) error { return nil }}
	s := &Sessions{Hub: &session.Hub{Adapter: a}, Adapter: a, Actor: "pie", Store: st}
	mux := http.NewServeMux()
	s.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st, ctx
}

// The page a reboot lands on offers back what the reboot took, and does not
// offer back what is still running.
func TestTheStartPageOffersBackWhatIsNotRunning(t *testing.T) {
	srv, st, ctx := restoreServer(t, fakeRunner{listing: owned("mustur/alive")})
	if err := st.RememberSession(ctx, "lost", "/checkout", "claude"); err != nil {
		t.Fatal(err)
	}
	if err := st.NoteSessionCLI(ctx, "lost", "abc-123", transcript(t, "abc-123")); err != nil {
		t.Fatal(err)
	}
	if err := st.RememberSession(ctx, "alive", "/checkout", "claude"); err != nil {
		t.Fatal(err)
	}

	body := getFrom(t, srv, "/sessions?new=1")
	if !strings.Contains(body, "/sessions/lost/restore") {
		t.Error("a session that is gone is not offered back")
	}
	if strings.Contains(body, "/sessions/alive/restore") {
		t.Error("a session that is still running is offered as lost")
	}
	if !strings.Contains(body, "Comes back with the conversation") {
		t.Error("the page does not say the conversation returns with it")
	}
}

// Without a conversation identifier the session starts again in the same place
// and empty. The page says so rather than promising a transcript.
func TestASessionWithNoConversationSaysItComesBackEmpty(t *testing.T) {
	srv, st, ctx := restoreServer(t, fakeRunner{})
	if err := st.RememberSession(ctx, "lost", "/checkout", "claude"); err != nil {
		t.Fatal(err)
	}
	body := getFrom(t, srv, "/sessions?new=1")
	if !strings.Contains(body, "empty") {
		t.Errorf("the page promises something it cannot fetch: %s", body)
	}
}

// The subtraction only means anything against a live list. With tmux
// unreachable every remembered session looks missing, and offering to start six
// that are all already running is worse than offering none.
func TestNothingIsOfferedBackWhenTmuxCannotBeAsked(t *testing.T) {
	srv, st, ctx := restoreServer(t, refusing{})
	if err := st.RememberSession(ctx, "lost", "/checkout", "claude"); err != nil {
		t.Fatal(err)
	}
	if body := getFrom(t, srv, "/sessions?new=1"); strings.Contains(body, "/restore") {
		t.Error("a session was offered back on the word of a tmux that did not answer")
	}
}

func TestRestoringStartsTheSessionOnItsOwnConversation(t *testing.T) {
	run := &appearing{after: owned("mustur/lost")}
	srv, st, ctx := restoreServer(t, run)
	if err := st.RememberSession(ctx, "lost", "/checkout", "claude"); err != nil {
		t.Fatal(err)
	}
	if err := st.NoteSessionCLI(ctx, "lost", "abc-123", transcript(t, "abc-123")); err != nil {
		t.Fatal(err)
	}

	res := postRestore(t, srv, "/sessions/lost/restore", srv.URL)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("restore got %d, want 303", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/sessions/lost" {
		t.Errorf("landed at %q, want the session that was just started", loc)
	}
	// The directory and the conversation come from the row, never from the
	// request: there is not even a list for a browser to choose from here.
	if !run.ran("new-session -d -s mustur/lost -c /checkout claude --resume abc-123") {
		t.Errorf("unexpected argv: %v", run.calls)
	}
}

func TestRestoreRefusesACrossOriginPost(t *testing.T) {
	run := &appearing{after: owned("mustur/lost")}
	srv, st, ctx := restoreServer(t, run)
	if err := st.RememberSession(ctx, "lost", "/checkout", "claude"); err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"", "https://evil.example"} {
		if res := postRestore(t, srv, "/sessions/lost/restore", origin); res.StatusCode != http.StatusForbidden {
			t.Errorf("a POST from %q got %d, want 403", origin, res.StatusCode)
		}
	}
	if run.ran("new-session") {
		t.Error("a refused request still started a session")
	}
}

// A project named in a URL cannot reach anything Mustur is not holding.
func TestRestoreRefusesAProjectNobodyRemembers(t *testing.T) {
	run := &appearing{after: owned("mustur/whatever")}
	srv, _, _ := restoreServer(t, run)

	res := postRestore(t, srv, "/sessions/whatever/restore", srv.URL)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("got %d, want a redirect carrying the refusal", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); !strings.Contains(loc, "not%20holding") {
		t.Errorf("the refusal does not say why: %q", loc)
	}
	if run.ran("new-session") {
		t.Error("a session Mustur never started was started from a URL")
	}
}

// refusing is a tmux that cannot be reached at all — not "no server running",
// which is the honest answer that nothing exists, but a failure.
type refusing struct{}

func (refusing) Run(context.Context, string, ...string) (string, error) {
	return "", errors.New("tmux: permission denied")
}

func postRestore(t *testing.T, srv *httptest.Server, path, origin string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	res, err := srv.Client().Transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res
}

// The hook reports a conversation as the CLI starts and the file is not written
// until the conversation has something in it. A session started and never
// spoken to has an identifier naming nothing, and --resume on that is a command
// that exits at once — which would arrive as "the session died on startup", two
// steps from anything explaining it.
func TestASessionWhoseTranscriptWasNeverWrittenComesBackEmpty(t *testing.T) {
	run := &appearing{after: owned("mustur/lost")}
	srv, st, ctx := restoreServer(t, run)
	if err := st.RememberSession(ctx, "lost", "/checkout", "claude"); err != nil {
		t.Fatal(err)
	}
	// An identifier, and a path to a file nobody ever wrote.
	if err := st.NoteSessionCLI(ctx, "lost", "abc-123", filepath.Join(t.TempDir(), "never.jsonl")); err != nil {
		t.Fatal(err)
	}
	if body := getFrom(t, srv, "/sessions?new=1"); !strings.Contains(body, "empty") {
		t.Error("the page promised a conversation that is not on disk")
	}
	if res := postRestore(t, srv, "/sessions/lost/restore", srv.URL); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("restore got %d, want 303", res.StatusCode)
	}
	if run.ran("--resume") {
		t.Errorf("resumed a conversation that does not exist: %v", run.calls)
	}
	if !run.ran("new-session -d -s mustur/lost -c /checkout claude") {
		t.Errorf("the session did not come back at all: %v", run.calls)
	}
}
