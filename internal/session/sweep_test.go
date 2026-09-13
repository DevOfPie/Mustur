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

// sweepRunner is a tmux that answers a listing and records what it was told to
// do. The sweep captures nothing itself any more -- everything it knows about a
// screen comes from the hub (MUS-Q-0105) -- so this only has to list, kill and
// start.
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

// hubFake stands in for the poller the hub keeps on every owned session.
type hubFake struct {
	st      Status
	agent   Agent
	quiet   time.Duration
	unknown bool // the poller has not read this session twice yet
	unseen  bool // the poller has not read it at all
	watched bool
}

func (h hubFake) Watching(string) bool { return h.watched }
func (h hubFake) Doing(string) Agent   { return h.agent }
func (h hubFake) Quiet(string, time.Time) (time.Duration, bool) {
	return h.quiet, !h.unknown
}
func (h hubFake) Furniture(string) (Status, bool) { return h.st, !h.unseen }

// screen builds a pane: some body, then the CLI's furniture.
//
// The shapes are the real ones — a right-aligned notice above a divider above
// the input box above the status line — because the whole feature turns on
// reading them, and a test that invents the layout tests the invention.
func screen(body, notice, typed, status string) string {
	pad := strings.Repeat(" ", 60)
	div := strings.Repeat("\u2500", 100)
	line := ""
	if notice != "" {
		line = pad + notice + "\n"
	}
	return body + "\n" + line + div + "\n\u276f " + typed + "\n" + div + "\n  " + status + "\n"
}

// asRead is what the hub would hold for that screen, through the real parsers
// rather than through a description of them.
func asRead(body, notice, typed, status string) (Status, Agent) {
	raw := screen(body, notice, typed, status)
	_, st := SplitChrome(raw)
	return st, DoingIn(raw)
}

const (
	updateNotice = "\u2714 Update installed \u00b7 Restart to update"
	waitingLine  = "\u23f5\u23f5 auto mode on (shift+tab to cycle)"
	workingLine  = "\u23f5\u23f5 auto mode on (shift+tab to cycle) \u00b7 esc to interrupt"
)

func sweeperFor(t *testing.T, run *sweepRunner, r recall, w Watcher) *Sweeper {
	t.Helper()
	return &Sweeper{
		Adapter: &Adapter{Run: run, Stat: func(string) error { return nil }},
		Recall:  r,
		Watch:   w,
		Now:     func() time.Time { return time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC) },
	}
}

// idle is a session at its prompt with an update waiting and nobody about.
func idle() hubFake {
	st, agent := asRead("done.", updateNotice, "", waitingLine)
	return hubFake{st: st, agent: agent, quiet: Quiet + time.Minute}
}

func TestAnIdleSessionWithAnUpdateIsRestartedWhereItWas(t *testing.T) {
	run := &sweepRunner{listing: owned("mustur/Research", 1, false), after: owned("mustur/Research", 1, false)}
	s := sweeperFor(t, run, recall{dir: "/checkout", cmd: "claude --resume abc-123"}, idle())
	s.Sweep(context.Background())

	if !run.ran("kill-session -t mustur/Research") {
		t.Fatalf("the session was not stopped: %v", run.calls)
	}
	if !run.ran("new-session -d -s mustur/Research -c /checkout claude --resume abc-123") {
		t.Errorf("it did not come back where it was, on its conversation: %v", run.calls)
	}
}

// Every case where nothing may happen. Each one is a thing that would be lost.
func TestTheSweepRefusesEveryCaseItWasGiven(t *testing.T) {
	turnInFlight := idle()
	turnInFlight.st, turnInFlight.agent = asRead("thinking", updateNotice, "", workingLine)

	typed := idle()
	typed.st, typed.agent = asRead("done.", updateNotice, "milestone 8 is accepted", waitingLine)

	noUpdate := idle()
	noUpdate.st, noUpdate.agent = asRead("done.", "", "", waitingLine)

	watched := idle()
	watched.watched = true

	tooRecent := idle()
	tooRecent.quiet = Quiet - time.Minute

	notYetKnown := idle()
	notYetKnown.unknown = true

	notYetRead := idle()
	notYetRead.unseen = true

	for _, tc := range []struct {
		name string
		hub  hubFake
		why  string
	}{
		{"a turn in flight", turnInFlight, "a turn was interrupted; the pane said esc to interrupt"},
		{"a line typed and not sent", typed, "a typed line was destroyed"},
		{"no update to take", noUpdate, "a session was restarted with nothing announcing an update"},
		{"somebody is reading it", watched, "a session was restarted under a browser tab holding it open"},
		{"quiet for less than the dwell", tooRecent, "a session that had just moved was restarted"},
		{"the dwell is not known yet", notYetKnown, "a session was restarted on a dwell nothing had measured"},
		{"the poller has not read it", notYetRead, "a session was restarted on a screen nothing had read"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := &sweepRunner{listing: owned("mustur/Research", 1, false), after: owned("mustur/Research", 1, false)}
			s := sweeperFor(t, run, recall{dir: "/checkout", cmd: "claude"}, tc.hub)
			s.Sweep(context.Background())
			if run.ran("kill-session") {
				t.Errorf("%s: %v", tc.why, run.calls)
			}
		})
	}
}

// Somebody attached in a terminal is somebody present.
//
// The owner's clause was "no browser tab open on it". A terminal is the same
// presence by another route, and tmux already reports it.
func TestASessionSomebodyIsAttachedToIsLeftAlone(t *testing.T) {
	run := &sweepRunner{listing: owned("mustur/Research", 1, true), after: owned("mustur/Research", 1, true)}
	s := sweeperFor(t, run, recall{dir: "/checkout", cmd: "claude"}, idle())
	s.Sweep(context.Background())
	if run.ran("kill-session") {
		t.Errorf("restarted a session somebody was attached to: %v", run.calls)
	}
}

// Nothing written down is nothing to start again, and stopping it anyway would
// lose a session to take an update nobody could then apply.
func TestASessionWithNothingRememberedIsNotStopped(t *testing.T) {
	run := &sweepRunner{listing: owned("mustur/Research", 1, false)}
	s := sweeperFor(t, run, recall{missing: true}, idle())
	s.Sweep(context.Background())
	if run.ran("kill-session") {
		t.Errorf("stopped a session it could not start again: %v", run.calls)
	}
}

// With tmux unanswering, nothing is restarted — the same answer every other
// path gives to the same silence (MUS-D-0062).
func TestNothingIsRestartedWhenTmuxCannotBeAsked(t *testing.T) {
	run := &sweepRunner{listing: ""}
	s := sweeperFor(t, run, recall{dir: "/checkout", cmd: "claude"}, idle())
	s.Sweep(context.Background())
	if run.ran("kill-session") {
		t.Errorf("restarted something on the word of a tmux that listed nothing: %v", run.calls)
	}
}

// With nothing polling, nothing is known, so nothing is touched.
func TestNothingIsRestartedWhenNothingIsPolling(t *testing.T) {
	run := &sweepRunner{listing: owned("mustur/Research", 1, false)}
	s := sweeperFor(t, run, recall{dir: "/checkout", cmd: "claude"}, nil)
	s.Sweep(context.Background())
	if run.ran("kill-session") {
		t.Errorf("restarted a session with no poller behind it: %v", run.calls)
	}
}

// The input box's contents are the owner's. The sweep needs to know it is not
// empty and must never carry what it says.
func TestWhatIsTypedIsNeverCarried(t *testing.T) {
	st, _ := asRead("done.", updateNotice, "milestone 8 is accepted", waitingLine)
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
