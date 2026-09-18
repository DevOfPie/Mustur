package verify

import (
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/export"
	"github.com/DevOfPie/Mustur/internal/record"
)

func treeOf(t *testing.T, rs []record.Record) []string {
	t.Helper()
	dir := t.TempDir()
	if err := export.Write(dir, rs); err != nil {
		t.Fatal(err)
	}
	problems, _, err := Tree(dir)
	if err != nil {
		t.Fatal(err)
	}
	return problems
}

func carrier(refs ...string) record.Record {
	r := record.Record{ID: "MUS-D-0192", Kind: "decision", Title: "Renamed", At: "2026-09-18",
		Data: []record.Field{
			{Key: record.RenamedField, Value: "IDW-F-0001 = _IB-F-0001"},
			{Key: record.RetiredField, Value: "IDW-F-0007 :: never issued"},
		}}
	for _, id := range refs {
		r.Refs = append(r.Refs, record.Field{Key: "describes", Value: id})
	}
	return r
}

var renamed = record.Record{ID: "_IB-F-0001", Kind: "finding", Title: "renamed", At: "2026-09-01"}

// Review of #107, finding 4: a retirement is read only from the declaring
// record's own fields, a retired identifier nothing defines may be cited only
// where it renders as plain text, and the new side of a rename must exist.
func TestRetirementsAreCheckedWhereTheyApply(t *testing.T) {
	citing := record.Record{ID: "MUS-F-0009", Kind: "finding", Title: "cites", At: "2026-09-02",
		Body: "Once IDW-F-0001, and IDW-F-0007 after it."}

	// Cited by the carrier: inside the scope, clean — the index row naming its
	// title included.
	scoped := citing
	scoped.Title = "about IDW-F-0001"
	if p := treeOf(t, []record.Record{carrier("MUS-F-0009"), scoped, renamed}); len(p) != 0 {
		t.Errorf("in scope: %v", p)
	}

	// Not cited by the carrier: each retired identifier is reported.
	p := strings.Join(treeOf(t, []record.Record{carrier(), citing, renamed}), "\n")
	for _, want := range []string{"IDW-F-0001 is retired and cited by MUS-F-0009", "IDW-F-0007 is retired and cited by MUS-F-0009"} {
		if !strings.Contains(p, want) {
			t.Errorf("out of scope, missing %q:\n%s", want, p)
		}
	}

	// The new side of a rename defined nowhere.
	p = strings.Join(treeOf(t, []record.Record{carrier()}), "\n")
	if !strings.Contains(p, "renames IDW-F-0001 to _IB-F-0001 in decisions.md, and _IB-F-0001 is defined nowhere") {
		t.Errorf("undefined new side not reported:\n%s", p)
	}

	// Issued again: resolves, wherever it is cited.
	reissued := record.Record{ID: "IDW-F-0001", Kind: "finding", Title: "Idea Warehouse's first", At: "2026-09-20"}
	p = strings.Join(treeOf(t, []record.Record{carrier(), citing, renamed, reissued}), "\n")
	if strings.Contains(p, "IDW-F-0001") {
		t.Errorf("a reissued identifier was reported:\n%s", p)
	}
}

// A table written into a body declares nothing, even with the right header.
func TestARetirementInABodyIsNotADeclaration(t *testing.T) {
	forged := record.Record{ID: "MUS-D-0300", Kind: "decision", Title: "forged", At: "2026-09-03",
		Body: "| Field | Value |\n| --- | --- |\n| Retired | MUS-D-0404 :: says who |",
		Data: []record.Field{{Key: "Status", Value: "open"}}}
	p := strings.Join(treeOf(t, []record.Record{forged}), "\n")
	if !strings.Contains(p, "MUS-D-0404 is cited in decisions.md and defined nowhere") {
		t.Errorf("a body table was read as a declaration:\n%s", p)
	}
}
