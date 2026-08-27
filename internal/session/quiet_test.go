package session

import (
	"context"
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

	h := &Hub{Adapter: a, Dir: t.TempDir()}
	t.Cleanup(h.Shutdown)
	if h.lastActive(ctx, project).IsZero() {
		t.Fatal("tmux was not asked, or could not say, when this session last did anything")
	}

	sub, _, _, _, err := h.Attach(ctx, project, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if got := sub.Quiet(time.Now()); got < time.Second {
		t.Errorf("a session quiet for three seconds reports %v to its first viewer", got.Truncate(time.Second))
	}

	// And the seed does not get overwritten by the seed. capture-pane hands
	// over scrollback the pane printed long ago; stamping it with now was the
	// other half of the same defect.
	time.Sleep(600 * time.Millisecond)
	if got := sub.Quiet(time.Now()); got < time.Second {
		t.Errorf("the replayed scrollback reset the counter: %v", got.Truncate(time.Second))
	}
}
