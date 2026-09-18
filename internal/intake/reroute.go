package intake

// Correcting a jot that "Route it for me" put in the wrong place.
//
// The owner asked for this on MUS-F-0044, having watched a request about
// Mustur's own session view land in the idea inbox. The obvious shape — a --to
// flag on amend — is not available, and the reason is worth stating because it
// is the whole design of this function.
//
// **The identifier is the routing.** The jot MUS-F-0044 is about was filed as
// IDW-F-0004, called IDW because the idea inbox's prefix was IDW when it went
// there; the prefix is derived from the destination at the moment it is filed.
// (MUS-D-0192 renames it _IB-F-0004 in place, with the intake box's other five
// IDW jots, so that IDW is Idea Warehouse's alone: the one time a prefix
// changes after filing.) So moving a record and renaming it are the same act,
// and identifiers are permanent. On MUS-Q-0058 the owner chose which of those two
// gives way: neither. A correction files a *new* record at the right
// destination and retires the old one in place, still resolving, pointing at
// its replacement.
//
// The cost is a stub left in the wrong project's list, and a counter that goes
// up rather than down. That was chosen with its eyes open: every alternative
// either breaks a citation that already exists somewhere unreachable, or leaves
// a prefix that lies about where its record lives.
//
// It lives here rather than in the command because it has two callers: `mustur
// reroute`, and the Move button on a record that names a destination taking
// jots only on a confirmed move (MUS-D-0193). Two copies of this would drift,
// and the drift would be in exactly the carrying-across below that took two
// findings to get right.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

// SupersededBy is the field a retired record carries. Its presence is what
// makes a record a stub rather than a claim.
const SupersededBy = "Superseded by"

// RerouteRequest is one correction.
type RerouteRequest struct {
	// Project is the store's own identifier prefix, for a destination that
	// names none.
	Project string
	ID      string
	// To is the routing record the jot belongs at.
	To    string
	Actor string
	// Why is one line on why the first routing was wrong. Empty writes one.
	Why string
	Now time.Time
}

// Rerouted is what a correction did.
type Rerouted struct {
	Fresh record.Record
	Old   record.Record
	Dest  Destination
	// Moved is how many pictures went across with it.
	Moved int
}

// AlreadyCorrected is the refusal for a jot that has been rerouted already. By
// is the record that carries it now, so a caller that pressed twice can be
// sent where the first press went.
type AlreadyCorrected struct {
	ID string
	By string
}

func (e *AlreadyCorrected) Error() string {
	return fmt.Sprintf("%s was already corrected, by %s. Reroute that one instead", e.ID, e.By)
}

// CorrectedBy is the record a superseded jot points at, or "".
func CorrectedBy(r record.Record) string {
	for _, ref := range r.Refs {
		if ref.Key == SupersededBy && strings.TrimSpace(ref.Value) != "" {
			return strings.TrimSpace(ref.Value)
		}
	}
	if v, ok := r.Get(SupersededBy); ok && strings.TrimSpace(v) != "" {
		id, _, _ := strings.Cut(strings.TrimSpace(v), " ")
		return id
	}
	return ""
}

// Reroute files a jot afresh at req.To and retires the original in place,
// superseded and still resolving.
//
// The new record, the retirement and the pictures are written in one
// transaction that first checks the jot is still the version this read. Two
// corrections of one jot at once — a double press — file one record between
// them: the loser finds the jot changed, reads it again, and is told it was
// already corrected and by what.
func Reroute(ctx context.Context, s *store.Store, req RerouteRequest) (Rerouted, error) {
	if strings.TrimSpace(req.To) == "" {
		return Rerouted{}, fmt.Errorf("reroute needs --to: a correction that does not say where is not a correction")
	}
	// Somebody writing the jot between the read and the write means go round:
	// the read says what they did, which is usually that they already
	// corrected it. Bounded, so a record under constant rewriting is an error
	// rather than a spin.
	var err error
	for range 5 {
		var done Rerouted
		done, err = reroute(ctx, s, req)
		if !errors.Is(err, store.ErrChanged) {
			return done, err
		}
	}
	return Rerouted{}, err
}

