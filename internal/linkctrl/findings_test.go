package linkctrl

import (
	"strings"
	"testing"
)

const findingsFixture = `# Deferred findings

## Open

| # | Found in | Finding | Where | Evidence | Severity | Reviewed |
| --- | --- | --- | --- | --- | --- | --- |
| F382 | M70 | **Tests asserting a timing bound fail under load.** More prose, see F90. | ` + "`Makefile`" + ` | ` + "`git ls-files -z \\| xargs -0 grep x`" + ` | Low | **Unreviewed.** |

## Closed

| # | Found in | Finding | Where | Evidence | Severity | Reviewed | Closed by |
| --- | --- | --- | --- | --- | --- | --- | --- |
| F1 | 0.1.0 release | Release-notes extraction sweeps up the wrong section. It was fixed. | x | y | Minor | Yes — owner approved 2026-07-31, scheduled as [M45](phase-details/m45.md) | **[M45](phase-details/m45.md)**, 2026-08-06 |
`

func TestFindingsKeepNumbersSectionsAndEscapedPipes(t *testing.T) {
	got, err := Findings(strings.NewReader(findingsFixture), "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%d findings", len(got))
	}
	open, closed := got[0], got[1]
	if open.ID != "LNK-F-0382" || closed.ID != "LNK-F-0001" {
		t.Fatalf("identifiers %s, %s", open.ID, closed.ID)
	}
	if open.Title != "Tests asserting a timing bound fail under load" {
		t.Fatalf("title %q", open.Title)
	}
	if v, _ := open.Get("Evidence"); !strings.Contains(v, "-z | xargs") {
		t.Fatalf("escaped pipe split the cell: %q", v)
	}
	if v, _ := open.Get("Dated"); v == "" || open.At != "2026-09-13" {
		t.Fatalf("an undated row was not marked: at %s", open.At)
	}
	if v, _ := closed.Get("Status"); v != "closed" {
		t.Fatalf("status %q", v)
	}
	if closed.At != "2026-07-31" {
		t.Fatalf("at %s, want the earliest date in the row", closed.At)
	}
	if v, _ := closed.Get("Closed by"); strings.Contains(v, "](") {
		t.Fatalf("a link into LinkCtrl's tree kept its target: %q", v)
	}
}

func TestFindingsRefuseADuplicateNumber(t *testing.T) {
	dup := findingsFixture + "| F1 | a | b | c | d | e | f | g |\n"
	if _, err := Findings(strings.NewReader(dup), "2026-09-13"); err == nil {
		t.Fatal("a second F1 was accepted")
	}
}
