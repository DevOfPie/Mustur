package linkctrl

import (
	"strings"
	"testing"
)

// MUS-D-0169: a number cited and defined nowhere takes its place in the order,
// and after the second pass nothing is left unplaced.
func TestACitedUndefinedMilestoneIsStubbedInOrder(t *testing.T) {
	first, renumber, err := Milestones(milestoneFixture(), nil, "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	cited := Unplaced([]Source{{Records: first}}, renumber)
	// Four files cite it, each read as a milestone and as its work unit.
	if c := cited["24.6"]; c.Count != 8 || c.At != "2026-08-01" {
		t.Fatalf("M24.6 cited %+v, want 8 times from 2026-08-01", c)
	}
	if len(cited) != 1 {
		t.Fatalf("unplaced %v", cited)
	}

	ms, renumber, err := Milestones(milestoneFixture(), cited, "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	if renumber["24.5"] != 5 || renumber["24.6"] != 6 || renumber["25"] != 7 {
		t.Fatalf("renumbering %v", renumber)
	}
	m := byID(ms)
	stub := m["LNK-M-0006"]
	if v, _ := stub.Get("LinkCtrl"); v != "M24.6" || !strings.Contains(stub.Body, "8 time(s)") {
		t.Fatalf("stub %+v", stub)
	}
	if _, ok := m["LNK-W-0006"]; ok {
		t.Fatal("a stub got a work unit")
	}

	sources := []Source{{Records: ms}}
	if _, left := Rewrite(sources, renumber); len(left) != 0 {
		t.Fatalf("still unplaced after stubbing: %v", left)
	}
	if got := byID(sources[0].Records)["LNK-M-0006"]; !strings.HasPrefix(got.Title, "`M24.6`") {
		t.Fatalf("the stub renamed itself: %q", got.Title)
	}
}
