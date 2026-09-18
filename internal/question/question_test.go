package question

import (
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

func q(id string, data ...string) record.Record {
	r := record.Record{ID: id, Kind: Kind, Title: "a question", At: "2026-08-21"}
	for i := 0; i+1 < len(data); i += 2 {
		r.Data = append(r.Data, record.Field{Key: data[i], Value: data[i+1]})
	}
	return r
}

// A question with no status is open. Treating a malformed record as closed
// would be exactly the failure this package prevents, wearing a disguise.
func TestMissingStatusIsOpen(t *testing.T) {
	if got := Status(q("MUS-Q-0001")); got != StatusOpen {
		t.Errorf("status with no field = %q, want %q", got, StatusOpen)
	}
	if got := Status(q("MUS-Q-0002", FieldStatus, "nonsense")); got != StatusOpen {
		t.Errorf("unrecognised status = %q, want %q", got, StatusOpen)
	}
	if got := Status(q("MUS-Q-0003", FieldStatus, "  ANSWERED ")); got != StatusAnswered {
		t.Errorf("status = %q, want %q", got, StatusAnswered)
	}
}

func TestSurfacedIsAboutTheFieldNotTheStatus(t *testing.T) {
	if Surfaced(q("MUS-Q-0001")) {
		t.Error("a question with no Surfaced field counts as surfaced")
	}
	if Surfaced(q("MUS-Q-0002", FieldSurfaced, "   ")) {
		t.Error("whitespace counts as surfaced")
	}
	if !Surfaced(q("MUS-Q-0003", FieldSurfaced, "2026-08-21")) {
		t.Error("a dated Surfaced field does not count as surfaced")
	}
}

// The gate blocks on open-and-never-surfaced, and on nothing else. An
// unanswered question does not block: the owner may be away, and work that
// stops for an absent owner is the cost this whole design refuses to pay.
func TestGateBlocksOnlyBuriedQuestions(t *testing.T) {
	cases := []struct {
		name    string
		rec     record.Record
		blocked bool
	}{
		{"never surfaced", q("MUS-Q-0001"), true},
		{"surfaced, still open", q("MUS-Q-0002", FieldSurfaced, "2026-08-21"), false},
		{"answered without ever being surfaced", q("MUS-Q-0003", FieldStatus, StatusAnswered), false},
		{"withdrawn", q("MUS-Q-0004", FieldStatus, StatusWithdrawn), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Gate([]record.Record{c.rec})
			if c.blocked && err == nil {
				t.Fatal("gate passed a buried question")
			}
			if !c.blocked && err != nil {
				t.Fatalf("gate blocked on %v", err)
			}
		})
	}
}

// The owner's qualification when they ratified the rule: surfacing is enough
// "as long as the work it is doing doesn't depend on the question's answer".
// So a question marked as needed is not waited out by being asked politely.
func TestNeededQuestionsBlockEvenAfterSurfacing(t *testing.T) {
	cases := []struct {
		name    string
		rec     record.Record
		blocked bool
	}{
		{"needed, surfaced, unanswered", q("MUS-Q-0001", FieldSurfaced, "2026-08-21", FieldNeeded, Yes), true},
		{"not needed, surfaced, unanswered", q("MUS-Q-0002", FieldSurfaced, "2026-08-21"), false},
		{"needed and answered", q("MUS-Q-0003", FieldSurfaced, "2026-08-21", FieldNeeded, Yes, FieldStatus, StatusAnswered), false},
		{"needed and withdrawn", q("MUS-Q-0004", FieldSurfaced, "2026-08-21", FieldNeeded, Yes, FieldStatus, StatusWithdrawn), false},
		{"needed spelled no", q("MUS-Q-0005", FieldSurfaced, "2026-08-21", FieldNeeded, "no"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Gate([]record.Record{c.rec})
			if c.blocked && err == nil {
				t.Fatal("gate passed a question the work depends on")
			}
			if !c.blocked && err != nil {
				t.Fatalf("gate blocked on %v", err)
			}
		})
	}
}

// The two reasons a question blocks are different, and the message says which,
// because the remedy is different too.
func TestGateDistinguishesUnsurfacedFromUnanswered(t *testing.T) {
	err := Gate([]record.Record{
		q("MUS-Q-0001"),
		q("MUS-Q-0002", FieldSurfaced, "2026-08-21", FieldNeeded, Yes),
	})
	if err == nil {
		t.Fatal("no error")
	}
	msg := err.Error()
	for _, want := range []string{"never surfaced as a prompt", "the work depends on the answer", "mustur surfaced"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message does not carry %q:\n%s", want, msg)
		}
	}
}

