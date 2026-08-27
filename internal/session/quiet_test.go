package session

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"
)

// A session quiet since before anybody attached says so to the first viewer.
//
// The stream learns lastAt by appending output, which is the right answer for
// every moment after the first and no answer at all for the first — exactly the
// case the quiet timer exists for. A reader that has just opened has seen
// nothing, so a session silent since Sunday reported silence of none.
//
// tmux knew all along: session_activity is already read by List for the route
// row's default. This is the seed that uses it.
func TestAQuietSessionSaysSoToItsFirstViewer(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux on PATH")
	}
	ctx := context.Background()
	a := &Adapter{}
	project := "zzProbeQuiet"
	if _, err := a.Start(ctx, project, t.TempDir(), "sh -c 'echo hello; sleep 30'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Stop(ctx, project) })

	// Long enough that zero and the truth cannot be confused.
	time.Sleep(3 * time.Second)

	h := &Hub{Adapter: a}
	t.Cleanup(h.Shutdown)
	if h.lastActive(ctx, project).IsZero() {
		t.Fatal("tmux was not asked, or could not say, when this session last did anything")
	}

	sub, _, err := h.Watch(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if got := sub.Quiet(time.Now()); got < time.Second {
		t.Errorf("a session quiet for three seconds reports %v to its first viewer", got.Truncate(time.Second))
	}

	// And the first frame does not reset it. Somebody starting to watch is not
	// the session doing something — the same mistake the capture-pane seed made
	// when it was stamped with now.
	time.Sleep(700 * time.Millisecond)
	if got := sub.Quiet(time.Now()); got < time.Second {
		t.Errorf("attaching reset the counter: %v", got.Truncate(time.Second))
	}
}

// What the pane says it is doing, read from the pane.
//
// The owner pointed at this: the CLI already prints whether a turn is running,
// and counting silence is a guess standing in for that fact. A tool call that
// produces nothing for two minutes is working; a session that finished four
// seconds ago is not; no amount of counting bytes tells those apart.
//
// One CLI's strings, deliberately. Anything else falls through to
// AgentUnknown and the surface goes back to the timer — degrading to the old
// guess is the right failure, where asserting "idle" about a CLI nobody has
// read would not be.
func TestDoingReadsThePane(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name string
		pane string
		want Agent
	}{
		{
			"mid-turn",
			"✽ Osmosing… (3s · ↓ 136 tokens · thinking with high effort)\n" +
				"❯ \n" +
				"  ⏵⏵ auto mode on (shift+tab to cycle) · esc to interrupt · ← 1 agent",
			AgentWorking,
		},
		{
			// The input box is drawn during a turn as well, so looking for the
			// caret first would call every working session idle. This is the
			// case that ordering protects.
			"waiting at the prompt",
			"● 30\n✻ Cooked for 7s · done 1:09 AM\n" +
				"❯ \n" +
				"  ⏵⏵ auto mode on (shift+tab to cycle) · ← 1 agent",
			AgentWaiting,
		},
		{
			"something else entirely",
			"$ tail -f /var/log/syslog\nAug 27 01:12:03 whippy-vm kernel: nothing to see",
			AgentUnknown,
		},
		{"an empty pane", "", AgentUnknown},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := &Adapter{Run: paneRunner{out: c.pane}}
			if got := a.Doing(ctx, "Whatever"); got != c.want {
				t.Errorf("pane reads as %q, want %q", got, c.want)
			}
		})
	}

	// A tmux that will not answer says nothing, rather than saying idle.
	a := &Adapter{Run: paneRunner{err: true}}
	if got := a.Doing(ctx, "Whatever"); got != AgentUnknown {
		t.Errorf("a failed capture reads as %q, want unknown", got)
	}
	// And a project name tmux would read as a target is refused before it is
	// handed to tmux at all.
	if got := a.Doing(ctx, "bad:name"); got != AgentUnknown {
		t.Errorf("an unsafe project reads as %q, want unknown", got)
	}
}

// paneRunner answers every command with one canned pane.
type paneRunner struct {
	out string
	err bool
}

func (p paneRunner) Run(context.Context, string, ...string) (string, error) {
	if p.err {
		return "", errNoPane
	}
	return p.out, nil
}

var errNoPane = fmt.Errorf("no server running")
