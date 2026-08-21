package session

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// fake records what was run and replies from a script, so the package can be
// tested without tmux and so the exact argv is assertable — the argv is where
// "send it literally" and "never touch a session we did not start" live.
type fake struct {
	calls  [][]string
	out    map[string]string
	errFor map[string]error
	// started is what list-sessions reports once new-session has run, so the
	// check Start makes after creating a session has something to find. A fake
	// where the session appears unconditionally could not tell the difference
	// between a command that survived and one that exited immediately.
	started string
	// vanished makes new-session succeed and the session not exist, which is
	// exactly what an agent CLI crashing on startup looks like.
	vanished bool
}

func (f *fake) Run(_ context.Context, name string, args ...string) (string, error) {
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)
	key := strings.Join(args, " ")
	if strings.HasPrefix(key, "list-sessions") && f.started != "" && f.ran("new-session") && !f.vanished {
		return f.started, nil
	}
	for prefix, err := range f.errFor {
		if strings.HasPrefix(key, prefix) {
			return f.out[prefix], err
		}
	}
	for prefix, out := range f.out {
		if strings.HasPrefix(key, prefix) {
			return out, nil
		}
	}
	return "", nil
}

func (f *fake) ran(sub string) bool {
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), sub) {
			return true
		}
	}
	return false
}

func listing(lines ...string) map[string]string {
	return map[string]string{"list-sessions": strings.Join(lines, "\n")}
}

// owned and unowned build the four-column line tmux returns. The last column
// is the user option Start sets, and it is the whole of "Mustur started this":
// unowned is what a session somebody created by hand looks like, whatever it
// is called.
func owned(name string, windows int, attached bool) string {
	return line(name, windows, attached, "1")
}

func unowned(name string, windows int, attached bool) string {
	return line(name, windows, attached, "")
}

func line(name string, windows int, attached bool, mark string) string {
	a := "0"
	if attached {
		a = "1"
	}
	return fmt.Sprintf("%s\t%d\t%s\t%s", name, windows, a, mark)
}

// The rule the whole package turns on: a session Mustur did not start is not
// visible here, and no method will act on one.
func TestASessionMusturDidNotStartIsInvisible(t *testing.T) {
	f := &fake{out: listing(
		owned("mustur/Mustur", 1, false),
		unowned("mustur/looks-like-ours", 1, false),
		unowned("work", 2, true),
	)}
	a := &Adapter{Run: f}

	got, err := a.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("listed %d sessions, want 1: %+v", len(got), got)
	}
	if got[0].Project != "Mustur" {
		t.Errorf("project = %q", got[0].Project)
	}
}

func TestListParsesWhatTmuxReports(t *testing.T) {
	f := &fake{out: listing(owned("mustur/LinkCtrl", 3, true))}
	a := &Adapter{Run: f}

	got, err := a.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("listed %d", len(got))
	}
	if got[0].Windows != 3 || !got[0].Attached {
		t.Errorf("got %+v, want 3 windows and attached", got[0])
	}
}

