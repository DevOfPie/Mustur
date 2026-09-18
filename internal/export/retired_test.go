package export

import (
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

// retiredRecords is MUS-D-0197's case after Idea Warehouse has issued its own
// IDW-F-0001: the renaming decision, the question it answers citing the old
// identifier, the renamed jot, the new IDW-F-0001, and a record elsewhere that
// cites IDW-F-0001 and means Idea Warehouse's.
func retiredRecords() []record.Record {
	return []record.Record{
		{ID: "MUS-D-0192", Kind: "decision", Title: "Renamed in place", At: "2026-09-18",
			Refs: []record.Field{{Key: "answers", Value: "MUS-Q-0153"}},
			Body: "IDW-F-0001 became [the box's first](findings.md#_ib-f-0001).",
			Data: []record.Field{
				{Key: record.RenamedField, Value: "IDW-F-0001 = _IB-F-0001"},
				{Key: record.RetiredField, Value: "IDW-F-0007 :: named and never issued"},
			}},
		{ID: "MUS-Q-0153", Kind: "question", Title: "What happens to IDW-F-0001?", At: "2026-09-18",
			Refs: []record.Field{{Key: "about", Value: "IDW-F-0001"}},
			Body: "See [the jot](findings.md#idw-f-0001) and IDW-F-0007."},
		{ID: "_IB-F-0001", Kind: "finding", Title: "The box's first", At: "2026-09-01"},
		{ID: "IDW-F-0001", Kind: "finding", Title: "Idea Warehouse's first", At: "2026-09-20"},
		{ID: "MUS-F-0009", Kind: "finding", Title: "Elsewhere", At: "2026-09-21",
			Refs: []record.Field{{Key: "about", Value: "IDW-F-0001"}},
			Body: "See [the idea](findings.md#idw-f-0001)."},
	}
}

func section(t *testing.T, file, id string) string {
	t.Helper()
	_, after, ok := strings.Cut(file, "## "+id+"\n")
	if !ok {
		t.Fatalf("%s is not in the file", id)
	}
	if end := strings.Index(after, "\n---\n"); end >= 0 {
		after = after[:end]
	}
	return after
}

func TestARetiredIdentifierIsPlainWhereTheRenameIsDescribed(t *testing.T) {
	files, err := Render(retiredRecords())
	if err != nil {
		t.Fatal(err)
	}
	q := section(t, string(files["questions.md"]), "MUS-Q-0153")
	if !strings.Contains(q, "about: IDW-F-0001\n") || strings.Contains(q, "[IDW-F-0001]") {
		t.Errorf("the question links a retired identifier:\n%s", q)
	}
	if !strings.Contains(q, "See the jot and IDW-F-0007.") {
		t.Errorf("the question's body link to a retired identifier survived:\n%s", q)
	}
	d := section(t, string(files["decisions.md"]), "MUS-D-0192")
	if !strings.Contains(d, "[MUS-Q-0153](questions.md#mus-q-0153)") {
		t.Errorf("the carrier's own citation of the question stopped linking:\n%s", d)
	}
	if !strings.Contains(d, "[the box's first](findings.md#_ib-f-0001)") {
		t.Errorf("a link to the new identifier was unlinked:\n%s", d)
	}
	// Elsewhere, the same spelling links to the record that holds it now.
	f := section(t, string(files["findings.md"]), "MUS-F-0009")
	if !strings.Contains(f, "about: [IDW-F-0001](#idw-f-0001)") || !strings.Contains(f, "[the idea](findings.md#idw-f-0001)") {
		t.Errorf("a record outside the rename lost its link to IDW-F-0001:\n%s", f)
	}
}
