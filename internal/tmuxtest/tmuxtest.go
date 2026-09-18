// Package tmuxtest keeps a test binary off the machine's tmux server.
//
// An Adapter with no Runner shells out to plain `tmux`, which reaches the
// default socket — the server holding the owner's running agents. A
// `go test ./...` once spawned dozens of panes beside them and the live
// session picker listed a test's session (MUS-F-0161). Every package whose
// tests can reach real tmux calls Main from its TestMain, so a plain
// `go test` with no environment prepared talks to a private server that dies
// with the binary.
package tmuxtest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Main points tmux at a private socket directory, runs the tests, kills the
// private server and exits with the tests' status.
//
// TMUX is unset as well as TMUX_TMPDIR being set: a tmux client started inside
// a tmux pane follows the socket named in $TMUX, whatever TMUX_TMPDIR says.
//
// systemd-run is shadowed by a shim that fails. Start puts a fresh server in
// the user unit mustur-tmux, whose name is shared with the live service: a
// test's private server holding it would cost the live service its scope for
// as long as the tests ran. Start's fallback when systemd-run fails is to run
// tmux directly, which is the path the tests then take.
func Main(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tmuxtest: %v\n", err)
		os.Exit(2)
	}
	os.Exit(code)
}

func run(m *testing.M) (int, error) {
	// The socket is <dir>/tmux-<uid>/default and a unix socket path is capped
	// near 108 bytes, so a long TMPDIR would make every tmux call fail.
	base := ""
	if len(os.TempDir()) > 40 {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "mustur-tmux-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		return 0, err
	}
	shim := "#!/bin/sh\necho 'systemd-run is disabled under go test (MUS-F-0161)' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "systemd-run"), []byte(shim), 0o755); err != nil {
		return 0, err
	}
	for k, v := range map[string]string{
		"TMUX_TMPDIR": dir,
		"PATH":        bin + string(os.PathListSeparator) + os.Getenv("PATH"),
	} {
		if err := os.Setenv(k, v); err != nil {
			return 0, err
		}
	}
	os.Unsetenv("TMUX")
	os.Unsetenv("TMUX_PANE")
	code := m.Run()
	if _, err := exec.LookPath("tmux"); err == nil {
		// No server having been started is the usual case, and not an error.
		_ = exec.Command("tmux", "kill-server").Run()
	}
	return code, nil
}
