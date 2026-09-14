package linkctrl

import (
	"strings"
	"testing"
)

// Found by the review of LinkCtrl PR 14: a wrapped header was cut at its first
// line break, no milestone said which phase it belonged to, and nothing led
// from a milestone to its work unit.
func TestMilestoneFieldsSurviveWrappingAndSayTheirPhase(t *testing.T) {
	src := milestoneFixture()
	src.Files["m45.md"] = "# M45 — The release\n\n**Depends on:** every milestone, and [M44.9](m44.9.md) having run.\n**Discharges:** the phase-end process obligations in\n[workflow.md](../workflow.md); the dangling reference; the\ncomment-truth sweep.\n\nWritten 2026-08-06.\n\n## Done means\n\n- Released.\n"
	src.Phases["phase-3.md"] = "| # | Milestone | Depends on | Discharges |\n| --- | --- | --- | --- |\n| [M45](m45.md) | The release | all | x |\n\n| # | Milestone | Depends on | Status |\n| --- | --- | --- | --- |\n| [M25](m25.md) | Twenty-five | — | done |\n| [M45](m45.md) | The release | all | done |\n"
	got, renumber, err := Milestones(src, nil, "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	m := byID(got)
	m45 := m[idOf(renumber["45"])]
	if v, _ := m45.Get("Discharges"); v != "the phase-end process obligations in workflow.md; the dangling reference; the comment-truth sweep." {
		t.Fatalf("wrapped discharges %q", v)
	}
	if v, _ := m45.Get("Depends on"); v != "every milestone, and M44.9 having run." {
		t.Fatalf("depends on %q", v)
	}
	if p, _ := m45.Get("Phase"); p != "3" {
		t.Fatalf("M45's phase %q", p)
	}
	if o, _ := m45.Get("Phase order"); o != "2" {
		t.Fatalf("M45's phase order %q, want 2: the build-plan table is not the status table", o)
	}
	if p, _ := m[idOf(renumber["24.5"])].Get("Phase"); p != "2" {
		t.Fatalf("M24.5's phase %q", p)
	}
	if p, _ := m[idOf(renumber["18"])].Get("Phase"); p != "1" {
		t.Fatalf("M18's phase %q", p)
	}
	if len(m45.Refs) == 0 || !strings.HasPrefix(m45.Refs[0].Value, "LNK-W-") || m45.Refs[0].Key != "work-unit" {
		t.Fatalf("M45 does not lead to its work unit: %v", m45.Refs)
	}
}

// A milestone file with no date anywhere is dated the day it is read, and its
// work unit has to say so too, or a repair on another day restamps it.
func TestAnUndatedWorkUnitSaysItWasDatedOnImport(t *testing.T) {
	src := milestoneFixture()
	src.Files["m26.md"] = "# M26 — Undated\n\n**Depends on:** nothing.\n\n## Done means\n\n- Done.\n"
	got, renumber, err := Milestones(src, nil, "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	serial := idOf(renumber["26"])[len("LNK-M-"):]
	wu := byID(got)["LNK-W-"+serial]
	if wu.At != "2026-09-13" {
		t.Fatalf("work unit at %q", wu.At)
	}
	if _, ok := wu.Get("Dated"); !ok {
		t.Fatal("an undated work unit does not say it was dated on import")
	}
}

func idOf(serial int) string {
	return "LNK-M-" + strings.Repeat("0", 4-len(itoa(serial))) + itoa(serial)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
