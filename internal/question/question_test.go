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
