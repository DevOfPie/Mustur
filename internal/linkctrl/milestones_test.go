package linkctrl

import (
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

func milestoneFixture() MilestoneSources {
	file := func(n, title string) string {
		return "# M" + n + " — " + title + "\n\n**Depends on:** [M21](m21.md). **Discharges:** F9.\n\nWritten 2026-08-01.\n\n## Done means\n\n- It works, unlike M24.6.\n\n## Risks\n\n- None.\n"
	}
	return MilestoneSources{
		Files: map[string]string{
			"m21.md":       file("21", "Audit log"),
			"m24.md":       file("24", "Twenty-four"),
			"m24.5.md":     file("24.5", "The added scope"),
			"m25.md":       file("25", "Twenty-five"),
			"_template.md": "# M<N> — <title>\n",
		},
		Phase1: "| Milestone | State |\n| --- | --- |\n| **M18 — separate hostnames** | **Done.** |\n| **M20 — root redirect** | **Done.** Verified. |\n\n## M20 in detail\n\nThe redirect.\n\n## M19 in detail\n\nNot a row here.\n",
		Phases: map[string]string{"phase-2.md": "| # | Milestone | Depends on | Status |\n| --- | --- | --- | --- |\n| [M24.5](m24.5.md) | The added scope | — | done |\n"},
		Plan:   "| # | Milestone | Depends on | Discharges |\n| --- | --- | --- | --- |\n| [M25](docs/build-notes/phase-details/m25.md) | Twenty-five | M24.5 | F12 |\n",
	}
}

func TestMilestonesRenumberInLinkCtrlOrder(t *testing.T) {
	got, renumber, err := Milestones(milestoneFixture(), "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"18": 1, "20": 2, "21": 3, "24": 4, "24.5": 5, "25": 6}
	if len(renumber) != len(want) {
		t.Fatalf("renumbering %v", renumber)
	}
	for old, serial := range want {
		if renumber[old] != serial {
			t.Fatalf("M%s took %d, want %d", old, renumber[old], serial)
		}
	}
	m := byID(got)
	half := m["LNK-M-0005"]
	if half.Title != "The added scope" || half.At != "2026-08-01" {
		t.Fatalf("M24.5: %q at %s", half.Title, half.At)
	}
	if v, _ := half.Get("LinkCtrl"); v != "M24.5" {
		t.Fatalf("old number field %q", v)
	}
	if v, _ := half.Get("Status"); v != "done" {
		t.Fatalf("status %q", v)
	}
	if !strings.Contains(half.Body, "It works") || strings.Contains(half.Body, "None.") {
		t.Fatalf("done-when section %q", half.Body)
	}
	if v, _ := half.Get("Depends on"); v != "M21." {
		t.Fatalf("depends on %q", v)
	}
	if wu := m["LNK-W-0005"]; len(wu.Refs) != 1 || wu.Refs[0].Value != "LNK-M-0005" {
		t.Fatalf("work unit for M24.5: %+v", wu)
	}
	if _, ok := m["LNK-W-0001"]; ok {
		t.Fatal("M18 has no file and got a work unit")
	}
	if b := m["LNK-M-0002"].Body; b != "The redirect." {
		t.Fatalf("M20's detail section: %q", b)
	}
	if v, _ := m["LNK-M-0006"].Get("Plan.md row"); v != "F12" {
		t.Fatalf("plan row %q", v)
	}
}

func TestRewritePointsReferencesAtTheNewNumber(t *testing.T) {
	ms, renumber, err := Milestones(milestoneFixture(), "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	finding := record.Record{
		ID: "LNK-F-0001", Kind: "finding", Title: "Found at M24.5", At: "2026-08-01",
		Body: "See M21 and M59, then `make M24` and\n```\nM25\n```\nand M25.",
		Data: []record.Field{{Key: "Found in", Value: "M24 *(reopened)*"}, {Key: "LinkCtrl", Value: "M24"}},
	}
	sources := []Source{{Records: []record.Record{finding}}, {Records: ms}}
	n, unresolved := Rewrite(sources, renumber)
	got := sources[0].Records[0]
	if got.Title != "Found at LNK-M-0005" {
		t.Fatalf("title %q", got.Title)
	}
	if !strings.Contains(got.Body, "See LNK-M-0003 and M59") || !strings.Contains(got.Body, "`make M24`") ||
		!strings.Contains(got.Body, "```\nM25\n```") || !strings.HasSuffix(got.Body, "and LNK-M-0006.") {
		t.Fatalf("body %q", got.Body)
	}
	if v, _ := got.Get("Found in"); v != "LNK-M-0004 *(reopened)*" {
		t.Fatalf("found in %q", v)
	}
	if v, _ := got.Get("LinkCtrl"); v != "M24" {
		t.Fatalf("the old-number field was rewritten: %q", v)
	}
	if unresolved["M59"] != 1 || unresolved["M24.6"] == 0 {
		t.Fatalf("unresolved %v", unresolved)
	}
	if n == 0 {
		t.Fatal("nothing rewritten")
	}
	if again, _ := Rewrite(sources, renumber); again != 0 {
		t.Fatalf("a second pass rewrote %d more", again)
	}
}
