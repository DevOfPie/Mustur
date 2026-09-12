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

// The picker carries what is not running, and lands on a page that offers it
// back (MUS-D-0150).
//
// The defect this closes: the restore list lived only on the start form, and
// /sessions redirects into a running session before that form is reached. After
// a deploy that took three sessions and left one, the owner arrived in the
// survivor and the other three were behind an unlabelled "+".
func TestThePickerCarriesWhatIsNotRunning(t *testing.T) {
	srv, st, ctx := restoreServer(t, fakeRunner{listing: owned("mustur/alive")})
	if err := st.RememberSession(ctx, "lost", "/checkout", "claude"); err != nil {
		t.Fatal(err)
	}
	if err := st.RememberSession(ctx, "alive", "/checkout", "claude"); err != nil {
		t.Fatal(err)
	}

	body := getFrom(t, srv, "/sessions/alive")
	if !strings.Contains(body, `<optgroup label="Not running">`) {
		t.Errorf("the picker on a running session does not carry the lost one: %s", body)
	}
	if !strings.Contains(body, `<option value="lost"`) {
		t.Error("the lost session is not in the dropdown")
	}
	// Two groups or none: a flat list of both would say a session that is gone
	// is somewhere to go.
	if !strings.Contains(body, `<optgroup label="Running">`) {
		t.Error("the running session is not grouped as running")
	}
}

// Choosing one out of that group lands on its own page, which carries the
// button. The dropdown itself starts nothing: a select fires change on every
// option a keyboard arrows past.
func TestALostSessionsPageOffersItBack(t *testing.T) {
	srv, st, ctx := restoreServer(t, fakeRunner{listing: owned("mustur/alive")})
	if err := st.RememberSession(ctx, "lost", "/checkout", "claude"); err != nil {
		t.Fatal(err)
	}
	if err := st.NoteSessionCLI(ctx, "lost", "abc-123", transcript(t, "abc-123")); err != nil {
		t.Fatal(err)
	}

	body := getFrom(t, srv, "/sessions/lost")
	if !strings.Contains(body, `action="/sessions/lost/restore"`) {
		t.Errorf("the page the picker lands on does not offer the session back: %s", body)
	}
	if !strings.Contains(body, "Comes back with the conversation") {
		t.Error("the page does not say whether the conversation returns")
	}
	if !strings.Contains(body, "/checkout") {
		t.Error("the page does not say where it ran")
	}
	// Nothing to stop and no sub-agents to open: there is no session behind it.
	if strings.Contains(body, `action="/sessions/lost/stop"`) {
		t.Error("a session that is not running offers a Stop button")
	}
	if strings.Contains(body, `id="out"`) {
		t.Error("a session that is not running renders a terminal")
	}
}

// A project nobody remembers is still nothing to show. The recover card is for
// sessions Mustur started, not for any name typed into the address bar.
func TestAnUnknownProjectIsStillNothingToShow(t *testing.T) {
	srv, _, _ := restoreServer(t, fakeRunner{listing: owned("mustur/alive")})

	body := getFrom(t, srv, "/sessions/whatever")
	if strings.Contains(body, "/restore") {
		t.Error("a project Mustur never started is offered back")
	}
	if !strings.Contains(body, "did not start a session for whatever") {
		t.Errorf("the page does not say what it is: %s", body)
	}
}

// With tmux unanswering the picker says nothing either: the same rule the start
// page follows, for the same reason.
func TestThePickerOffersNothingWhenTmuxCannotBeAsked(t *testing.T) {
	srv, st, ctx := restoreServer(t, refusing{})
	if err := st.RememberSession(ctx, "lost", "/checkout", "claude"); err != nil {
		t.Fatal(err)
	}
	if body := getFrom(t, srv, "/sessions/lost"); strings.Contains(body, "/restore") {
		t.Error("a session was offered back on the word of a tmux that did not answer")
	}
}

// The picker is bound before the script gives up on a page with no terminal.
//
// The guard `if (!project || !out) return` exists because most of that file is
// a socket painting a screen. The picker is not: the pages it has to work on
// now include the ones with no screen at all, and bound after the guard it was
// a dropdown that did nothing on exactly those pages — with no submit button
// either, because that one lives in a noscript.
func TestThePickerIsBoundBeforeTheTerminalGuard(t *testing.T) {
	js, err := os.ReadFile("assets/session.js")
	if err != nil {
		t.Fatal(err)
	}
	bind := strings.Index(string(js), `document.getElementById("pick")`)
	guard := strings.Index(string(js), "if (!project || !out) return;")
	if bind < 0 || guard < 0 {
		t.Fatal("the picker binding or the terminal guard is gone")
	}
	if bind > guard {
		t.Error("the picker is bound after the script returns, so it is dead on every page with no terminal")
	}
}

// The button says which of the two things pressing it does.
//
// It read "Start it again" in both cases, above a line saying the conversation
// comes back — so the control and the sentence above it disagreed, and the
// owner pressed it expecting a fresh session (MUS-F-0116). The behaviour was
// always the restore; only the word was wrong.
func TestTheRestoreButtonSaysResumeWhenTheConversationComesBack(t *testing.T) {
	srv, st, ctx := restoreServer(t, fakeRunner{})
	if err := st.RememberSession(ctx, "withtalk", "/checkout", "claude"); err != nil {
		t.Fatal(err)
	}
	if err := st.NoteSessionCLI(ctx, "withtalk", "abc-123", transcript(t, "abc-123")); err != nil {
		t.Fatal(err)
	}
	if err := st.RememberSession(ctx, "empty", "/checkout", "claude"); err != nil {
		t.Fatal(err)
	}

	body := getFrom(t, srv, "/sessions?new=1")
	if !strings.Contains(body, "Resume it") {
		t.Error("a session whose conversation comes back is offered as a fresh start")
	}
	if !strings.Contains(body, "Start it again") {
		t.Error("a session with no conversation on disk must still say it starts again")
	}
	// The page the picker lands on says the same thing, and it is a second
	// template rather than the same one.
	one := getFrom(t, srv, "/sessions/withtalk")
	if !strings.Contains(one, "Resume it") {
		t.Errorf("the page the picker lands on still offers a fresh start: %s", one)
	}
}
