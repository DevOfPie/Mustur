package store

import (
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

// An old identifier spelled inside something longer is not a citation and is
// left alone — but it is reported, and --apply refuses until told it is not
// one (review of #107, finding 2). What reads as a citation is rewritten,
// including in italics and bold.
func TestARenameReportsWhatItCannotReadAndRewritesItalics(t *testing.T) {
	s, ctx := renameFixture(t)
	add := func(id, body string) {
		t.Helper()
		r := record.Record{ID: id, Kind: "finding", Title: "prose", At: "2026-09-04", Body: body}
		if err := s.Append(ctx, r, "create", "test"); err != nil {
			t.Fatal(err)
		}
	}
	add("MUS-F-0101", "IDW-F-00010 and XIDW-F-0001 are not IDW-F-0001.")
	add("MUS-F-0102", "_IDW-F-0001_ and __IDW-F-0004__ and x_IDW-F-0001 and IDW-F-0001_x.")
	before := dump(t, s)

	report, err := s.Rename(ctx, intakeBox, RenameOptions{Keep: []string{"MUS-D-0192"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Unmatched) != 1 || report.Unmatched[0].Record != "MUS-F-0101" ||
		strings.Join(report.Unmatched[0].Where, ",") != "body" {
		t.Fatalf("unmatched %+v, want MUS-F-0101's body only", report.Unmatched)
	}

	_, err = s.Rename(ctx, intakeBox, RenameOptions{Keep: []string{"MUS-D-0192"}, Apply: true})
	if err == nil || !strings.Contains(err.Error(), "--accept-unmatched") {
		t.Fatalf("apply with an unmatched row gave %v", err)
	}
	if dump(t, s) != before {
		t.Fatal("a refused apply wrote")
	}

	if _, err := s.Rename(ctx, intakeBox, RenameOptions{Keep: []string{"MUS-D-0192"}, Apply: true, AcceptUnmatched: true}); err != nil {
		t.Fatal(err)
	}
	longer, _ := s.Get(ctx, "MUS-F-0101")
	if longer.Body != "IDW-F-00010 and XIDW-F-0001 are not _IB-F-0001." {
		t.Errorf("body %q", longer.Body)
	}
	italic, _ := s.Get(ctx, "MUS-F-0102")
	if want := "__IB-F-0001_ and ___IB-F-0004__ and x__IB-F-0001 and _IB-F-0001_x."; italic.Body != want {
		t.Errorf("body %q, want %q", italic.Body, want)
	}
}
