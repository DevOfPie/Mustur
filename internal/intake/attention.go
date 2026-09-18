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
// True while it names a destination it was kept from, has not been moved
// (a moved record is superseded), and nobody has chosen to keep it where it is.
func NeedsAttention(r record.Record) bool {
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
	n := 0
	for _, r := range records {
		if NeedsAttention(r) {
			n++
		}
	}
	return n
}

// Keep declines the move a record proposes, leaving it where it is. The record
// says who kept it and when, and stops needing attention.
func Keep(ctx context.Context, s *store.Store, id, actor string, now time.Time) (record.Record, error) {
	r, err := s.Get(ctx, id)
	if err != nil {
		return record.Record{}, err
	}
	if !NeedsAttention(r) {
		return record.Record{}, fmt.Errorf("%s proposes no move, so there is nothing to keep it from", r.ID)
	}
	r.Data = append(r.Data, record.Field{Key: KeptField, Value: actor + " " + now.Format("2006-01-02 15:04 MST")})
	if err := s.Append(ctx, r, "amend", actor); err != nil {
		return record.Record{}, err
	}
	return r, nil
}
