package session

// Every test here is a thing the sweep must refuse to do.
//
// One of them restarts a session; the rest are the cases where it must not, and
// they are the point. This is the only code in Mustur that acts on a running
// agent without a person pressing something, so the tests that matter are the
// ones proving it keeps its hands off.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// sweepRunner is a tmux that answers a listing and a capture, and records what
// it was told to do.
type sweepRunner struct {
	mu      sync.Mutex
	listing string
	// after is what tmux lists once the session has been started again, which
	// is what Start waits to see before it reports success.
	after string
	pane  map[string]string
	calls []string
}

func (r *sweepRunner) Run(_ context.Context, _ string, args ...string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	joined := strings.Join(args, " ")
	r.calls = append(r.calls, joined)
	switch {
	case len(args) > 0 && args[0] == "kill-session":
		// tmux drops it from the listing, and Start's one-per-project check
		// asks the listing. A fake that keeps answering with a session that has
		// been killed is a fake that agrees with a bug rather than a tmux
		// (MUS-D-0114).
		r.listing = ""
		return "", nil
	case len(args) > 0 && args[0] == "new-session":
		r.listing = r.after
		return "", nil
	case len(args) > 0 && args[0] == "list-sessions":
		return r.listing, nil
	case len(args) > 0 && args[0] == "capture-pane":
		for project, screen := range r.pane {
			if strings.Contains(joined, "mustur/"+project) {
				return screen, nil
			}
		}
		return "", nil
	}
	return "", nil
}

func (r *sweepRunner) ran(sub string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

// recall is what Start wrote down, as the sweep reads it back.
type recall struct {
	dir, cmd string
	missing  bool
}

func (r recall) Remembered(context.Context, string) (string, string, bool) {
	if r.missing {
		return "", "", false
	}
	return r.dir, r.cmd, true
}

type watching bool

func (w watching) Watching(string) bool { return bool(w) }

// screen builds a pane: some body, then the CLI's furniture.
//
// The shapes are the real ones — a right-aligned notice above a divider above
// the input box above the status line — because the whole feature turns on
// reading them, and a test that invents the layout tests the invention.
func screen(body, notice, typed, status string) string {
	pad := strings.Repeat(" ", 60)
	div := strings.Repeat("─", 100)
	line := ""
	if notice != "" {
		line = pad + notice + "\n"
	}
	return body + "\n" + line + div + "\n❯ " + typed + "\n" + div + "\n  " + status + "\n"
}

const (
	updateNotice = "✔ Update installed · Restart to update"
	waitingLine  = "⏵⏵ auto mode on (shift+tab to cycle)"
	workingLine  = "⏵⏵ auto mode on (shift+tab to cycle) · esc to interrupt"
)

func sweeperFor(t *testing.T, run *sweepRunner, r recall, w Watcher) (*Sweeper, *fakeClock) {
	t.Helper()
	clock := &fakeClock{at: time.Date(2026, 9, 11, 8, 0, 0, 0, time.UTC)}
	return &Sweeper{
		Adapter: &Adapter{Run: run, Stat: func(string) error { return nil }},
		Recall:  r,
		Watch:   w,
		Now:     clock.now,
	}, clock
}

type fakeClock struct{ at time.Time }

func (c *fakeClock) now() time.Time       { return c.at }
func (c *fakeClock) pass(d time.Duration) { c.at = c.at.Add(d) }

// settle runs the sweep twice so the screen has been seen unchanged, then lets
// the dwell pass and sweeps again. That is the shortest path to the only state
// in which anything is allowed to happen.
func settle(ctx context.Context, s *Sweeper, c *fakeClock) {
	s.Sweep(ctx) // first sight: records the screen, does nothing
	c.pass(Quiet + time.Minute)
	s.Sweep(ctx)
}

func TestAnIdleSessionWithAnUpdateIsRestartedWhereItWas(t *testing.T) {
	run := &sweepRunner{
		listing: owned("mustur/Research", 1, false), after: owned("mustur/Research", 1, false),
		pane: map[string]string{"Research": screen("done.", updateNotice, "", waitingLine)},
	}
	s, c := sweeperFor(t, run, recall{dir: "/checkout", cmd: "claude --resume abc-123"}, watching(false))
	settle(context.Background(), s, c)

	if !run.ran("kill-session -t mustur/Research") {
		t.Fatalf("the session was not stopped: %v", run.calls)
	}
	if !run.ran("new-session -d -s mustur/Research -c /checkout claude --resume abc-123") {
		t.Errorf("it did not come back where it was, on its conversation: %v", run.calls)
	}
}

// The four refusals. Each one is a thing that would be destroyed.
func TestTheSweepRefusesEveryCaseItWasGiven(t *testing.T) {
	for _, tc := range []struct {
		name    string
		screen  string
		watched bool
		why     string
	}{
		{
			name:   "a turn in flight",
			screen: screen("thinking", updateNotice, "", workingLine),
			why:    "a turn was interrupted; the pane said esc to interrupt",
		},
		{
			name:   "a line typed and not sent",
			screen: screen("done.", updateNotice, "milestone 8 is accepted", waitingLine),
			why:    "a typed line was destroyed, which is the case found on the machine",
		},
		{
			name:    "somebody is reading it",
			screen:  screen("done.", updateNotice, "", waitingLine),
			watched: true,
			why:     "a session was restarted under a browser tab holding it open",
		},
		{
			name:   "no update to take",
			screen: screen("done.", "", "", waitingLine),
			why:    "a session was restarted with nothing announcing an update",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := &sweepRunner{
				listing: owned("mustur/Research", 1, false),
				pane:    map[string]string{"Research": tc.screen},
			}
			s, c := sweeperFor(t, run, recall{dir: "/checkout", cmd: "claude"}, watching(tc.watched))
			settle(context.Background(), s, c)
			if run.ran("kill-session") {
				t.Errorf("%s: %v", tc.why, run.calls)
			}
		})
	}
}

