package session

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// A systemd unit name: letters, digits and ":_.\-", at most 256 bytes with the
// ".scope" suffix.
var unitName = regexp.MustCompile(`^[A-Za-z0-9:_.\\-]+$`)

// The live service runs on the default socket and must keep the name it has
// always had, or a deploy would leave its running server in a scope nothing
// names (MUS-F-0176).
func TestTheDefaultSocketKeepsTheLiveScopeName(t *testing.T) {
	uid := os.Getuid()
	for _, s := range []string{
		"",
		defaultTmuxSocket(),
		resolveTmuxSocket("", "", uid),
		resolveTmuxSocket("", "/tmp/", uid),
		resolveTmuxSocket(defaultTmuxSocket()+",1234,0", "/elsewhere", uid),
	} {
		if got := TmuxScopeFor(s); got != TmuxScope {
			t.Errorf("TmuxScopeFor(%q) = %q, want %q", s, got, TmuxScope)
		}
	}
}

// Two instances on different sockets never share a unit, and one instance
// always gets the same one.
func TestEverySocketHasItsOwnScope(t *testing.T) {
	uid := os.Getuid()
	a := TmuxScopeFor(resolveTmuxSocket("", t.TempDir(), uid))
	b := TmuxScopeFor(resolveTmuxSocket("", t.TempDir(), uid))
	if a == b {
		t.Fatalf("two sockets were given one scope, %q", a)
	}
	for _, n := range []string{a, b} {
		if n == TmuxScope {
			t.Errorf("a non-default socket took the live service's scope")
		}
		if !unitName.MatchString(n) || len(n)+len(".scope") > 256 {
			t.Errorf("%q is not a valid unit name", n)
		}
	}
	dir := t.TempDir()
	if x, y := TmuxScopeFor(resolveTmuxSocket("", dir, uid)), TmuxScopeFor(resolveTmuxSocket("", dir+"/", uid)); x != y {
		t.Errorf("one socket was given two scopes: %q and %q", x, y)
	}
	// A symlink to a directory is that directory to tmux, which takes its
	// realpath.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if x, y := TmuxScopeFor(resolveTmuxSocket("", dir, uid)), TmuxScopeFor(resolveTmuxSocket("", link, uid)); x != y {
		t.Errorf("a symlinked TMUX_TMPDIR was given a scope of its own: %q and %q", x, y)
	}
	// Characters a unit name cannot carry still make one.
	if n := TmuxScopeFor("/tmp/a dir/with é and @/default"); !unitName.MatchString(n) {
		t.Errorf("%q is not a valid unit name", n)
	}
}

// Inside a pane tmux follows $TMUX, whatever TMUX_TMPDIR says.
func TestTheSocketFollowsTMUXFirst(t *testing.T) {
	if got := resolveTmuxSocket("/run/x/sock,42,0", "/tmp", 1000); got != "/run/x/sock" {
		t.Errorf("got %q", got)
	}
	if got := resolveTmuxSocket("", "", 1000); filepath.Base(filepath.Dir(got)) != "tmux-1000" || filepath.Base(got) != "default" {
		t.Errorf("got %q", got)
	}
}

// Under go test the socket is private, so the scope Start would ask for is not
// the live service's even with systemd-run unshadowed.
func TestATestServerDoesNotAskForTheLiveScope(t *testing.T) {
	if n := TmuxScopeFor(tmuxSocket()); n == TmuxScope {
		t.Errorf("a test binary on socket %q would ask for %q", tmuxSocket(), n)
	}
}
