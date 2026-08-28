package main

// Correcting a jot that "Route it for me" put in the wrong place.
//
// The owner asked for this on MUS-F-0044, having watched a request about
// Mustur's own session view land in the idea inbox. The obvious shape — a --to
// flag on amend — is not available, and the reason is worth stating because it
// is the whole design of this command.
//
// **The identifier is the routing.** IDW-F-0004 is called IDW because it went
// to the idea inbox; the prefix is derived from the destination at the moment
// it is filed. So moving a record and renaming it are the same act, and
// identifiers are permanent. On MUS-Q-0058 the owner chose which of those two
// gives way: neither. A correction files a *new* record at the right
// destination and retires the old one in place, still resolving, pointing at
// its replacement.
//
// The cost is a stub left in the wrong project's list, and a counter that goes
// up rather than down. That was chosen with its eyes open: every alternative
// either breaks a citation that already exists somewhere unreachable, or leaves
// a prefix that lies about where its record lives.

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/intake"
	"github.com/DevOfPie/Mustur/internal/record"
)

// supersededBy is the field a retired record carries. Its presence is what
// makes a record a stub rather than a claim.
const supersededBy = "Superseded by"

func cmdReroute(args []string) error {
	fs := flag.NewFlagSet("reroute", flag.ContinueOnError)
	db := dbFlag(fs)
	project := fs.String("project", "MUS", "identifier prefix for a store holding more than one project")
	actor := fs.String("actor", defaultActor(), "who is making the correction")
	to := fs.String("to", "", "the destination it should have gone to")
	why := fs.String("why", "", "one line on why the first routing was wrong")
	id, err := parseWithPositional(fs, args, "reroute needs one identifier")
	if err != nil {
		return err
	}
	if strings.TrimSpace(*to) == "" {
		return fmt.Errorf("reroute needs --to: a correction that does not say where is not a correction")
	}

	s, ctx, err := openStore(*db)
	if err != nil {
		return err
	}
	defer s.Close()

	old, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	// Correcting a correction would leave two stubs pointing at each other and
	// no way to tell which one anybody meant.
	if v, ok := old.Get(supersededBy); ok && strings.TrimSpace(v) != "" {
		return fmt.Errorf("%s was already corrected, by %s. Reroute that one instead", old.ID, strings.TrimSpace(v))
	}
	if strings.TrimSpace(old.Body) == "" {
		return fmt.Errorf("%s has no body to re-file; amend it rather than rerouting it", old.ID)
	}
	// This corrects a jot, and a jot is a record intake filed: "Routed to" is
	// written by intake.File and by nothing else, so its presence is exactly
	// the question being asked.
	//
	// Without it, reroute took anything with a body. A project record went
	// through: its description was filed as a fresh finding in the idea inbox
	// and the record defining the store's own prefix was marked superseded by
	// it. Nothing in the command's purpose covered that, and nothing in the
	// command stopped it (MUS-F-0058).
	if _, routed := old.Get("Routed to"); !routed {
		return fmt.Errorf("%s was not filed through the intake box, so it has no routing to correct. "+
			"reroute is for a jot that \"Route it for me\" put in the wrong place", old.ID)
	}

	// Filed through intake rather than written here, so the destination is
	// resolved, the prefix is chosen and the fields are shaped by exactly the
	// code that files everything else. A correction that took its own path
	// would drift from the thing it is correcting.
	fresh, dest, err := intake.File(ctx, s, intake.Request{
		Project: *project,
		Text:    old.Body,
		Actor:   *actor,
		Now:     time.Now(),
		To:      *to,
		// A correction re-files the record's own body, so it matches its own
		// original exactly. Without this it is handed the original back and
		// told it is already routed there, for the first minute after filing —
		// which is the minute somebody notices the routing was wrong.
		Deliberate: true,
	})
	if err != nil {
		return err
	}
	if fresh.ID == old.ID {
		return fmt.Errorf("%s is already routed there", old.ID)
	}

	note := strings.TrimSpace(*why)
	if note == "" {
		note = "routed to " + fieldOr(old, "Routed to", "nowhere") + " when it belonged to " + dest.Name
	}

	// A correction is about where the record lives, not what it says. So intake
	// keeps only the two things it alone can decide — the destination and the
	// prefix in the identifier — and everything the record actually claimed is
	// carried across unchanged.
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
	fresh.Data = kept
	for _, f := range old.Refs {
		if !routingField(f.Key) {
			fresh.Refs = append(fresh.Refs, f)
		}
	}
	fresh.Data = append(fresh.Data, record.Field{Key: "Corrects", Value: old.ID + " — " + note})
	fresh.Refs = append(fresh.Refs, record.Field{Key: "Corrects", Value: old.ID})
	if err := s.Append(ctx, fresh, "amend", *actor); err != nil {
		return err
	}

	// The old one stays, still resolving, and stops making a claim.
	setStatus(&old, "superseded")
	old.Data = append(old.Data, record.Field{Key: supersededBy, Value: fresh.ID + " — " + note})
	old.Refs = append(old.Refs, record.Field{Key: supersededBy, Value: fresh.ID})
	if err := s.Append(ctx, old, "amend", *actor); err != nil {
		return err
	}

	// The pictures go with the record, not with the stub. A jot filed from a
	// phone carries its evidence in the attachment, and leaving it behind means
	// the record anybody reads has none.
	moved, err := s.MoveAttachments(ctx, old.ID, fresh.ID)
	if err != nil {
		return err
	}

	fmt.Printf("%s now carries it, routed to %s.\n%s stays, superseded and still resolving.\n",
		fresh.ID, dest.Name, old.ID)
	if moved > 0 {
		fmt.Printf("%d picture(s) moved across.\n", moved)
	}
	return nil
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

// fieldOr reads a field, or returns a fallback.
func fieldOr(r record.Record, key, fallback string) string {
	if v, ok := r.Get(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}
