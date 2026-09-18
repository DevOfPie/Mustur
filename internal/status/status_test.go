package status

import (
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

func TestParseWordReadsTheShapeTheProjectsCarry(t *testing.T) {
	w, err := ParseWord("in-review = open :: fixed on a pull request that has not merged")
	if err != nil {
		t.Fatal(err)
	}
	if w.Word != "in-review" || w.State != Open || w.Definition != "fixed on a pull request that has not merged" {
		t.Errorf("%+v", w)
	}
	// A definition may itself carry "=" and "::" after the first "::".
	if w, err := ParseWord("merged = dropped :: folded in; carries merged into :: see = there"); err != nil || w.Definition != "folded in; carries merged into :: see = there" {
		t.Errorf("%+v, %v", w, err)
	}
}

func TestParseWordRefusesWhatDeclaresNothing(t *testing.T) {
	for _, v := range []string{
		"fixed = done",                 // no definition separator
		"fixed :: repaired",            // no state
		"fixed = closed :: repaired",   // not a State
		"fixed = done ::   ",           // empty definition
		" = done :: repaired",          // no word
		"two words = done :: repaired", // not one word
	} {
		if _, err := ParseWord(v); err == nil {
			t.Errorf("%q parsed", v)
		}
	}
}

func project(id, prefix string, words ...string) record.Record {
	r := record.Record{ID: id, Kind: "project", Title: prefix, At: "2026-09-18",
		Data: []record.Field{{Key: PrefixField, Value: prefix}}}
	for _, w := range words {
		r.Data = append(r.Data, record.Field{Key: WordField, Value: w})
	}
	return r
}

func finding(id string, fields ...string) record.Record {
	r := record.Record{ID: id, Kind: "finding", Title: "t", At: "2026-09-18"}
	for i := 0; i+1 < len(fields); i += 2 {
		r.Data = append(r.Data, record.Field{Key: fields[i], Value: fields[i+1]})
	}
	return r
}

func TestWordsAnswerStateForAWord(t *testing.T) {
	ws, errs := Of(project("MUS-P-0001", "MUS",
		"unreviewed = open :: not triaged",
		"fixed = done :: repaired",
		"fixed = dropped :: declared twice",
		"broken"))
	if len(errs) != 2 {
		t.Fatalf("want the duplicate and the unparseable one reported, got %v", errs)
	}
	if s, ok := ws.State("fixed"); !ok || s != Done {
		t.Errorf("fixed = %q, %v; the first declaration stands", s, ok)
	}
	if _, ok := ws.State("Fixed"); ok {
		t.Error("words are matched exactly")
	}
}

func TestCheckReportsEveryWayAFindingCanBeWrong(t *testing.T) {
	rs := []record.Record{
		project("MUS-P-0001", "MUS", "open = open :: work remains", "fixed = done :: repaired"),
		project("MUS-P-0002", "IDW", "unreviewed = open :: a jot", "superseded = dropped :: rerouted"),
		finding("MUS-F-0001", StateField, Open, StatusField, "open"),
		finding("MUS-F-0002", StatusField, "open"),
		finding("MUS-F-0003", StateField, "closed", StatusField, "fixed"),
		finding("MUS-F-0004", StateField, Done, StatusField, "resolved"),
		finding("MUS-F-0005", StateField, Open, StatusField, "fixed"),
		finding("MUS-F-0006", StateField, Open),
		finding("IDW-F-0001", StateField, Dropped, StatusField, "superseded"),
		finding("HRD-F-0001", StateField, Open, StatusField, "open"),
		// Not a finding: nothing asked of it.
		{ID: "MUS-D-0001", Kind: "decision", Title: "d", At: "2026-09-18"},
	}
	got := strings.Join(Check(rs), "\n")
	for _, want := range []string{
		"MUS-F-0002 has no State",
		`MUS-F-0003 has State "closed"`,
		`MUS-F-0004 has Status "resolved", which its project does not declare`,
		`MUS-F-0005 has Status "fixed", which means done, and State open`,
		"MUS-F-0006 has no Status word",
		"HRD-F-0001: no project record declares its prefix",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, clean := range []string{"MUS-F-0001", "IDW-F-0001", "MUS-D-0001"} {
		if strings.Contains(got, clean) {
			t.Errorf("%s is right and was reported:\n%s", clean, got)
		}
	}
	if n := len(Check(rs)); n != 6 {
		t.Errorf("%d problems, want 6:\n%s", n, got)
	}
}

func TestDeclaredIsWhetherAnyProjectHasAList(t *testing.T) {
	bare := project("MUS-P-0001", "MUS")
	if Declared([]record.Record{bare, finding("MUS-F-0001")}) {
		t.Error("a project with no Status word declared one")
	}
	if !Declared([]record.Record{bare, project("MUS-P-0002", "IDW", "noted = done :: x")}) {
		t.Error("a project's list was not seen")
	}
}

func TestSetReplacesInPlaceOrAppends(t *testing.T) {
	r := finding("MUS-F-0001", "Evidence", "", StatusField, "unreviewed")
	Set(&r, Dropped, Superseded)
	if WordOf(r) != Superseded || StateOf(r) != Dropped {
		t.Errorf("%+v", r.Data)
	}
	if r.Data[1].Key != StatusField || len(r.Data) != 3 {
		t.Errorf("Status should stay where it was and State be appended: %+v", r.Data)
	}
}
