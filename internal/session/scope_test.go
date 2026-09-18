package session

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// A unit name as systemd.unit(5) defines it: ASCII letters, digits and
// ":", "-", "_", ".", and at most 255 characters with the ".scope" suffix.
// systemd also allows "\", but only as the escape systemd-escape writes
// (`a b` becomes `a\x20b`); a name built here is hex and never needs one, so a
// backslash in it would be a bug and this refuses it.
var unitName = regexp.MustCompile(`^[A-Za-z0-9:_.-]+$`)

const unitNameMax = 255

// The live service runs on the default socket and must keep the name it has
// always had, or a deploy would leave its running server in a scope nothing
// names (MUS-F-0176). The inputs are literal: resolving the default and
// comparing it with itself would pass whatever resolveTmuxSocket did.
func TestTheDefaultSocketKeepsTheLiveScopeName(t *testing.T) {
	uid := os.Getuid()
	def := fmt.Sprintf("/tmp/tmux-%d/default", uid)
	if real, err := filepath.EvalSymlinks("/tmp"); err != nil || real != "/tmp" {
		t.Skipf("/tmp is %q here (%v), not /tmp", real, err)
	}
	for _, c := range []struct{ tmux, tmpdir string }{
		{"", ""},
		{"", "/tmp"},
		{"", "/tmp/"},
		{"", "/tmp/."},
		{def + ",1234,0", "/elsewhere"},
		{",1234,0", ""},
		// tmux takes the realpath of TMUX_TMPDIR and skips it when that
		// fails, so a directory that does not exist is the default socket.
		{"", "/nonexistent/mustur-f117"},
		// The value is not split on ':' — only tmux's template is — so a
		// colon in it is one path that does not exist, even when both
		// halves do. Measured on tmux 3.6.
		{"", "/nonexistent/mustur-f117:" + t.TempDir()},
		{"", t.TempDir() + ":" + t.TempDir()},
	} {
		got := resolveTmuxSocket(c.tmux, c.tmpdir, uid)
		if got != def {
			t.Errorf("resolveTmuxSocket(%q, %q) = %q, want %q", c.tmux, c.tmpdir, got, def)
		}
		if n := TmuxScopeFor(got); n != TmuxScope {
			t.Errorf("TmuxScopeFor(%q) = %q, want %q", got, n, TmuxScope)
		}
	}
	if n := TmuxScopeFor(""); n != TmuxScope {
		t.Errorf("TmuxScopeFor(\"\") = %q, want %q", n, TmuxScope)
	}
}

// A relative TMUX_TMPDIR is resolved against the working directory, as
// realpath does, so it is the same socket as its absolute form — and two
// working directories are two sockets.
func TestARelativeTMUXTMPDIRIsItsAbsoluteForm(t *testing.T) {
	uid := os.Getuid()
	base := t.TempDir()
	for _, d := range []string{"a/x", "b/x"} {
		if err := os.MkdirAll(filepath.Join(base, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(filepath.Join(base, "a"))
	rel := resolveTmuxSocket("", "x", uid)
	abs := resolveTmuxSocket("", filepath.Join(base, "a", "x"), uid)
	if rel != abs || !filepath.IsAbs(rel) {
		t.Errorf("relative %q, absolute %q", rel, abs)
	}
	t.Chdir(filepath.Join(base, "b"))
	if other := resolveTmuxSocket("", "x", uid); TmuxScopeFor(other) == TmuxScopeFor(rel) {
		t.Errorf("x from two directories was given one scope, %q", TmuxScopeFor(rel))
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
		if !unitName.MatchString(n) || len(n)+len(".scope") > unitNameMax {
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
