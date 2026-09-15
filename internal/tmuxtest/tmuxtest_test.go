package tmuxtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) { Main(m) }

// A session started under Main lives on a socket inside the private directory,
// and a scope cannot be taken — measured against tmux itself rather than by
// reading the environment back.
func TestTmuxReachesOnlyThePrivateServer(t *testing.T) {
	dir := os.Getenv("TMUX_TMPDIR")
	if dir == "" || os.Getenv("TMUX") != "" {
		t.Fatalf("TMUX_TMPDIR=%q TMUX=%q, want a private dir and no TMUX", dir, os.Getenv("TMUX"))
	}
	if err := exec.Command("systemd-run", "--user", "--scope", "true").Run(); err == nil {
		t.Fatal("systemd-run succeeded; a test server could take the live service's scope")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", "zzTmuxtest").CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v: %s", err, out)
	}
	defer exec.Command("tmux", "kill-session", "-t", "zzTmuxtest").Run()
	out, err := exec.Command("tmux", "display-message", "-p", "-t", "zzTmuxtest", "#{socket_path}").Output()
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(out))
	if !strings.HasPrefix(got, dir+string(filepath.Separator)) {
		t.Fatalf("socket %q is not under %q", got, dir)
	}
}
