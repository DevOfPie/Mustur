package session

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func realTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux on PATH; this test only means something against the real thing")
	}
}

// start makes a session named for the test and removes it afterwards, by the
// name it chose and never by pattern.
func start(t *testing.T, a *Adapter, project, cmd string) {
	t.Helper()
	ctx := context.Background()
	if _, err := a.Start(ctx, project, t.TempDir(), cmd); err != nil {
		t.Fatalf("starting %s: %v", project, err)
	}
	t.Cleanup(func() { _ = a.Stop(context.Background(), project) })
}

// watch attaches and tears down, with the poller running fast enough that a
// test does not spend its life waiting.
func watch(t *testing.T, h *Hub, project string) (*Sub, Frame) {
	t.Helper()
	sub, first, err := h.Watch(context.Background(), project)
	if err != nil {
		t.Fatalf("watching %s: %v", project, err)
	}
	t.Cleanup(sub.Close)
	return sub, first
}

// A viewer is handed the screen immediately, not after a tick.
func TestTheFirstFrameArrivesWithTheAttachment(t *testing.T) {
	realTmux(t)
	a := &Adapter{}
	h := &Hub{Adapter: a}
	t.Cleanup(h.Shutdown)
	project := "zzFrameFirst"
	start(t, a, project, "sh -c 'echo hello-from-the-pane; sleep 20'")
	time.Sleep(600 * time.Millisecond)

	_, first := watch(t, h, project)
	if !strings.Contains(first.HTML, "hello-from-the-pane") {
		t.Errorf("the first frame does not carry the screen:\n%s", first.HTML)
	}
	// Rendered, not raw: an escape reaching the page is the defect this whole
	// change exists to fix.
	if strings.Contains(first.HTML, "\x1b") {
		t.Errorf("an escape survived into the frame:\n%q", first.HTML)
	}
}

// A frame arrives when the screen changes, and not otherwise.
func TestAFrameArrivesOnlyWhenTheScreenChanges(t *testing.T) {
	realTmux(t)
	old := PollEvery
	PollEvery = 120 * time.Millisecond
	t.Cleanup(func() { PollEvery = old })

	a := &Adapter{}
	h := &Hub{Adapter: a}
	t.Cleanup(h.Shutdown)
	project := "zzFrameChange"
	start(t, a, project, "sh -c 'echo one; sleep 20'")
	time.Sleep(400 * time.Millisecond)

	sub, _ := watch(t, h, project)

	// Nothing is happening, so nothing should arrive.
	select {
	case f := <-sub.C:
		t.Fatalf("a frame arrived from a still screen:\n%s", f.HTML)
	case <-time.After(500 * time.Millisecond):
	}

	// Now make it move.
	if err := a.Send(context.Background(), project, "echo two"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(6 * time.Second)
	for {
		select {
		case f, ok := <-sub.C:
			if !ok {
				t.Fatal("the subscription closed")
			}
			if strings.Contains(f.HTML, "two") {
				return
			}
		case <-deadline:
			t.Fatal("the screen changed and no frame carried it")
		}
	}
}

// Two viewers of one session share one poller and both see the same screen.
func TestTwoViewersShareOnePoller(t *testing.T) {
	realTmux(t)
	a := &Adapter{}
	h := &Hub{Adapter: a}
	t.Cleanup(h.Shutdown)
	project := "zzFrameShare"
	start(t, a, project, "sh -c 'echo shared; sleep 20'")
	time.Sleep(400 * time.Millisecond)

	_, one := watch(t, h, project)
	_, two := watch(t, h, project)
	if one.HTML != two.HTML {
		t.Error("two viewers of the same session were handed different screens")
	}
	h.mu.Lock()
	n := len(h.panes)
	h.mu.Unlock()
	if n != 1 {
		t.Errorf("%d pollers for one session; two viewers should share one", n)
	}
}

// A session Mustur did not start cannot be watched by naming it.
func TestWatchRefusesASessionMusturDidNotStart(t *testing.T) {
	realTmux(t)
	name := Prefix + "zzFrameTheirs"
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", name, "sleep 20").CombinedOutput(); err != nil {
		t.Skipf("could not make an unowned session: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", name).Run() })

	h := &Hub{Adapter: &Adapter{}}
	t.Cleanup(h.Shutdown)
	if _, _, err := h.Watch(context.Background(), "zzFrameTheirs"); err == nil {
		t.Error("a session carrying no ownership option was watched anyway")
	}
}

// The blank rows tmux pads a capture out to the pane height with are dropped,
// or a two-line session renders as a screenful of nothing.
func TestTrimBlankDropsThePadding(t *testing.T) {
	in := "one\ntwo\n\x1b[38;5;246m\x1b[39m\n   \n\n"
	if got := trimBlank(in); got != "one\ntwo" {
		t.Errorf("padding survived: %q", got)
	}
	// The pane is 80 columns and a phone is not: a line padded out to the pane's
	// width wraps into two or three empty lines of its own at 390px.
	if got := trimBlank("prompt\u00a0        \nnext   "); got != "prompt\u00a0\nnext" {
		t.Errorf("trailing padding survived: %q", got)
	}
	// Which end the padding lands on depends on what the CLI is drawing: a
	// transcript leaves it below, a modal pinned to the bottom leaves it above.
	if got := trimBlank("\n\n\n  content  \n\n"); got != "  content" {
		t.Errorf("leading padding survived: %q", got)
	}
	// An interior blank is not padding — it is a paragraph break.
	if got := trimBlank("one\n\ntwo"); got != "one\n\ntwo" {
		t.Errorf("an interior blank was eaten: %q", got)
	}
	// A hundred of them is. A tall pane pins a dialogue to the bottom and
	// leaves the space between it and the transcript empty, which no amount of
	// trimming the ends can reach.
	if got := trimBlank("top" + strings.Repeat("\n", 101) + "bottom"); got != "top\n\n\nbottom" {
		t.Errorf("the gap did not collapse: %q", got)
	}
	if got := trimBlank("\n\n"); got != "" {
		t.Errorf("an empty pane came back as %q", got)
	}
	if got := trimBlank("only"); got != "only" {
		t.Errorf("a single line was trimmed to %q", got)
	}
}
