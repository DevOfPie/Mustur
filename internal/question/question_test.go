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
