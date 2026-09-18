package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// `mustur session list` against a tmux that fails is an error, not a machine
// with nothing on it. Before MUS-F-0160 the adapter answered nil, nil for
// anything printed through "error connecting to", and this command printed
// "no sessions Mustur started" and exited zero (MUS-D-0062).
//
// The failure is a real tmux refused its socket: a file at the socket path that
// nobody may write, so connect() fails with EACCES and tmux prints "error
// connecting to <socket> (Permission denied)" -- the exact shape the old match
// swallowed. No server is started, so nothing here can reach one that is
// running.
func TestSessionListReturnsTheErrorWhenTmuxFails(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root is permitted everything")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux on PATH; this test only means something against the real thing")
	}
	// Short and under /tmp: the socket would be <dir>/tmux-<uid>/default, and
	// a unix socket path is capped near 108 bytes.
	dir, err := os.MkdirTemp("/tmp", "mustur-list-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sockdir := filepath.Join(dir, "tmux-"+strconv.Itoa(os.Getuid()))
	if err := os.Mkdir(sockdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sockdir, "default"), nil, 0); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_TMPDIR", dir)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	err = cmdSession([]string{"list"})
	os.Stdout = stdout
	w.Close()
	printed, _ := io.ReadAll(r)

	if err == nil {
		t.Fatal("a socket tmux was refused was listed as no error")
	}
	if !strings.Contains(err.Error(), "Permission denied") {
		t.Errorf("the error does not carry what tmux said: %v", err)
	}
	if strings.Contains(string(printed), "no sessions") {
		t.Errorf("a failed listing printed absence: %q", printed)
	}
}
