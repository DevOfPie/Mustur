package session

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

// These run against real tmux. The package's other tests use a fake runner,
// and a fake that agrees with the code proves nothing about tmux — which is
// where every defect in this milestone's predecessor actually lived.
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

func TestOutputReachesAViewer(t *testing.T) {
	realTmux(t)
	a := &Adapter{}
	h := &Hub{Adapter: a, Dir: t.TempDir()}
	project := "zzStreamOut"

	start(t, a, project, "sh -c 'for i in 1 2 3; do echo line-$i; sleep 0.2; done; sleep 30'")

	sub, _, _, _, err := h.Attach(context.Background(), project, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	var got strings.Builder
	deadline := time.After(15 * time.Second)
	for !strings.Contains(got.String(), "line-3") {
		select {
		case u := <-sub.C:
			got.WriteString(u.Text)
		case <-deadline:
			t.Fatalf("did not see line-3; got %q", got.String())
		}
	}
}

// A viewer that reconnects gets what it missed, exactly once. The counter is
// monotonic so a duplicate or a hole is visible rather than plausible.
func TestAReconnectingViewerGetsTheGapExactlyOnce(t *testing.T) {
	realTmux(t)
	a := &Adapter{}
	h := &Hub{Adapter: a, Dir: t.TempDir()}
	project := "zzStreamResume"

	start(t, a, project, "sh -c 'i=0; while [ $i -lt 40 ]; do i=$((i+1)); echo n$i; sleep 0.1; done; sleep 30'")

	first, _, _, _, err := h.Attach(context.Background(), project, 0)
	if err != nil {
		t.Fatal(err)
	}
	var seen strings.Builder
	var at int64
	deadline := time.After(15 * time.Second)
	for !strings.Contains(seen.String(), "n5") {
		select {
		case u := <-first.C:
			seen.WriteString(u.Text)
			at = u.Seq
		case <-deadline:
			t.Fatalf("never saw n5; got %q", seen.String())
		}
	}
	first.Close()

	time.Sleep(700 * time.Millisecond) // Output continues with nobody watching.

	second, backlog, from, gap, err := h.Attach(context.Background(), project, at)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if gap {
		t.Fatal("a short absence reported a gap; the buffer should have covered it")
	}
	if from != at {
		t.Fatalf("resumed at %d, asked for %d", from, at)
	}

	all := seen.String() + string(backlog)
	deadline = time.After(15 * time.Second)
	for !strings.Contains(all, "n12") {
		select {
		case u := <-second.C:
			all += u.Text
		case <-deadline:
			t.Fatalf("never saw n12 after resuming; got %q", all)
		}
	}
	// Exactly once: every counter appears on exactly one line across the seam.
	// capture-pane ends lines with \n and pipe-pane with \r\n, so the lines are
	// normalised rather than matched against a guessed terminator.
	lines := map[string]int{}
	for _, l := range strings.Split(strings.ReplaceAll(all, "\r\n", "\n"), "\n") {
		lines[strings.TrimSpace(l)]++
	}
	for _, want := range []string{"n1", "n5", "n9", "n12"} {
		if n := lines[want]; n != 1 {
			t.Errorf("%q appears on %d lines across the reconnect, want 1:\n%s", want, n, all)
		}
	}
}

// Supervision, and the whole of it: notice the session is gone and say so.
func TestAViewerIsToldWhenTheSessionEnds(t *testing.T) {
	realTmux(t)
	old := pollEvery
	pollEvery = 200 * time.Millisecond
	t.Cleanup(func() { pollEvery = old })

	a := &Adapter{}
	h := &Hub{Adapter: a, Dir: t.TempDir()}
	project := "zzStreamEnd"

	start(t, a, project, "sh -c 'echo working; sleep 60'")

	sub, _, _, _, err := h.Attach(context.Background(), project, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	if err := a.Stop(context.Background(), project); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(15 * time.Second)
	for {
		select {
		case u := <-sub.C:
			if u.Ended {
				return
			}
		case <-deadline:
			t.Fatal("the viewer was never told the session ended")
		}
	}
}

// One reader per session, however many viewers. Two attachments must not open
// two pipes, which is what keeps client cost flat as sessions multiply.
func TestTwoViewersShareOneReader(t *testing.T) {
	realTmux(t)
	a := &Adapter{}
	h := &Hub{Adapter: a, Dir: t.TempDir()}
	project := "zzStreamShare"

	start(t, a, project, "sh -c 'while true; do echo tick; sleep 0.2; done'")

	one, _, _, _, err := h.Attach(context.Background(), project, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer one.Close()
	two, _, _, _, err := h.Attach(context.Background(), project, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer two.Close()

	h.mu.Lock()
	n := len(h.streams)
	refs := h.streams[project].refs
	h.mu.Unlock()

	if n != 1 {
		t.Errorf("%d streams for one session", n)
	}
	if refs != 2 {
		t.Errorf("refcount %d, want 2", refs)
	}

	// Both see output.
	for _, s := range []*Sub{one, two} {
		select {
		case u := <-s.C:
			if u.Text == "" && !u.Ended {
				t.Error("a viewer got an empty update")
			}
		case <-time.After(10 * time.Second):
			t.Fatal("a viewer saw nothing")
		}
	}
}

func TestAttachRefusesASessionMusturDidNotStart(t *testing.T) {
	h := &Hub{Adapter: &Adapter{Run: &fake{out: listing(unowned("mustur/theirs", 1, false))}}}
	if _, _, _, _, err := h.Attach(context.Background(), "theirs", 0); err == nil {
		t.Fatal("attached to a session Mustur did not start")
	}
}

// The seam between the capture-pane seed and the pipe.
//
// Both carry whatever the pane printed while the reader was being set up, so
// the join repeats lines. At 5 lines/s it never shows; a review found 6-11
// duplicated lines on four of six sessions at 50 lines/s, which is nearer an
// agent's actual output rate. Six sessions because the overlap depends on
// timing and one is not evidence.
func TestTheSeedSeamDoesNotDuplicateLines(t *testing.T) {
	realTmux(t)
	a := &Adapter{}
	re := regexp.MustCompile(`LINE (\d{6})`)

	for n := 0; n < 6; n++ {
		start(t, a, fmt.Sprintf("zzSeam%d", n),
			"sh -c 'i=0; while true; do i=$((i+1)); printf \"LINE %06d\\n\" $i; sleep 0.02; done'")
	}
	// Let the panes get ahead, so capture-pane has something to overlap with.
	time.Sleep(2 * time.Second)

	for n := 0; n < 6; n++ {
		project := fmt.Sprintf("zzSeam%d", n)
		h := &Hub{Adapter: a, Dir: t.TempDir()}
		sub, backlog, _, _, err := h.Attach(context.Background(), project, 0)
		if err != nil {
			t.Fatalf("%s: %v", project, err)
		}
		var b strings.Builder
		b.Write(backlog)
		deadline := time.After(3 * time.Second)
	collect:
		for {
			select {
			case u := <-sub.C:
				b.WriteString(u.Text)
			case <-deadline:
				break collect
			}
		}
		sub.Close()
		h.Shutdown()

		seen := map[string]int{}
		var dups []string
		for _, m := range re.FindAllStringSubmatch(b.String(), -1) {
			seen[m[1]]++
			if seen[m[1]] == 2 {
				dups = append(dups, m[1])
			}
		}
		if len(dups) > 0 {
			t.Errorf("%s: %d line numbers arrived twice across the seam: %v",
				project, len(dups), dups)
		}
	}
}

// A viewer whose offset is ahead of the stream was reading a previous reader —
// the linger expired and a new one began at zero. Handing it the new stream's
// position silently loses everything between, which a review measured at 5,550
// lines with nothing said.
func TestAViewerFromAPreviousReaderIsToldItMissedSomething(t *testing.T) {
	s := &Stream{project: "x", subs: map[chan Update]struct{}{}}
	s.buf = []byte("fresh output")
	s.next = int64(len(s.buf))

	if _, _, gap := s.since(9999); !gap {
		t.Fatal("a viewer ahead of this stream was told nothing was missing")
	}
}

// A viewer away longer than the buffer is told, rather than shown a hole.
func TestAViewerAwayTooLongIsToldWhatItMissed(t *testing.T) {
	s := &Stream{project: "x", subs: map[chan Update]struct{}{}}
	s.buf = []byte("tail")
	s.next = 100000 // Far beyond what the buffer holds.

	_, at, gap := s.since(5)
	if !gap {
		t.Fatal("no gap reported for a viewer far behind the buffer")
	}
	if at != s.next-int64(len(s.buf)) {
		t.Errorf("resumed at %d, want the oldest byte still held", at)
	}
}
