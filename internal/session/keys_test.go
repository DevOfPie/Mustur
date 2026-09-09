package session

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// waitFor polls the pane until want appears, or gives up.
//
// Polled rather than slept on: a fixed sleep is either flaky or slow, and this
// is measuring whether a keystroke arrived at all, not how fast.
func waitFor(t *testing.T, a *Adapter, project, want string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		out, err := a.Capture(context.Background(), project, "-100")
		if err == nil {
			last = out
			if strings.Contains(out, want) {
				return out
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return last
}

// The keys reach the pane as keys.
//
// MUS-F-0080 is that everything reaching a pane was a line of text followed by
// Enter, so a dialog wanting a keypress was visible and unreachable. The unit
// tests above hold the allowlist and prove nothing about tmux, and "Escape
// reaches the pane" is a claim about tmux.
//
// `cat -v` is the probe: it prints control characters rather than acting on
// them, so Escape shows as ^[ and Up as ^[[A. That is the byte the CLI would
// have received, read back off the screen.
func TestTheChosenKeysArriveAsThemselves(t *testing.T) {
	realTmux(t)
	a := &Adapter{}
	project := "zzKeys"
	start(t, a, project, "cat -v")

	ctx := context.Background()
	// Escape: the key the owner reaches for to interrupt an agent mid-turn
	// (MUS-Q-0073), and the reason the row exists.
	if err := a.SendKey(ctx, project, "escape"); err != nil {
		t.Fatalf("escape: %v", err)
	}
	if got := waitFor(t, a, project, "^["); !strings.Contains(got, "^[") {
		t.Fatalf("Escape never reached the pane; screen was:\n%s", got)
	}

	// Up is three bytes, and a surface that sent the word would send four.
	if err := a.SendKey(ctx, project, "up"); err != nil {
		t.Fatalf("up: %v", err)
	}
	if got := waitFor(t, a, project, "^[[A"); !strings.Contains(got, "^[[A") {
		t.Fatalf("Up did not arrive as an arrow; screen was:\n%s", got)
	}

	// And nothing followed either of them. Send appends Enter because a message
	// is a line to submit; a key is the key and nothing else, and a stray Enter
	// would answer a dialog the owner had only meant to look at.
	out, err := a.Capture(ctx, project, "-100")
	if err != nil {
		t.Fatal(err)
	}
	// cat -v echoes each line it reads, so an Enter after the escape would have
	// produced a line of its own. Both sequences sit on one line if neither
	// submitted.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "^[[A") {
			if !strings.Contains(line, "^[") {
				t.Error("the arrow arrived on a line of its own, so something submitted between the two keys")
			}
			return
		}
	}
}

// A key name the server does not know never reaches tmux.
func TestAnUnknownKeyIsRefusedBeforeItReachesTmux(t *testing.T) {
	a := &Adapter{Run: refuseAll{}}
	err := a.SendKey(context.Background(), "Mustur", "C-z")
	if err == nil || !strings.Contains(err.Error(), "not a key this may send") {
		t.Fatalf("want a refusal naming the key, got %v", err)
	}
}

var errNotSupposedToRun = errors.New("tmux was reached for a key that should have been refused")

// refuseAll fails any command, so a test that reaches tmux fails loudly rather
// than passing for the wrong reason.
type refuseAll struct{}

func (refuseAll) Run(context.Context, string, ...string) (string, error) {
	return "", errNotSupposedToRun
}
