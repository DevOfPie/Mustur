package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// catalogPath is a StrucGu checkout beside this repository, if there is one.
// The unit tests below do not need it — they carry their own catalog — but a
// run against the real modules is worth having when the checkout is there, and
// is skipped rather than failed when it is not.
func catalogPath(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(root), "StrucGu")
	if _, err := os.Stat(filepath.Join(path, "modules")); err != nil {
		t.Skipf("no StrucGu catalog at %s", path)
	}
	return path
}

func TestAgainstThisRepository(t *testing.T) {
	cat, err := LoadCatalog(catalogPath(t))
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Run(root, cat, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rep.Results {
		t.Log(fmt.Sprintf("%-9s %-16s %-34s %s", r.State, r.Module, r.ID, r.Detail))
	}
	if rep.Total() == 0 {
		t.Fatal("the audit produced no results at all")
	}
	// Every judgment entry the adopted modules declare has to appear, whether
	// or not anyone is here to judge it.
	if rep.Counts[NeedsJudgment] == 0 {
		t.Error("no judgment lines: a mechanical run is looking complete")
	}
}
