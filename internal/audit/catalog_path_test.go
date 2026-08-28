package audit

import (
	"os"
	"path/filepath"
	"testing"
)

// "Beside the audited tree" has to mean beside the repository, not beside
// whatever directory the run started in.
//
// Agent work here happens in a git worktree under .claude/worktrees/<name>, so
// the parent is .claude/worktrees and there is no checkout of anything there.
// The conformance harness skipped on that, and it skipped on every agent run —
// which is every run (MUS-F-0061).
func TestTheCatalogIsFoundFromInsideAWorktree(t *testing.T) {
	home := t.TempDir()
	catalog := filepath.Join(home, "StrucGu")
	if err := os.MkdirAll(filepath.Join(catalog, "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(home, "Mustur")
	worktree := filepath.Join(repo, ".claude", "worktrees", "some-branch")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}

	// The ordinary checkout, which must keep answering as it always did.
	if got := CatalogBeside(repo); got != catalog {
		t.Errorf("from the repository: %s, want %s", got, catalog)
	}
	// And from three levels down, which is where the work happens.
	if got := CatalogBeside(worktree); got != catalog {
		t.Errorf("from a worktree: %s, want %s", got, catalog)
	}
}

// A directory that merely shares the name is not a catalog. Without the modules
// check the walk stops at the first thing called StrucGu and audits against
// nothing.
func TestADirectoryNamedStrucGuIsNotACatalog(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "Mustur")
	// Named right, empty inside.
	if err := os.MkdirAll(filepath.Join(repo, "StrucGu"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(repo, ".claude", "worktrees", "wt")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	// Nothing anywhere is a real catalog, so it names the obvious place rather
	// than the last directory it happened to look in.
	want := filepath.Join(filepath.Dir(nested), "StrucGu")
	if got := CatalogBeside(nested); got != want {
		t.Errorf("with no catalog anywhere: %s, want the sibling %s", got, want)
	}
}
