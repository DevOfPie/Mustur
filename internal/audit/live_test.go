package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// catalogPath is the StrucGu checkout these tests read: MUSTUR_STRUCGU if it is
// set, and a checkout beside this repository otherwise. The command honours the
// same variable, and a test that ignored it would be unrunnable exactly where
// the documentation says to point it.
//
// The unit tests do not need it — they carry their own catalog — but the
// conformance harness does, and a skip there is a hole in the evidence rather
// than an absent nicety. So the skip says loudly what did not run: silence and
// "did not look" are the same string here too.
func catalogPath(t *testing.T) string {
	t.Helper()
	path := os.Getenv("MUSTUR_STRUCGU")
	if path == "" {
		path = CatalogBeside("../..")
	}
	if _, err := os.Stat(filepath.Join(path, "modules")); err != nil {
		t.Skipf("NO CONFORMANCE EVIDENCE IN THIS RUN: no StrucGu catalog at %s. "+
			"Set MUSTUR_STRUCGU or clone DevOfPie/StrucGu beside this repository.", path)
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
