package intake

import (
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/status"
)

// Each cause and each way out, on one record shape.
func TestNeedsAttention(t *testing.T) {
	jot := func(fields ...record.Field) record.Record {
		return record.Record{ID: "IDW-F-0001", Kind: "finding", Title: "t", At: "2026-09-18",
			Data: append([]record.Field{{Key: "Status", Value: "unreviewed"}}, fields...),
			Refs: []record.Field{{Key: "Routed to", Value: "MUS-P-0002"}}}
	}
	elsewhere := jot(record.Field{Key: NamesField, Value: "MUS-P-0003"})
	elsewhere.Refs = []record.Field{{Key: "Routed to", Value: "MUS-P-0001"}}
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
		{"routed somewhere other than the intake box", elsewhere, false},
	}
	for _, c := range cases {
		if got := NeedsAttention(c.r, "MUS-P-0002"); got != c.want {
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
	// 10:00 UTC in September is 03:00 in Los Angeles, on daylight time.
	if v, _ := kept.Get(KeptField); v != "owner@example.com 2026-09-18 03:00 PDT" {
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
	if NeedsAttention(done.Fresh, "MUS-P-0002") || NeedsAttention(done.Old, "MUS-P-0002") {
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

// The reviewer's scenario on PR 108: a jot naming Archive is rerouted to
// Mustur rather than moved to Archive. The Mustur record must not carry the
// proposal, offer Move to Archive and Keep in intake box, or count on the
// badge.
func TestRerouteElsewhereDropsTheProposal(t *testing.T) {
	s, ctx := openWith(t, withOptOut())
	r, _, err := File(ctx, s, Request{Project: "MUS", Text: "archive the old intake notes", Actor: "pie", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Keep(ctx, s, r.ID, "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	done, err := Reroute(ctx, s, RerouteRequest{Project: "MUS", ID: r.ID, To: "MUS-P-0001", Actor: "owner", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{NamesField, KeptField} {
		if v, ok := done.Fresh.Get(k); ok {
			t.Errorf("the record rerouted to Mustur carries %s = %q", k, v)
		}
	}

	// And a record outside the box that carries Names anyway — written by
	// hand, or by an older build — is not counted.
	stray := done.Fresh
	stray.Data = append(stray.Data, record.Field{Key: NamesField, Value: "MUS-P-0003"})
	if err := s.Append(ctx, stray, "amend", "test"); err != nil {
		t.Fatal(err)
	}
	if n := AttentionCount(ctx, s); n != 0 {
		t.Errorf("AttentionCount = %d with the only Names outside the intake box", n)
	}
}

// Winter is PST, not a fixed offset written as PDT.
func TestKeptIsPacificInWinterToo(t *testing.T) {
	s, ctx := openWith(t, withOptOut())
	r, _, err := File(ctx, s, Request{Project: "MUS", Text: "archive the winter notes", Actor: "pie", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	kept, err := Keep(ctx, s, r.ID, "owner", time.Date(2026, 12, 1, 20, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := kept.Get(KeptField); v != "owner 2026-12-01 12:30 PST" {
		t.Errorf("Kept = %q", v)
	}
}

// Keeping a jot declines a move and triages nothing, so the jot stays open and
// unreviewed (MUS-D-0196). Keep appends who kept it and touches no other field.
func TestKeepingAJotLeavesItsStateAlone(t *testing.T) {
	s, ctx := openWith(t, withOptOut())
	r, _, err := File(ctx, s, Request{Project: "MUS", Text: "archive the old intake notes", Actor: "pie", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	kept, err := Keep(ctx, s, r.ID, "owner@example.com", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if w, st := status.WordOf(kept), status.StateOf(kept); w != status.Unreviewed || st != status.Open {
		t.Errorf("kept: Status %q, State %q; want unreviewed and open", w, st)
	}
}
