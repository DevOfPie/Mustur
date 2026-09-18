package verify

import (
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/export"
	"github.com/DevOfPie/Mustur/internal/record"
)

// An identifier a record declares retired is known to the check, and resolves
// to nothing, which is the point (MUS-D-0197).
func TestARetiredIdentifierIsNotDangling(t *testing.T) {
	dir := t.TempDir()
	rs := []record.Record{
		{ID: "MUS-D-0192", Kind: "decision", Title: "IDW-F-0001 becomes _IB-F-0001", At: "2026-09-18",
			Refs: []record.Field{{Key: "answers", Value: "MUS-Q-0153"}},
			Data: []record.Field{
				{Key: record.RenamedField, Value: "IDW-F-0001 = _IB-F-0001"},
				{Key: record.RetiredField, Value: "IDW-F-0007 :: named and never issued"},
			}},
		{ID: "MUS-Q-0153", Kind: "question", Title: "IDW-F-0001 to 0006", At: "2026-09-18",
			Body: "IDW-F-0001, and IDW-F-0007 after it. MUS-D-0404 is not retired."},
		{ID: "_IB-F-0001", Kind: "finding", Title: "renamed", At: "2026-09-01"},
	}
	if err := export.Write(dir, rs); err != nil {
		t.Fatal(err)
	}
	problems, _, err := Tree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "MUS-D-0404 is cited") {
		t.Fatalf("problems: %v", problems)
	}
}

// A declaration that does not parse is reported rather than read as declaring
// nothing, which would let a typo pass the check.
func TestAMalformedRetirementIsReported(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "decisions.md", "## MUS-D-0001\n\n| Field | Value |\n| --- | --- |\n"+
		"| Renamed | IDW-F-0001 -> _IB-F-0001 |\n| Retired | IDW-F-0007 |\n")
	problems, _, err := Tree(dir)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(problems, "\n")
	for _, want := range []string{"decisions.md: Renamed", "decisions.md: Retired", "IDW-F-0001 is cited", "IDW-F-0007 is cited"} {
		if !strings.Contains(joined, want) {
			t.Errorf("problems do not mention %q:\n%s", want, joined)
		}
	}
}
