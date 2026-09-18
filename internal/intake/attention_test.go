package intake

import (
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/record"
)

// Each cause and each way out, on one record shape.
func TestNeedsAttention(t *testing.T) {
	jot := func(fields ...record.Field) record.Record {
		return record.Record{ID: "IDW-F-0001", Kind: "finding", Title: "t", At: "2026-09-18",
			Data: append([]record.Field{{Key: "Status", Value: "unreviewed"}}, fields...)}
	}
	cases := []struct {
		name string
		r    record.Record
		want bool
	}{
		{"names nothing", jot(), false},
		{"names a destination", jot(record.Field{Key: NamesField, Value: "MUS-P-0003"}), true},
		{"names two", jot(record.Field{Key: NamesField, Value: "MUS-P-0003, MUS-P-0004"}), true},
		{"an empty Names", jot(record.Field{Key: NamesField, Value: " , "}), false},
		{"kept", jot(record.Field{Key: NamesField, Value: "MUS-P-0003"}, record.Field{Key: KeptField, Value: "pie 2026-09-18 10:00 PDT"}), false},
		{"moved", func() record.Record {
			r := jot(record.Field{Key: NamesField, Value: "MUS-P-0003"})
			r.Data[0].Value = "Superseded"
			return r
		}(), false},
	}
	for _, c := range cases {
		if got := NeedsAttention(c.r); got != c.want {
			t.Errorf("%s: NeedsAttention = %v, want %v", c.name, got, c.want)
		}
	}
}

// Keep says who and when, stops the record needing attention, and refuses a
// record that proposes nothing.
func TestKeepingAJotEndsItsAttention(t *testing.T) {
	s, ctx := openWith(t, withOptOut())
	r, _, err := File(ctx, s, Request{Project: "MUS", Text: "archive the old intake notes", Actor: "pie", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if n := AttentionCount(ctx, s); n != 1 {
		t.Fatalf("AttentionCount = %d before keeping, want 1", n)
	}
	kept, err := Keep(ctx, s, r.ID, "owner@example.com", time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := kept.Get(KeptField); v != "owner@example.com 2026-09-18 10:00 UTC" {
		t.Errorf("Kept = %q", v)
	}
	if n := AttentionCount(ctx, s); n != 0 {
		t.Errorf("AttentionCount = %d after keeping, want 0", n)
	}
	if _, err := Keep(ctx, s, r.ID, "owner@example.com", time.Now()); err == nil {
		t.Error("a jot already kept was kept again")
	}

	plain, _, err := File(ctx, s, Request{Project: "MUS", Text: "whippy-vm needs more disk", Actor: "pie", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Keep(ctx, s, plain.ID, "owner@example.com", time.Now()); err == nil {
		t.Error("a jot proposing no move was kept")
	}
}

// Moving a jot where it named is the move being confirmed: the new record
// does not propose it again, and the stub is superseded, so neither needs
// attention.
func TestMovingAJotWhereItNamedConfirmsIt(t *testing.T) {
	s, ctx := openWith(t, withOptOut())
	r, _, err := File(ctx, s, Request{Project: "MUS", Text: "archive the old intake notes", Actor: "pie", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	done, err := Reroute(ctx, s, RerouteRequest{Project: "MUS", ID: r.ID, To: "MUS-P-0003", Actor: "owner", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(done.Fresh.ID, "ARC-F-") {
		t.Errorf("moved to %s, want an ARC identifier", done.Fresh.ID)
	}
	if v, ok := done.Fresh.Get(NamesField); ok {
		t.Errorf("the moved record still proposes the move: %s = %q", NamesField, v)
	}
	if NeedsAttention(done.Fresh) || NeedsAttention(done.Old) {
		t.Error("a moved jot still needs attention")
	}
	if n := AttentionCount(ctx, s); n != 0 {
		t.Errorf("AttentionCount = %d after the move, want 0", n)
	}
}

// Of two names, moving to one leaves the other proposed.
func TestConfirmingOneNameLeavesTheOther(t *testing.T) {
	data := []record.Field{
		{Key: "Status", Value: "unreviewed"},
		{Key: NamesField, Value: "MUS-P-0003, MUS-P-0004"},
	}
	got := confirmed(data, "MUS-P-0003")
	if len(got) != 2 || got[1].Value != "MUS-P-0004" {
		t.Errorf("confirmed = %v", got)
	}
	got = confirmed([]record.Field{{Key: NamesField, Value: "MUS-P-0003"}, {Key: KeptField, Value: "x"}}, "MUS-P-0003")
	if len(got) != 0 {
		t.Errorf("confirming the last name left %v", got)
	}
}
