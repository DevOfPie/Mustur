package session

// MUS-F-0160: a listing that failed is not a machine with no sessions on it
// (MUS-D-0062), and until this file the adapter could not tell them apart for
// anything tmux printed through "error connecting to".

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// What tmux 3.6 printed, measured against throwaway TMUX_TMPDIR directories.
// The socket paths are as they were; only the class of each line matters.
var (
	measuredAbsent = map[string]string{
		"no socket ever made (ENOENT)":             "error connecting to /tmp/tmp.rsysKXmQEE/tmux-1000/default (No such file or directory)\n",
		"stale socket, server gone (ECONNREFUSED)": "no server running on /tmp/tmp.bfZTAva7cT/tmux-1000/default\n",
	}
	measuredFailure = map[string]string{
		"socket there, not permitted": "error connecting to /tmp/tmp.riPjfowsBR/tmux-1000/default (Permission denied)\n",
		"server died under the call":  "server exited unexpectedly\n",
		"socket directory unsafe":     "directory /tmp/tmp.mpt0F4hEaE/tmux-1000 has unsafe permissions\n",
		"client killed, said nothing": "",
		// The shape the old match swallowed without tmux ever printing it
		// alone: the absence text carried inside something else.
		"absence text inside a longer failure": "open terminal failed: no such file or directory\n",
	}
)

func listFailing(out string) *fake {
	return &fake{
		out:    map[string]string{"list-sessions": out},
		errFor: map[string]error{"list-sessions": fmt.Errorf("exit status 1")},
	}
}

func TestOnlyTmuxsTwoAbsenceLinesAreNoSessions(t *testing.T) {
	for name, out := range measuredAbsent {
		got, err := (&Adapter{Run: listFailing(out)}).List(context.Background())
		if err != nil || len(got) != 0 {
			t.Errorf("%s: got %v, %v; want no sessions and no error", name, got, err)
		}
	}
	for name, out := range measuredFailure {
		if _, err := (&Adapter{Run: listFailing(out)}).List(context.Background()); err == nil {
			t.Errorf("%s: %q was read as a machine with no sessions", name, out)
		}
	}
}

// A failure is not "no server", so no second server is spawned onto a socket
// that is already serving one.
func TestAConnectionFailureIsNotAServerToSpawn(t *testing.T) {
	a := &Adapter{Run: listFailing(measuredFailure["socket there, not permitted"])}
	if p := a.scopePrefix(context.Background()); p != nil {
		t.Errorf("a refused connection was taken as no server: prefix %v", p)
	}
}

// adopt keeps every pane when the listing fails, logs it once, and still drops
// panes on an honest absence.
func TestAdoptKeepsEveryPaneWhenTheListingFails(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	for name, out := range measuredFailure {
		ch := make(chan Frame)
		p := &pane{project: "kept", subs: map[chan Frame]struct{}{ch: {}}, done: make(chan struct{})}
		h := &Hub{Adapter: &Adapter{Run: listFailing(out)}, panes: map[string]*pane{"kept": p}}
		h.adopt(context.Background())
		h.adopt(context.Background())
		if h.panes["kept"] != p {
			t.Errorf("%s: a failed listing dropped a running session's pane", name)
		}
		select {
		case <-ch:
			t.Errorf("%s: a viewer's socket was closed on a failed listing", name)
		default:
		}
	}
	if n := strings.Count(buf.String(), "listing failed"); n != len(measuredFailure) {
		t.Errorf("logged %d failures for %d failing tmuxes adopted twice each:\n%s", n, len(measuredFailure), buf.String())
	}

	for name, out := range measuredAbsent {
		ch := make(chan Frame)
		p := &pane{project: "gone", subs: map[chan Frame]struct{}{ch: {}}, done: make(chan struct{})}
		h := &Hub{Adapter: &Adapter{Run: listFailing(out)}, panes: map[string]*pane{"gone": p}}
		h.adopt(context.Background())
		if _, ok := h.panes["gone"]; ok {
			t.Errorf("%s: a pane outlived a tmux with no server", name)
		}
	}
}

func TestTheSweepLogsAFailedListingAndRestartsNothing(t *testing.T) {
	var buf bytes.Buffer
	run := &sweepRunner{}
	s := sweeperFor(t, run, recall{dir: "/checkout", cmd: "claude"}, idle())
	s.Adapter.Run = failingList{run}
	s.Log = slog.New(slog.NewTextHandler(&buf, nil))
	s.Sweep(context.Background())
	if run.ran("kill-session") || run.ran("new-session") {
		t.Errorf("restarted on a failed listing: %v", run.calls)
	}
	if !strings.Contains(buf.String(), "Permission denied") {
		t.Errorf("the failure was not logged: %q", buf.String())
	}
}