func reroute(ctx context.Context, s *store.Store, req RerouteRequest) (Rerouted, error) {
	old, version, err := s.GetVersioned(ctx, req.ID)
	if err != nil {
		return Rerouted{}, err
	}
	// Correcting a correction would leave two stubs pointing at each other and
	// no way to tell which one anybody meant.
	if by := CorrectedBy(old); by != "" {
		return Rerouted{}, &AlreadyCorrected{ID: old.ID, By: by}
	}
	if strings.TrimSpace(old.Body) == "" {
		return Rerouted{}, fmt.Errorf("%s has no body to re-file; amend it rather than rerouting it", old.ID)
	}
	// This corrects a jot, and a jot is a record intake filed: "Routed to" is
	// written by File and by nothing else, so its presence is exactly the
	// question being asked.
	//
	// Without it, reroute took anything with a body. A project record went
	// through: its description was filed as a fresh finding in the idea inbox
	// and the record defining the store's own prefix was marked superseded by
	// it. Nothing in the command's purpose covered that, and nothing in the
	// command stopped it (MUS-F-0058).
	if _, routed := old.Get("Routed to"); !routed {
		return Rerouted{}, fmt.Errorf("%s was not filed through the intake box, so it has no routing to correct. "+
			"reroute is for a jot that \"Route it for me\" put in the wrong place", old.ID)
	}

	// Drafted by the code that files everything else, so the destination is
	// resolved, the prefix is chosen and the fields are shaped exactly as a
	// filing would shape them. A correction that took its own path would
	// drift from the thing it is correcting. Drafted rather than filed: the
	// filing happens below, in the transaction that retires the original.
	fresh, dest, under, err := draft(ctx, s, Request{
		Project: req.Project,
		Text:    old.Body,
		Actor:   req.Actor,
		Now:     req.Now,
		To:      req.To,
	}, strings.TrimSpace(old.Body))
	if err != nil {
		return Rerouted{}, err
	}

	note := strings.TrimSpace(req.Why)
	if note == "" {
		note = "routed to " + fieldOr(old, "Routed to", "nowhere") + " when it belonged to " + dest.Name
	}

	// A correction is about where the record lives, not what it says. So the
	// draft keeps only the two things it alone can decide — the destination
	// and the prefix in the identifier — and everything the record actually
	// claimed is carried across unchanged.
	//
	// Without this the new record's title was re-derived from the body, so a
	// jot that had since been given a proper title got an automatic one back,
	// and a finding already marked fixed came out unreviewed. Rerouting would
	// have quietly undone every amendment made since it was filed.
	fresh.Title, fresh.Body, fresh.At = old.Title, old.Body, old.At
	routingField := func(k string) bool {
		return strings.EqualFold(k, "Routed to") || strings.EqualFold(k, "Routing")
	}
	kept := fresh.Data[:0:0]
	for _, f := range fresh.Data {
		if routingField(f.Key) {
			kept = append(kept, f)
		}
	}
	for _, f := range old.Data {
		if !routingField(f.Key) {
			kept = append(kept, f)
		}
	}
	// Arriving where it named is the move that field was proposing, so the
	// proposal does not travel with it: a record carried into the place it
	// named and still saying it names it would ask for the same move again.
	fresh.Data = confirmed(kept, dest.ID)
	for _, f := range old.Refs {
		if !routingField(f.Key) {
			fresh.Refs = append(fresh.Refs, f)
		}
	}
	fresh.Data = append(fresh.Data, record.Field{Key: "Corrects", Value: old.ID + " — " + note})
	fresh.Refs = append(fresh.Refs, record.Field{Key: "Corrects", Value: old.ID})

	// The old one stays, still resolving, and stops making a claim. The
	// pictures go with the record, not with the stub: a jot filed from a phone
	// carries its evidence in the attachment, and leaving it behind means the
	// record anybody reads has none.
	retire := func(freshID string) record.Record {
		stub := old
		stub.Data = append([]record.Field(nil), old.Data...)
		stub.Refs = append([]record.Field(nil), old.Refs...)
		setStatus(&stub, "superseded")
		stub.Data = append(stub.Data, record.Field{Key: SupersededBy, Value: freshID + " — " + note})
		stub.Refs = append(stub.Refs, record.Field{Key: SupersededBy, Value: freshID})
		return stub
	}
	filed, moved, err := s.Supersede(ctx, fresh, under, ident.Finding, old.ID, version, retire, req.Actor)
	if err != nil {
		return Rerouted{}, err
	}
	return Rerouted{Fresh: filed, Old: retire(filed.ID), Dest: dest, Moved: moved}, nil
}

// confirmed takes a destination out of a record's Names, because the record
// has just been moved there. Kept goes with the last of them: it declined a
// move nobody is proposing any more.
func confirmed(data []record.Field, dest string) []record.Field {
	var names []string
	found := false
	for _, f := range data {
		if strings.EqualFold(f.Key, NamesField) {
			found = true
			for _, id := range splitNames(f.Value) {
				if id != dest {
					names = append(names, id)
				}
			}
		}
	}
	if !found {
		return data
	}
	out := data[:0:0]
	for _, f := range data {
		switch {
		case strings.EqualFold(f.Key, NamesField):
			if len(names) > 0 {
				out = append(out, record.Field{Key: NamesField, Value: strings.Join(names, ", ")})
			}
		case strings.EqualFold(f.Key, KeptField) && len(names) == 0:
		default:
			out = append(out, f)
		}
	}
	return out
}

// setStatus replaces the Status field in place, or appends it.
func setStatus(r *record.Record, status string) {
	for i := range r.Data {
		if strings.EqualFold(r.Data[i].Key, "Status") {
			r.Data[i].Value = status
			return
		}
	}
	r.Data = append(r.Data, record.Field{Key: "Status", Value: status})
}
