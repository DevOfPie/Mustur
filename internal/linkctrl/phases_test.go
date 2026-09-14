package linkctrl

import (
	"strings"
	"testing"
)

// MUS-D-0173 and MUS-D-0179: a phase keeps its file whole, tables included,
// cites its milestones in its status table's order, is dated by the earliest
// of them rather than by any date a row mentions, and is closed only when
// every milestone it holds is done.
func TestPhasesKeepTheirFileAndCiteTheirMilestones(t *testing.T) {
	src := milestoneFixture()
	src.Phases["phase-2.md"] = "# Phase 2 — the milestones\n\nOpened 2026-07-31.\n\n" +
		"| # | Milestone | Depends on | Status |\n| --- | --- | --- | --- |\n" +
		"| [M25](m25.md) | Twenty-five | — | done |\n| [M24.5](m24.5.md) | The added scope | *(see the note below)* | done |\n\n" +
		"| # | Milestone | Depends on | Discharges |\n| --- | --- | --- | --- |\n| [M25](m25.md) | Twenty-five | — | Reopened four times |\n\n" +
		"### Phase 2 decisions\n\n| # | Decision | Taken | Outcome |\n| --- | --- | --- | --- |\n| D72 | QR encoder | 2026-08-03 | a library first published 2018-06-05 |\n\n" +
		"### Not in Phase 2\n\n| Area | State |\n| --- | --- |\n| SCIM | deferred, with its reason |\n\nSee [M21](m21.md).\n"
	ms, renumber, err := Milestones(src, nil, "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Phases(src, renumber, ms, "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]int{}
	for i, p := range got {
		byID[p.ID] = i
	}
	p := got[byID["LNK-S-0002"]]
	if p.Kind != "phase" || p.Title != "Phase 2 — the milestones" {
		t.Fatalf("phase %s %s %q", p.ID, p.Kind, p.Title)
	}
	if p.At != "2026-08-01" {
		t.Fatalf("phase 2 at %s, want its earliest milestone's date, not 2018 from a row", p.At)
	}
	for _, kept := range []string{"*(see the note below)*", "Reopened four times", "| D72 | QR encoder | 2026-08-03 |", "| SCIM | deferred, with its reason |", "#### Not in Phase 2"} {
		if !strings.Contains(p.Body, kept) {
			t.Fatalf("the phase lost %q: %q", kept, p.Body)
		}
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

// Phase 1 has no Status column: its milestones' state is the bold rows of its
// own table, and a row that is not done leaves the phase live.
func TestPhaseOnesStatusIsReadFromItsStateColumn(t *testing.T) {
	src := milestoneFixture()
	// The shared fixture's phase-2.md is only a status table; as a phase file it
	// needs the heading every phase file carries.
	src.Phases["phase-2.md"] = "# Phase 2 — the milestones\n\n" + src.Phases["phase-2.md"]
	src.Phases["phase-1.md"] = "# Phase 1 — after the review\n\n" + src.Phase1
	ms, renumber, err := Milestones(src, nil, "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	closedPhases, err := Phases(src, renumber, ms, "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := closedPhases[0].Get("Status"); !strings.HasPrefix(s, "closed") {
		t.Fatalf("every phase 1 row is done and the phase reads %q", s)
	}

	src.Phase1 = strings.Replace(src.Phase1, "| **M20 — root redirect** | **Done.** Verified. |", "| **M20 — root redirect** | In progress. |", 1)
	src.Phases["phase-1.md"] = "# Phase 1 — after the review\n\n" + src.Phase1
	ms, renumber, err = Milestones(src, nil, "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	livePhases, err := Phases(src, renumber, ms, "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := livePhases[0].Get("Status"); s != "live" {
		t.Fatalf("a phase 1 row in progress and the phase reads %q", s)
	}
}