type failingList struct{ *sweepRunner }

func (f failingList) Run(ctx context.Context, name string, args ...string) (string, error) {
	if len(args) > 0 && args[0] == "list-sessions" {
		return measuredFailure["socket there, not permitted"], fmt.Errorf("exit status 1")
	}
	return f.sweepRunner.Run(ctx, name, args...)
}

// The same three states against the real tmux, on a socket directory of the
// test's own, so a tmux that changes its wording fails here rather than on the
// owner's machine.
func TestRealTmuxAbsenceAndFailureAreToldApart(t *testing.T) {
	realTmux(t)
	ctx := context.Background()
	a := &Adapter{}
	t.Setenv("TMUX", "")
	uid := strconv.Itoa(os.Getuid())
	tmux := func(dir string, args ...string) (string, error) {
		cmd := exec.Command("tmux", args...)
		cmd.Env = append(os.Environ(), "TMUX_TMPDIR="+dir)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	serve := func(t *testing.T) (dir string, pid int) {
		t.Helper()
		dir = t.TempDir()
		if out, err := tmux(dir, "new-session", "-d", "-s", "probe", "sleep 600"); err != nil {
			t.Skipf("could not start a throwaway server: %v: %s", err, out)
		}
		t.Cleanup(func() { _, _ = tmux(dir, "kill-server") })
		// Wait for the pane's child to have exec'd. Killed before then, the
		// forked child can still hold the listening socket, and the next
		// client connects and is told "server exited unexpectedly" rather
		// than "no server running" -- seen once here, and a failure is the
		// right reading of it, but not the state this test sets up.
		for i := 0; ; i++ {
			if cmd, _ := tmux(dir, "display", "-p", "#{pane_current_command}"); cmd == "sleep" {
				break
			}
			if i == 100 {
				t.Fatal("the pane never ran its command")
			}
			time.Sleep(10 * time.Millisecond)
		}
		out, err := tmux(dir, "display", "-p", "#{pid}")
		if pid, err = strconv.Atoi(out); err != nil {
			t.Fatalf("server pid %q: %v", out, err)
		}
		t.Setenv("TMUX_TMPDIR", dir)
		return dir, pid
	}

	t.Run("no server ever started", func(t *testing.T) {
		t.Setenv("TMUX_TMPDIR", t.TempDir())
		if got, err := a.List(ctx); err != nil || len(got) != 0 {
			t.Errorf("got %v, %v; want no sessions and no error", got, err)
		}
	})

	t.Run("stale socket", func(t *testing.T) {
		dir, pid := serve(t)
		kill(t, pid, "-KILL")
		waitGone(t, pid)
		if _, err := os.Stat(filepath.Join(dir, "tmux-"+uid, "default")); err != nil {
			t.Skipf("socket did not outlive its server here: %v", err)
		}
		if got, err := a.List(ctx); err != nil || len(got) != 0 {
			t.Errorf("got %v, %v; want no sessions and no error", got, err)
		}
	})

	t.Run("socket not permitted", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("root is permitted everything")
		}
		dir, _ := serve(t)
		sock := filepath.Join(dir, "tmux-"+uid, "default")
		if err := os.Chmod(sock, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(sock, 0o700) })
		if _, err := a.List(ctx); err == nil {
			t.Error("a server refusing the connection was read as no sessions")
		}
	})

	t.Run("server dies mid-call", func(t *testing.T) {
		_, pid := serve(t)
		kill(t, pid, "-STOP")
		done := make(chan error, 1)
		go func() { _, err := a.List(ctx); done <- err }()
		time.Sleep(300 * time.Millisecond)
		kill(t, pid, "-KILL")
		select {
		case err := <-done:
			if err == nil {
				t.Error("a server dying under the listing was read as no sessions")
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the listing never returned")
		}
	})
}

func kill(t *testing.T, pid int, sig string) {
	t.Helper()
	if out, err := exec.Command("kill", sig, strconv.Itoa(pid)).CombinedOutput(); err != nil {
		t.Fatalf("kill %s %d: %v: %s", sig, pid, err, out)
	}
}

func waitGone(t *testing.T, pid int) {
	t.Helper()
	for i := 0; i < 50; i++ {
		if exec.Command("kill", "-0", strconv.Itoa(pid)).Run() != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server %d did not die", pid)
}
