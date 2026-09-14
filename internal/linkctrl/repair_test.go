package linkctrl

import (
	"strings"
	"testing"
)

// A repair re-states what the import owns and never what somebody has written
// since: LNK-Q-0002 carries the owner's answer, and replacing it with the
// imported version would erase it.
func TestRepairAmendsOnlyWhatTheImportStillOwns(t *testing.T) {
	s, ctx := openStore(t)
	findings, err := Findings(strings.NewReader(findingsFixture), "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(ctx, s, []Source{{Records: findings}}); err != nil {
		t.Fatal(err)
	}
	owner := findings[1]
	owner.Title = "corrected by a person"
	if err := s.Append(ctx, owner, "amend", "whippy"); err != nil {
		t.Fatal(err)
	}

	reread, err := Findings(strings.NewReader(findingsFixture), "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	reread[0].Title = "read better"
	reread[1].Title = "read better too"
	absent := reread[0]
	absent.ID = "LNK-F-0999"
	src := []Source{{Records: append(reread, absent)}}

	amended, skipped, missing, err := Repair(ctx, s, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(amended) != 1 || amended[0] != "LNK-F-0382" {
		t.Fatalf("amended %v, want only LNK-F-0382", amended)
	}
	if len(skipped) != 1 || skipped[0] != "LNK-F-0001" {
		t.Fatalf("skipped %v, want the record a person wrote since", skipped)
	}
	if len(missing) != 1 || missing[0] != "LNK-F-0999" {
		t.Fatalf("missing %v", missing)
	}
	if got, _ := s.Get(ctx, "LNK-F-0001"); got.Title != "corrected by a person" {
		t.Fatalf("the person's correction was replaced: %q", got.Title)
	}
	if got, _ := s.Get(ctx, "LNK-F-0382"); got.Title != "read better" {
		t.Fatalf("the repair did not land: %q", got.Title)
	}
	if again, _, _, err := Repair(ctx, s, src); err != nil || len(again) != 0 {
		t.Fatalf("a second repair amended %v, %v", again, err)
	}
}

// LNK-F-0382's row carries no date, so it is stamped the day it is read. A
// repair the next day is not a change to it.
func TestRepairOnAnotherDayKeepsTheImportsDate(t *testing.T) {
	s, ctx := openStore(t)
	findings, err := Findings(strings.NewReader(findingsFixture), "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(ctx, s, []Source{{Records: findings}}); err != nil {
		t.Fatal(err)
	}
	nextDay, err := Findings(strings.NewReader(findingsFixture), "2026-09-14")
	if err != nil {
		t.Fatal(err)
	}
	amended, _, _, err := Repair(ctx, s, []Source{{Records: nextDay}})
	if err != nil || len(amended) != 0 {
		t.Fatalf("a repair a day later amended %v, %v", amended, err)
	}
}
