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
		// A prefix no list covers: only a State that is not one is reported.
		finding("HRD-F-0001", StateField, Open, StatusField, "open"),
		finding("HRD-F-0002", StateField, "closed"),
		// A second Status, and one spelled in lower case, beside a good pair.
		finding("MUS-F-0007", StateField, Open, StatusField, "open", StatusField, "fixed"),
		finding("MUS-F-0008", StateField, Open, StatusField, "open", "status", "garbage"),
		// Not a finding: nothing asked of it.
		{ID: "MUS-D-0001", Kind: "decision", Title: "d", At: "2026-09-18"},
	}
	got := strings.Join(Check(rs, ""), "\n")
	for _, want := range []string{
		"MUS-F-0002: it has no State; open means open",
		`MUS-F-0003: State "closed" is not open, done or dropped`,
		`MUS-F-0004: Status "resolved" is not a word MUS declares`,
		"MUS-F-0005: Status fixed means done, and State says open",
		"MUS-F-0006: it has no Status word",
		`HRD-F-0002: State "closed" is not open, done or dropped`,
		"MUS-F-0007: Status is given 2 times",
		`MUS-F-0008: a field is named "status", which is spelled Status`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, clean := range []string{"MUS-F-0001", "IDW-F-0001", "HRD-F-0001", "MUS-D-0001"} {
		if strings.Contains(got, clean) {
			t.Errorf("%s is right and was reported:\n%s", clean, got)
		}
	}
	if n := len(Check(rs, "")); n != 8 {
		t.Errorf("%d problems, want 8:\n%s", n, got)
	}
	// Scoped, the other projects' findings are not read.
	if got := strings.Join(Check(rs, "IDW"), "\n"); got != "" {
		t.Errorf("IDW alone: %s", got)
	}
}

// Fill supplies the State a declared word means and nothing else; Trim takes
// the space off both values; Unlisted names the record to give a list to.
func TestFillTrimAndUnlisted(t *testing.T) {
	rs := []record.Record{
		project("MUS-P-0001", "MUS", "fixed = done :: repaired"),
		project("MUS-P-0003", "HRD"),
	}
	p, _ := Index(rs)
	r := finding("", StatusField, "fixed")
	Fill("MUS", &r, p)
	if StateOf(r) != Done {
		t.Errorf("a declared word alone: %v", r.Data)
	}
	r = finding("", StatusField, "fixed", StateField, Open)
	Fill("MUS", &r, p)
	if StateOf(r) != Open {
		t.Error("Fill overwrote a State that was given; the mismatch is Finding's to refuse")
	}
	r = finding("", StatusField, "whatever")
	Fill("MUS", &r, p)
	if StateOf(r) != "" {
		t.Error("Fill guessed a State for an undeclared word")
	}
	r = finding("", StatusField, " fixed ", StateField, " done\n")
	Trim(&r)
	if r.Data[0].Value != "fixed" || r.Data[1].Value != Done {
		t.Errorf("Trim: %q", r.Data)
	}
	if msg := Unlisted("HRD", finding("", StatusField, "open"), rs); !strings.Contains(msg, `add "Status word" fields`) || !strings.Contains(msg, "MUS-P-0003") {
		t.Errorf("Unlisted HRD: %q", msg)
	}
	if msg := Unlisted("LNK", finding("", StatusField, "open"), rs); !strings.Contains(msg, "no project record has the prefix LNK") {
		t.Errorf("Unlisted LNK: %q", msg)
	}
	if msg := Unlisted("MUS", finding("", StatusField, "fixed"), rs); msg != "" {
		t.Errorf("a listed project: %q", msg)
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

func TestFindingRefusesOneFindingTheWayCheckReportsIt(t *testing.T) {
	p, _ := Index([]record.Record{
		project("MUS-P-0001", "MUS", "open = open :: work remains", "fixed = done :: repaired"),
		project("MUS-P-0003", "HRD"),
	})
	long := strings.Repeat("prose ", 20)
	for _, c := range []struct {
		prefix string
		r      record.Record
		want   string
	}{
		{"MUS", finding("MUS-F-0001", StatusField, "open", StateField, Open), ""},
		{"MUS", finding("", StatusField, "fixed", StateField, Done), ""},
		{"MUS", record.Record{Kind: "decision"}, ""},
		{"MUS", finding("", StateField, Open), "a new MUS finding: it has no Status word"},
		{"MUS", finding("MUS-F-0002", StatusField, long, StateField, Open), `Status "` + long[:60] + `…" is not a word MUS declares`},
		{"MUS", finding("MUS-F-0003", StatusField, "fixed"), "it has no State; fixed means done"},
		{"MUS", finding("MUS-F-0004", StatusField, "fixed", StateField, "closed"), `State "closed" is not open, done or dropped`},
		{"MUS", finding("MUS-F-0005", StatusField, "fixed", StateField, Open), "Status fixed means done, and State says open"},
		{"HRD", finding("HRD-F-0001", StatusField, "open", StateField, Open), ""},
		{"HRD", finding("HRD-F-0002", StatusField, "anything"), ""},
		{"HRD", finding("HRD-F-0003", StateField, "Done"), `State "Done" is not open, done or dropped`},
		{"MUS", finding("MUS-F-0009", StatusField, "open", StateField, Open, "STATE", Open), `a field is named "STATE", which is spelled State`},
		{"LNK", finding("LNK-F-0001", StatusField, "open", StateField, Open), ""},
	} {
		no := Finding(c.prefix, c.r, p)
		switch {
		case c.want == "" && no != nil:
			t.Errorf("%s refused: %v", c.r.ID, no)
		case c.want != "" && (no == nil || !strings.Contains(no.Error(), c.want)):
			t.Errorf("%s: got %v, want %q", c.r.ID, no, c.want)
		}
	}
	// No list anywhere: nothing is checked.
	bare, _ := Index([]record.Record{project("MUS-P-0001", "MUS")})
	if no := Finding("MUS", finding("MUS-F-0001", StatusField, long), bare); no != nil {
		t.Errorf("a store with no list refused: %v", no)
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