// The dwell is the owner's number and it is a floor, not a suggestion.
func TestASessionQuietForLessThanTheDwellIsLeftAlone(t *testing.T) {
	run := &sweepRunner{
		listing: owned("mustur/Research", 1, false), after: owned("mustur/Research", 1, false),
		pane: map[string]string{"Research": screen("done.", updateNotice, "", waitingLine)},
	}
	s, c := sweeperFor(t, run, recall{dir: "/checkout", cmd: "claude"}, watching(false))
	ctx := context.Background()
	s.Sweep(ctx)
	c.pass(Quiet - time.Minute)
	s.Sweep(ctx)
	if run.ran("kill-session") {
		t.Errorf("restarted a session quiet for less than the dwell: %v", run.calls)
	}
	c.pass(2 * time.Minute)
	s.Sweep(ctx)
	if !run.ran("kill-session") {
		t.Error("the dwell passed and nothing happened")
	}
}

// The spinner turns and the token count ticks, so the capture changes while the
// screen says nothing new. Measuring the dwell on the capture would reset it
// forever and the feature would never fire (MUS-F-0135).
func TestTheDwellIsNotResetByTheCLIsOwnFurnitureMoving(t *testing.T) {
	run := &sweepRunner{
		listing: owned("mustur/Research", 1, false), after: owned("mustur/Research", 1, false),
		pane: map[string]string{"Research": screen("done.", updateNotice, "", waitingLine+" · 41.2k tokens")},
	}
	s, c := sweeperFor(t, run, recall{dir: "/checkout", cmd: "claude"}, watching(false))
	ctx := context.Background()
	s.Sweep(ctx)
	// Same body, a status line that has moved on.
	run.mu.Lock()
	run.pane["Research"] = screen("done.", updateNotice, "", waitingLine+" · 41.9k tokens")
	run.mu.Unlock()
	c.pass(Quiet + time.Minute)
	s.Sweep(ctx)
	if !run.ran("kill-session") {
		t.Errorf("a ticking status line reset the dwell: %v", run.calls)
	}
}

// Anything the session actually said is a real change and starts the dwell over.
func TestSomethingSaidResetsTheDwell(t *testing.T) {
	run := &sweepRunner{
		listing: owned("mustur/Research", 1, false), after: owned("mustur/Research", 1, false),
		pane: map[string]string{"Research": screen("done.", updateNotice, "", waitingLine)},
	}
	s, c := sweeperFor(t, run, recall{dir: "/checkout", cmd: "claude"}, watching(false))
	ctx := context.Background()
	s.Sweep(ctx)
	run.mu.Lock()
	run.pane["Research"] = screen("done.\nand one more thing", updateNotice, "", waitingLine)
	run.mu.Unlock()
	c.pass(Quiet + time.Minute)
	s.Sweep(ctx) // sees a different body: records it, does not act
	if run.ran("kill-session") {
		t.Errorf("restarted a session that had just said something: %v", run.calls)
	}
}

