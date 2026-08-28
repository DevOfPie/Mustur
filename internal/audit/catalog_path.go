package audit

// Finding the catalog when the tree being audited is a worktree.
//
// The rule is "a StrucGu checkout beside the audited tree", and it was read as
// the parent of whatever directory it was handed. Agent work in this repository
// happens in a git worktree under `.claude/worktrees/<name>`, so the parent is
// `.claude/worktrees` and there is no StrucGu there. The conformance harness
// then skipped — loudly, which is the one thing it got right — and it skipped
// on every agent run, which is every run (MUS-F-0061).
//
// So "beside" walks up. The first ancestor with a StrucGu holding modules wins,
// which is the same answer as before for an ordinary checkout and the right one
// from any depth of worktree. A catalog is recognised by having modules in it
// rather than by its name alone, so a directory that merely shares the name is
// not mistaken for one.

import (
	"os"
	"path/filepath"
)

// CatalogBeside finds a StrucGu checkout beside the audited tree or above it.
//
// It returns the nearest one that looks like a catalog. With none anywhere it
// returns the sibling path, so a caller's error names the obvious place to put
// one rather than the last directory this happened to look in.
func CatalogBeside(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return filepath.Join("..", "StrucGu")
	}
	sibling := filepath.Join(filepath.Dir(abs), "StrucGu")
	for dir := abs; ; {
		candidate := filepath.Join(filepath.Dir(dir), "StrucGu")
		if fi, err := os.Stat(filepath.Join(candidate, "modules")); err == nil && fi.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return sibling
		}
		dir = parent
	}
}