func TestAskedByIsReadBack(t *testing.T) {
	if got := AskedBy(q("MUS-Q-0001", FieldAskedBy, " whippy ")); got != "whippy" {
		t.Errorf("AskedBy = %q", got)
	}
	if got := AskedBy(q("MUS-Q-0002")); got != "" {
		t.Errorf("AskedBy on a record without the field = %q", got)
	}
}

func TestOptionsKeepTheirOrderAndParts(t *testing.T) {
	r := q("MUS-Q-0001",
		FieldOption, "Check StrucGu out in CI"+OptionSep+"Recommended · runs on every push"+OptionSep+"The catalog is fetched per run.",
		FieldOption, "Vendor a pinned copy"+OptionSep+"Runs offline · a stale copy proves nothing",
		FieldOption, "Leave it out",
	)
	got := Options(r)
	if len(got) != 3 {
		t.Fatalf("parsed %d options, want 3", len(got))
	}
	if got[0].Label != "Check StrucGu out in CI" || got[0].Detail != "The catalog is fetched per run." {
		t.Errorf("first option = %+v", got[0])
	}
	if !got[0].IsRecommended() {
		t.Error("the recommended option does not report itself as one")
	}
	if got[1].Detail != "" {
		t.Errorf("an option with two parts gained a detail: %q", got[1].Detail)
	}
	if got[2].Label != "Leave it out" || got[2].Line != "" {
		t.Errorf("a bare label did not survive: %+v", got[2])
	}
	if got[1].IsRecommended() {
		t.Error("a non-recommended option reports itself as recommended")
	}
}

func TestAQuestionWithoutOptionsHasNone(t *testing.T) {
	if got := Options(q("MUS-Q-0001", FieldBlocks, "milestone 3")); len(got) != 0 {
		t.Errorf("parsed %d options from a question with none", len(got))
	}
}

func TestGateIgnoresOtherKinds(t *testing.T) {
	f := record.Record{ID: "MUS-F-0001", Kind: "finding", Title: "a finding", At: "2026-08-21"}
	if err := Gate([]record.Record{f}); err != nil {
		t.Fatalf("gate blocked on a finding: %v", err)
	}
}

// The message has to say what to do about it. A gate that blocks without
// naming the remedy teaches the reader to route around it.
func TestGateNamesEveryBuriedQuestionAndTheRemedy(t *testing.T) {
	err := Gate([]record.Record{
		q("MUS-Q-0001", FieldBlocks, "milestone 3"),
		q("MUS-Q-0002", FieldSurfaced, "2026-08-21"),
		q("MUS-Q-0003"),
	})
	if err == nil {
		t.Fatal("no error")
	}
	msg := err.Error()
	for _, want := range []string{"MUS-Q-0001", "MUS-Q-0003", "milestone 3", "mustur surfaced"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message does not mention %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "MUS-Q-0002") {
		t.Errorf("message names a surfaced question:\n%s", msg)
	}
}

// The marker stays in the record and comes off the surface.
//
// MUS-F-0072: the owner asked for the recommendation to be a mark on the
// option's row rather than text in its description, and it was both — the
// prefix is what sets the flag, and the line was rendered with the word still
// on the front of it.
func TestSaysTakesTheMarkerOffAndKeepsTheSentence(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"Recommended. A media query decides it", "A media query decides it"},
		{"Recommended: one screen, not nine", "one screen, not nine"},
		{"Recommended — the essays stay", "the essays stay"},
		// The separator the existing questions in this store are written with.
		{"Recommended · conformance runs on every push", "conformance runs on every push"},
		{"Recommended the essays stay", "the essays stay"},
		// Not recommended: untouched, including a line that merely mentions it.
		{"one rule, no detection", "one rule, no detection"},
		{"cheaper than the Recommended one", "cheaper than the Recommended one"},
		// The whole line is the marker: there is nothing else to say, and an
		// empty description would lose it silently.
		{"Recommended", "Recommended"},
	} {
		o := Option{Label: "x", Line: c.line}
		if got := o.Says(); got != c.want {
			t.Errorf("Says(%q) = %q, want %q", c.line, got, c.want)
		}
	}
	// The flag still comes from the raw line, so the record is unchanged.
	if !(Option{Line: "Recommended. x"}).IsRecommended() {
		t.Error("stripping broke the flag it is derived from")
	}
}

