package linkctrl

import (
	"strings"
	"testing"
)

// MUS-D-0173: a phase keeps its prose and the tables nothing else holds, drops
// the ones held on milestones and decisions with a line saying where, and cites
// its milestones in its status table's order.
func TestPhasesKeepTheirProseAndCiteTheirMilestones(t *testing.T) {
	src := milestoneFixture()
	src.Phases["phase-2.md"] = "# Phase 2 — the milestones\n\nOpened 2026-07-31.\n\n" +
		"| # | Milestone | Depends on | Status |\n| --- | --- | --- | --- |\n" +
		"| [M25](m25.md) | Twenty-five | — | done |\n| [M24.5](m24.5.md) | The added scope | — | done |\n\n" +
		"### Phase 2 decisions\n\n| # | Decision | Outcome |\n| --- | --- | --- |\n| D1 | Mailer | Ships. |\n\n" +
		"### Not in Phase 2\n\n| Area | State |\n| --- | --- |\n| SCIM | deferred, with its reason |\n\nSee [M21](m21.md).\n"
	ms, renumber, err := Milestones(src, nil, "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Phases(src, renumber, "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("%d phases", len(got))
	}
	p := got[0]
	if p.ID != "LNK-S-0002" || p.Kind != "phase" || p.Title != "Phase 2 — the milestones" || p.At != "2026-07-31" {
		t.Fatalf("phase %s %s %q at %s", p.ID, p.Kind, p.Title, p.At)
	}
	if strings.Contains(p.Body, "| Mailer |") || strings.Contains(p.Body, "| Twenty-five |") {
		t.Fatalf("a table held elsewhere stayed in the body: %q", p.Body)
	}
	if !strings.Contains(p.Body, "| SCIM | deferred, with its reason |") || !strings.Contains(p.Body, "#### Not in Phase 2") {
		t.Fatalf("the phase's own prose or table was lost: %q", p.Body)
	}
	if strings.Count(p.Body, "*This table's rows are held on") != 2 {
		t.Fatalf("a dropped table left no line saying where it went: %q", p.Body)
	}
	want := []string{idOf(renumber["25"]), idOf(renumber["24.5"])}
	if len(p.Refs) != 2 || p.Refs[0].Value != want[0] || p.Refs[1].Value != want[1] {
		t.Fatalf("milestone refs %v, want %v in status-table order", p.Refs, want)
	}
	if s, _ := p.Get("Status"); !strings.HasPrefix(s, "closed") {
		t.Fatalf("status %q", s)
	}
	for _, r := range ms {
		if r.ID != want[0] {
			continue
		}
		found := false
		for _, f := range r.Refs {
			if f.Key == "phase" && f.Value == "LNK-S-0002" {
				found = true
			}
		}
		if !found {
			t.Fatalf("M25 does not cite its phase: %v", r.Refs)
		}
	}
}
