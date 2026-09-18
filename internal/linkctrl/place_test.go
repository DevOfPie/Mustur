package linkctrl

import (
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/status"
)

func lnkProject(words ...string) record.Record {
	r := record.Record{ID: "MUS-P-0004", Kind: "project", Title: "LinkCtrl", At: "2026-09-13",
		Data: []record.Field{{Key: status.PrefixField, Value: Prefix}}}
	for _, w := range words {
		r.Data = append(r.Data, record.Field{Key: status.WordField, Value: w})
	}
	return r
}

// Imported findings take LNK's own words (MUS-D-0196): a section LNK declares
// is that word and its State, and one it does not — Open, here — arrives
// unreviewed and open with the section in its Note. What is written passes
// the findings gate, and a repair straight after changes nothing.
func TestTheImportPlacesFindingsInLNKsList(t *testing.T) {
	s, ctx := openStore(t)
	if err := s.Append(ctx, lnkProject("unreviewed = open :: not triaged", "closed = done :: closed in LinkCtrl"), "create", "test"); err != nil {
		t.Fatal(err)
	}
	findings, err := Findings(strings.NewReader(findingsFixture), "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(ctx, s, []Source{{Records: findings}}); err != nil {
		t.Fatal(err)
	}
	all, err := s.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range all {
		if r.Kind != "finding" {
			continue
		}
		note, _ := r.Get("Note")
		seen[status.WordOf(r)+"/"+status.StateOf(r)+"/"+note] = true
	}
	for _, want := range []string{"unreviewed/open/LinkCtrl section: open", "closed/done/"} {
		if !seen[want] {
			t.Errorf("no finding placed as %q; got %v", want, seen)
		}
	}
	if problems := status.Check(all, Prefix); len(problems) != 0 {
		t.Errorf("the import wrote what the gate rejects: %v", problems)
	}
	if amended, _, created, err := Repair(ctx, s, []Source{{Records: findings}}); err != nil || len(amended)+len(created) != 0 {
		t.Errorf("a repair after the import: amended %v, created %v, %v", amended, created, err)
	}
}

// Where LNK's list cannot place a finding at all, the import writes nothing
// rather than something the gate rejects.
func TestTheImportRefusesWhatLNKsListCannotPlace(t *testing.T) {
	s, ctx := openStore(t)
	if err := s.Append(ctx, lnkProject("fixed = done :: repaired"), "create", "test"); err != nil {
		t.Fatal(err)
	}
	findings, err := Findings(strings.NewReader(findingsFixture), "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(ctx, s, []Source{{Records: findings}}); err == nil || !strings.Contains(err.Error(), "nothing was") {
		t.Errorf("err = %v", err)
	}
	if n, _ := s.Count(ctx); n != 1 {
		t.Errorf("%d records after a refused import, want the project alone", n)
	}
}