// No tmux server is the honest answer that nothing is running, not a failure.
func TestNoServerIsNoSessionsRatherThanAnError(t *testing.T) {
	f := &fake{
		out:    map[string]string{"list-sessions": "no server running on /tmp/tmux-1000/default"},
		errFor: map[string]error{"list-sessions": fmt.Errorf("exit status 1")},
	}
	a := &Adapter{Run: f}

	got, err := a.List(context.Background())
	if err != nil {
		t.Fatalf("a missing server reported as an error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("listed %d sessions with no server", len(got))
	}
}

// A real tmux failure must not be swallowed by the same path.
func TestARealListFailureIsAnError(t *testing.T) {
	f := &fake{
		out:    map[string]string{"list-sessions": "something else went wrong"},
		errFor: map[string]error{"list-sessions": fmt.Errorf("exit status 1")},
	}
	a := &Adapter{Run: f}

	if _, err := a.List(context.Background()); err == nil {
		t.Fatal("a real failure was reported as no sessions")
	}
}

func TestStartRefusesASecondSessionForAProject(t *testing.T) {
	f := &fake{out: listing(owned("mustur/Mustur", 1, false))}
	a := &Adapter{Run: f}

	_, err := a.Start(context.Background(), "Mustur", "/tmp", "claude")
	if err == nil {
		t.Fatal("started a second session for a project that has one")
	}
	if !strings.Contains(err.Error(), "one per project") {
		t.Errorf("error does not say why: %v", err)
	}
}

func TestStartPassesTheDirectoryAndCommand(t *testing.T) {
	f := &fake{started: owned("mustur/Mustur", 1, false)}
	a := &Adapter{Run: f, Stat: func(string) error { return nil }}

	if _, err := a.Start(context.Background(), "Mustur", "/some/checkout", "claude"); err != nil {
		t.Fatal(err)
	}
	if !f.ran("new-session -d -s mustur/Mustur -c /some/checkout claude") {
		t.Errorf("unexpected argv: %v", f.calls)
	}
}

// The session is marked as Mustur's before anything can see it. Until that
// runs it is not ours by the only test that counts.
func TestStartMarksTheSessionAsMusturs(t *testing.T) {
	f := &fake{started: owned("mustur/Mustur", 1, false)}
	a := &Adapter{Run: f, Stat: func(string) error { return nil }}

	if _, err := a.Start(context.Background(), "Mustur", "/some/checkout", "claude"); err != nil {
		t.Fatal(err)
	}
	if !f.ran("set-option -t mustur/Mustur " + OwnedOption + " 1") {
		t.Errorf("the session was not marked: %v", f.calls)
	}
}

// tmux new-session succeeds whether or not the command survives it, so a CLI
// that crashes on startup was reported as a started session and then was not
// one.
func TestStartReportsACommandThatExitedImmediately(t *testing.T) {
	f := &fake{started: owned("mustur/Mustur", 1, false), vanished: true}
	a := &Adapter{Run: f, Stat: func(string) error { return nil }}

	_, err := a.Start(context.Background(), "Mustur", "/some/checkout", "true")
	if err == nil {
		t.Fatal("a session that does not exist was reported as started")
	}
	if !strings.Contains(err.Error(), "exited immediately") {
		t.Errorf("error does not say what happened: %v", err)
	}
}

// tmux falls back to $HOME for a directory that is not there, so a session
// reported as running in a checkout would be running somewhere else.
func TestStartRefusesADirectoryThatIsNotThere(t *testing.T) {
	f := &fake{started: owned("mustur/Mustur", 1, false)}
	a := &Adapter{Run: f, Stat: func(string) error { return fmt.Errorf("no such directory") }}

	_, err := a.Start(context.Background(), "Mustur", "/gone", "claude")
	if err == nil {
		t.Fatal("started a session in a directory that does not exist")
	}
	if f.ran("new-session") {
		t.Errorf("tmux was called anyway: %v", f.calls)
	}
}

func TestStartNeedsACommand(t *testing.T) {
	a := &Adapter{Run: &fake{}}
	if _, err := a.Start(context.Background(), "Mustur", "/tmp", "  "); err == nil {
		t.Fatal("started a session with no command")
	}
}

// tmux reads : and . as target separators, so a project name carrying one would
// address a window or a pane instead of a session.
func TestAProjectNameCannotAddressAWindowOrPane(t *testing.T) {
	for _, bad := range []string{"Mustur:0", "Mustur.1", "a b", "", "../etc"} {
		if _, err := NameFor(bad); err == nil {
			t.Errorf("%q accepted as a project name", bad)
		}
	}
	if got, err := NameFor("DevOfPie_Mustur-2"); err != nil || got != "mustur/DevOfPie_Mustur-2" {
		t.Errorf("NameFor gave %q, %v", got, err)
	}
}

// The text goes literally, so a body tmux would otherwise read as a key name
// arrives as characters. Enter is separate, or it would be typed as a word.
func TestSendTypesLiterallyThenPressesEnter(t *testing.T) {
	f := &fake{out: listing(owned("mustur/Mustur", 1, false))}
	a := &Adapter{Run: f}

	if err := a.Send(context.Background(), "Mustur", "Use Enter and C-c literally"); err != nil {
		t.Fatal(err)
	}
	if !f.ran("send-keys -t mustur/Mustur -l Use Enter and C-c literally") {
		t.Errorf("text was not sent literally: %v", f.calls)
	}
	last := f.calls[len(f.calls)-1]
	if strings.Join(last, " ") != "tmux send-keys -t mustur/Mustur Enter" {
		t.Errorf("Enter was not sent as its own key: %v", last)
	}
}

func TestSendRefusesASessionMusturDidNotStart(t *testing.T) {
	f := &fake{out: listing(unowned("work", 1, true))}
	a := &Adapter{Run: f}

	err := a.Send(context.Background(), "work", "hello")
	if err == nil {
		t.Fatal("typed into a session Mustur did not start")
	}
	if f.ran("send-keys") {
		t.Errorf("send-keys ran anyway: %v", f.calls)
	}
}

func TestStopRefusesASessionMusturDidNotStart(t *testing.T) {
	f := &fake{out: listing(unowned("work", 1, true))}
	a := &Adapter{Run: f}

	if err := a.Stop(context.Background(), "work"); err == nil {
		t.Fatal("killed a session Mustur did not start")
	}
	if f.ran("kill-session") {
		t.Errorf("kill-session ran anyway: %v", f.calls)
	}
}
