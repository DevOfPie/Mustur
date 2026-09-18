package store

import (
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

// A kept record keeps its old identifiers, and only a declaring record that
// cites it shows them as plain text there. Kept outside that scope, they would
// link to Idea Warehouse's records once the spelling is issued again, so
// --apply refuses (review of #107, finding 5).
func TestAKeptRecordOutsideThePlainScopeIsRefused(t *testing.T) {
	s, ctx := renameFixture(t)
	outside := record.Record{ID: "MUS-D-0197", Kind: "decision", Title: "Retired", At: "2026-09-18",
		Body: "IDW-F-0001 is retired."}
	if err := s.Append(ctx, outside, "create", "test"); err != nil {
		t.Fatal(err)
	}
	before := dump(t, s)
	keep := []string{"MUS-D-0192", "MUS-D-0197"}
	report, err := s.Rename(ctx, intakeBox, RenameOptions{Keep: keep})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(report.Unscoped, "; ") != "MUS-D-0197: IDW-F-0001" {
		t.Fatalf("unscoped %v", report.Unscoped)
	}
	_, err = s.Rename(ctx, intakeBox, RenameOptions{Keep: keep, Apply: true})
	if err == nil || !strings.Contains(err.Error(), "MUS-D-0197: IDW-F-0001") {
		t.Fatalf("apply gave %v", err)
	}
	if dump(t, s) != before {
		t.Fatal("a refused apply wrote")
	}

	// Cited by the declaring record, it is inside the scope and the rename runs.
	carrier, _ := s.Get(ctx, "MUS-D-0192")
	carrier.Refs = append(carrier.Refs, record.Field{Key: "describes", Value: "MUS-D-0197"})
	if err := s.Append(ctx, carrier, "amend", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rename(ctx, intakeBox, RenameOptions{Keep: keep, Apply: true}); err != nil {
		t.Fatal(err)
	}
	if kept, _ := s.Get(ctx, "MUS-D-0197"); kept.Body != "IDW-F-0001 is retired." {
		t.Errorf("a kept record was rewritten: %q", kept.Body)
	}
}

// A kept record that cites no old identifier is probably a typo, and is
// reported rather than refused.
func TestAKeptRecordCitingNothingOldIsReported(t *testing.T) {
	s, ctx := renameFixture(t)
	report, err := s.Rename(ctx, intakeBox, RenameOptions{Keep: []string{"MUS-D-0192", "IDW-F-0005"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(report.Idle, ",") != "IDW-F-0005" {
		t.Errorf("idle %v, want IDW-F-0005", report.Idle)
	}
}