// Nothing written down is nothing to start again, and stopping it anyway would
// lose a session to take an update nobody could then apply.
func TestASessionWithNothingRememberedIsNotStopped(t *testing.T) {
	run := &sweepRunner{
		listing: owned("mustur/Research", 1, false), after: owned("mustur/Research", 1, false),
		pane: map[string]string{"Research": screen("done.", updateNotice, "", waitingLine)},
	}
	s, c := sweeperFor(t, run, recall{missing: true}, watching(false))
	settle(context.Background(), s, c)
	if run.ran("kill-session") {
		t.Errorf("stopped a session it could not start again: %v", run.calls)
	}
}

// With tmux unanswering, nothing is restarted — the same answer every other
// path gives to the same silence (MUS-D-0062).
func TestNothingIsRestartedWhenTmuxCannotBeAsked(t *testing.T) {
	run := &sweepRunner{listing: "", pane: map[string]string{}}
	s, c := sweeperFor(t, run, recall{dir: "/checkout", cmd: "claude"}, watching(false))
	settle(context.Background(), s, c)
	if run.ran("kill-session") {
		t.Errorf("restarted something on the word of a tmux that listed nothing: %v", run.calls)
	}
}

// The input box's contents are the owner's. The sweep needs to know it is not
// empty and must never carry what it says.
func TestWhatIsTypedIsNeverCarried(t *testing.T) {
	_, st := SplitChrome(screen("done.", updateNotice, "milestone 8 is accepted", waitingLine))
	if !st.Typed {
		t.Fatal("a typed line was not noticed")
	}
	if strings.Contains(st.Mode+strings.Join(st.Items, " ")+st.Note+st.Hint+st.Update, "milestone 8") {
		t.Error("what was typed reached the status; it is the owner's and belongs on their screen only")
	}
}

// The input box is not empty when it looks empty.
//
// The CLI draws its own dim suggestion into the box. The first version of this
// guard read that as a person's draft, so a session showing a suggestion would
// never have taken an update and the feature would have done nothing on the one
// machine it runs on. The owner caught it by saying they had typed nothing into
// a session this called typed.
//
// Both fixtures are real captures, because nothing in the tree had one: the
// ghost came off mustur/Milestone_Work, and the typed one was measured by
// starting a throwaway session, typing into it and not pressing Enter. What it
// showed is that typed text carries no SGR at all after the caret -- it is not
// colour 231, which is what the transcript above the box uses -- so "dim" is
// the only thing separating the two, and the test says so in both directions.
func TestTheCLIsOwnSuggestionIsNotSomebodyTyping(t *testing.T) {
	for _, tc := range []struct {
		file  string
		typed bool
		why   string
	}{
		{"prompt-ghost-suggestion.txt", false, "the CLI's own dim suggestion was read as a person's draft, so this session would never take an update"},
		{"prompt-typed-draft.txt", true, "a real typed draft was read as an empty box, and a restart would destroy it"},
	} {
		raw, err := os.ReadFile(filepath.Join("testdata", tc.file))
		if err != nil {
			t.Fatal(err)
		}
		if _, st := SplitChrome(string(raw)); st.Typed != tc.typed {
			t.Errorf("%s: Typed=%v, want %v -- %s", tc.file, st.Typed, tc.typed, tc.why)
		}
	}
}

// An empty box is an empty box, whatever else is on the screen.
func TestAnEmptyPromptIsNotTyped(t *testing.T) {
	for _, name := range []string{"screen-working.txt", "screen-no-prompt.txt", "prompt-scrolled-past.txt"} {
		raw, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, st := SplitChrome(string(raw)); st.Typed {
			t.Errorf("%s: an empty box read as typed", name)
		}
	}
}

// Somebody attached in a terminal is somebody present.
//
// The owner's clause was "no browser tab open on it". A terminal is the same
// presence by another route, and tmux already reports it.
func TestASessionSomebodyIsAttachedToIsLeftAlone(t *testing.T) {
	run := &sweepRunner{
		listing: owned("mustur/Research", 1, true),
		after:   owned("mustur/Research", 1, true),
		pane:    map[string]string{"Research": screen("done.", updateNotice, "", waitingLine)},
	}
	s, c := sweeperFor(t, run, recall{dir: "/checkout", cmd: "claude"}, watching(false))
	settle(context.Background(), s, c)
	if run.ran("kill-session") {
		t.Errorf("restarted a session somebody was attached to: %v", run.calls)
	}
	if run.ran("capture-pane") {
		t.Error("an attached session's pane was read at all; the listing already said to leave it")
	}
}
