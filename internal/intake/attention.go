package intake

// What needs attention (MUS-D-0193).
//
// A jot that names a destination taking jots only on a confirmed move waits in
// the intake box with the move proposed and not taken. The owner chose a Move
// button on the record over a question per jot, and asked for the Records tab to
// carry a count of what is waiting — so something has to say what is waiting,
// and it is this function and nothing else. The badge, the pinned section on the
// index, the banner on a record and the intake box's line all ask it.
//
// A future cause of attention is added here, or it is a second definition that
// the badge and the list will disagree about.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

// KeptField is the field a jot carries once somebody declined the move its
// Names field proposes. Who and when, so the record says it was a decision.
const KeptField = "Kept"

// NeedsAttention reports whether a record is waiting on somebody.
//
// True while it sits in the intake box — box, the routing record DefaultIn
// finds — names a destination it was kept from, has not been moved (a moved
// record is superseded), and nobody has chosen to keep it where it is.
//
// Where it sits is part of the definition because MUS-D-0193 is about "a jot
// in _IB". A jot naming Archive that was rerouted to Mustur instead is not
// waiting on a move to Archive, and counting it offered "Keep in intake box" on
// a record that was not in it (review of PR 108).
func NeedsAttention(r record.Record, box string) bool {
	if box == "" || filedTo(r) != box {
		return false
	}
	if len(Named(r)) == 0 {
		return false
	}
	if v, ok := r.Get("Status"); ok && strings.EqualFold(strings.TrimSpace(v), "superseded") {
		return false
	}
	if _, kept := r.Get(KeptField); kept {
		return false
	}
	return true
}

// filedTo is the routing record a jot was filed to, from its citation.
func filedTo(r record.Record) string {
	for _, ref := range r.Refs {
		if ref.Key == "Routed to" {
			return strings.TrimSpace(ref.Value)
		}
	}
	return ""
}

// Named returns the destinations a record's Names field proposes, in order.
func Named(r record.Record) []string {
	v, ok := r.Get(NamesField)
	if !ok {
		return nil
	}
	return splitNames(v)
}

func splitNames(v string) []string {
	var out []string
	for _, id := range strings.Split(v, ",") {
		if id = strings.TrimSpace(id); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// AttentionCount is how many records need attention, for the badge.
func AttentionCount(ctx context.Context, s *store.Store) int {
	records, err := s.List(ctx, "")
	if err != nil {
		return 0
	}
	box := DefaultIn(records)
	n := 0
	for _, r := range records {
		if NeedsAttention(r, box) {
			n++
		}
	}
	return n
}

// ErrAlreadyKept is Keep on a record somebody already kept. Nothing is
// written; a second press has nothing left to do.
var ErrAlreadyKept = errors.New("already kept")

// Keep declines the move a record proposes, leaving it where it is. The record
// says who kept it and when, and stops needing attention.
//
// Written only if the record is still the version read, for the reason Reroute
// is: two presses at once would otherwise both append a Kept.
func Keep(ctx context.Context, s *store.Store, id, actor string, now time.Time) (record.Record, error) {
	var err error
	for range 5 {
		var r record.Record
		r, err = keep(ctx, s, id, actor, now)
		if !errors.Is(err, store.ErrChanged) {
			return r, err
		}
	}
	return record.Record{}, err
}

func keep(ctx context.Context, s *store.Store, id, actor string, now time.Time) (record.Record, error) {
	r, version, err := s.GetVersioned(ctx, id)
	if err != nil {
		return record.Record{}, err
	}
	if _, kept := r.Get(KeptField); kept {
		return r, fmt.Errorf("%s: %w", r.ID, ErrAlreadyKept)
	}
	routing, err := routingRecords(ctx, s)
	if err != nil {
		return record.Record{}, err
	}
	if !NeedsAttention(r, DefaultIn(routing)) {
		return record.Record{}, fmt.Errorf("%s proposes no move, so there is nothing to keep it from", r.ID)
	}
	r.Data = append(r.Data, record.Field{Key: KeptField, Value: actor + " " + now.Format("2006-01-02 15:04 MST")})
	if err := s.AmendIf(ctx, r, version, actor); err != nil {
		return record.Record{}, err
	}
	return r, nil
}