// The recommendation has one place, and putting it elsewhere is refused.
//
// MUS-F-0095: thirteen questions in a row were raised with the marker at the
// front of the paragraph rather than the line. IsRecommended never saw it, no
// star ever drew, and the owner answered every one of them without being told
// which was recommended — the asker had been saying so in prose instead, which
// is why nothing surfaced it.
func TestCheckOptionRefusesAMisplacedRecommendation(t *testing.T) {
	err := CheckOption("A :: one line :: Recommended. the paragraph")
	if err == nil {
		t.Fatal("an option with the marker in its paragraph was accepted")
	}
	// The message has to carry the corrected option, or the next person writes
	// it wrong again while reading about writing it wrong.
	if !strings.Contains(err.Error(), `A :: Recommended. one line :: the paragraph`) {
		t.Errorf("the refusal does not show the fix: %v", err)
	}

	for _, ok := range []string{
		"A :: Recommended. one line :: the paragraph", // where it belongs
		"A :: one line :: the paragraph",              // no marker at all
		"A :: one line",                               // no paragraph to misplace it in
		"A",                                           // a bare label
		// The word inside the paragraph rather than at the front of it: prose
		// about a recommendation is not a misplaced marker.
		"A :: one line :: cheaper than the Recommended one",
	} {
		if err := CheckOption(ok); err != nil {
			t.Errorf("CheckOption(%q) refused a well-formed option: %v", ok, err)
		}
	}
}

// A marker at the head of the label moves to the line, where the star reads it.
//
// MUS-Q-0173 was raised with "Recommended Refuse a foreign Origin" as its
// label: the word showed in bold inside the answer's name and no star drew,
// because IsRecommended reads only the line.
func TestNormaliseOptionMovesAMarkerOffTheLabel(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		// Moved to the head of the line, the paragraph untouched.
		{"Recommended Refuse a foreign Origin :: closes it :: the detail",
			"Refuse a foreign Origin :: Recommended. closes it :: the detail"},
		{"Recommended: A :: one line", "A :: Recommended. one line"},
		{"Recommended · A :: one line :: p", "A :: Recommended. one line :: p"},
		// No line to move it onto: the marker becomes the line.
		{"Recommended. A", "A :: Recommended"},
		// Both carry it: the label's copy goes, the line stays as written.
		{"Recommended A :: Recommended. one line :: p", "A :: Recommended. one line :: p"},
		// A bare-word label would leave the option nameless, so it stays.
		{"Recommended :: one line :: p", "Recommended :: one line :: p"},
		{"Recommended", "Recommended"},
		// Not followed by a separator: not the marker.
		{"Recommendedly cheaper :: one line", "Recommendedly cheaper :: one line"},
		// No marker at all: returned exactly as given.
		{"A :: one line :: p", "A :: one line :: p"},
	} {
		got := NormaliseOption(c.in)
		if got != c.want {
			t.Errorf("NormaliseOption(%q) = %q, want %q", c.in, got, c.want)
		}
		if err := CheckOption(got); err != nil {
			t.Errorf("NormaliseOption(%q) produced an option CheckOption refuses: %v", c.in, err)
		}
	}

	// The point of it: the star draws and the label reads as the answer.
	os := Options(q("MUS-Q-0001", FieldOption,
		NormaliseOption("Recommended Refuse a foreign Origin :: closes it :: the detail")))
	if len(os) != 1 {
		t.Fatalf("Options = %+v, want one", os)
	}
	if o := os[0]; !o.IsRecommended() || o.Label != "Refuse a foreign Origin" || o.Says() != "closes it" {
		t.Errorf("normalised option = %+v, recommended %v, says %q", o, o.IsRecommended(), o.Says())
	}
}

// A commit gate is about the work being committed.
//
// While the store held one project this was the whole store and the question
// never arose. Since a second moved in, one project's unsurfaced question fails
// every other project's gate: a Mustur commit was blocked by two questions
// raised minutes earlier by the session onboarding Hoard, which no Mustur
// session can surface and none may answer (MUS-F-0142).
func TestTheGateCanBeNarrowedToOneProject(t *testing.T) {
	mine := record.Record{ID: "MUS-Q-0001", Kind: Kind, Title: "mine, and surfaced",
		Data: []record.Field{{Key: FieldStatus, Value: StatusOpen}, {Key: FieldSurfaced, Value: "2026-09-13 04:00"}}}
	theirs := record.Record{ID: "HRD-Q-0007", Kind: Kind, Title: "theirs, and never surfaced",
		Data: []record.Field{{Key: FieldStatus, Value: StatusOpen}}}

	if err := Gate(OfProject([]record.Record{mine, theirs}, "MUS")); err != nil {
		t.Errorf("another project's buried question blocked this one's gate: %v", err)
	}
	if err := Gate(OfProject([]record.Record{mine, theirs}, "HRD")); err == nil {
		t.Error("a project's own buried question did not block its gate")
	}
	// No prefix is the whole store, which is what a person running it by hand
	// wants to see.
	if err := Gate(OfProject([]record.Record{mine, theirs}, "")); err == nil {
		t.Error("an empty prefix dropped questions instead of keeping them all")
	}
	// A prefix must not match by accident.
	if got := OfProject([]record.Record{mine, theirs}, "MU"); len(got) != 0 {
		t.Errorf("prefix MU matched %d record(s); it is a project prefix, not a substring", len(got))
	}
}
