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
}

func (f *fake) Run(_ context.Context, name string, args ...string) (string, error) {
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)
	key := strings.Join(args, " ")
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

// The rule the whole package turns on: a session Mustur did not start is not
// visible here, and no method will act on one.
func TestASessionMusturDidNotStartIsInvisible(t *testing.T) {
	f := &fake{out: listing(
		"mustur/Mustur\t1\t0",
		"work\t2\t1",
		"scratch\t1\t0",
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
	f := &fake{out: listing("mustur/LinkCtrl\t3\t1")}
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
	f := &fake{out: listing("mustur/Mustur\t1\t0")}
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
	f := &fake{}
	a := &Adapter{Run: f}

	if _, err := a.Start(context.Background(), "Mustur", "/home/whippy/repos/DevOfPie/Mustur", "claude"); err != nil {
		t.Fatal(err)
	}
	if !f.ran("new-session -d -s mustur/Mustur -c /home/whippy/repos/DevOfPie/Mustur claude") {
		t.Errorf("unexpected argv: %v", f.calls)
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
	f := &fake{out: listing("mustur/Mustur\t1\t0")}
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
	f := &fake{out: listing("work\t1\t1")}
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
	f := &fake{out: listing("work\t1\t1")}
	a := &Adapter{Run: f}

	if err := a.Stop(context.Background(), "work"); err == nil {
		t.Fatal("killed a session Mustur did not start")
	}
	if f.ran("kill-session") {
		t.Errorf("kill-session ran anyway: %v", f.calls)
	}
}
