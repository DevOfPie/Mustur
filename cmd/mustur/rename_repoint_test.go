package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/intake"
	"github.com/DevOfPie/Mustur/internal/store"
)

// The window the review of #107 found: rename empties IDW, and until the
// intake box's prefix is changed the next jot takes IDW-F-0001. With
// --repoint the prefix changes in the same transaction, so the next jot after
// the rename is filed under the new prefix and continues its sequence.
func TestRenameRepointsTheIntakeBoxInTheSameTransaction(t *testing.T) {
	path, id := jotted(t) // One jot in the seeded idea inbox, under IDW.
	if !strings.HasPrefix(id, "IDW-F-") {
		t.Fatalf("the seeded inbox filed %s", id)
	}
	to := "_IB-F-" + id[len(id)-4:]

	// A dry run names the repoint and writes nothing.
	if err := cmdRename([]string{"--db", path, id + "=" + to, "--repoint", "MUS-P-0002=_IB"}); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	inbox, _ := s.Get(ctx, "MUS-P-0002")
	if p, _ := inbox.Get(intake.PrefixField); p != "IDW" {
		t.Fatalf("a dry run repointed the inbox to %q", p)
	}
	s.Close()

	if err := cmdRename([]string{"--db", path, id + "=" + to, "--repoint", "MUS-P-0002=_IB", "--apply", "--actor", "test"}); err != nil {
		t.Fatal(err)
	}
	s, err = store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	inbox, _ = s.Get(ctx, "MUS-P-0002")
	if p, _ := inbox.Get(intake.PrefixField); p != "_IB" {
		t.Errorf("the inbox's prefix is %q, want _IB", p)
	}
	if _, ok := inbox.Get(intake.DefaultField); !ok || inbox.Title != "Idea inbox" {
		t.Errorf("the repoint lost the rest of the record: %+v", inbox)
	}
	h, _ := s.History(ctx, "MUS-P-0002")
	if last := h[len(h)-1]; last.Op != "amend" || last.Actor != "test" {
		t.Errorf("the repoint is not an amend by the actor: %+v", last)
	}

	next, _, err := intake.File(ctx, s, intake.Request{Project: "MUS", Text: "after the rename", Actor: "test", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if next.ID != "_IB-F-0002" {
		t.Errorf("the next jot is %s, want _IB-F-0002", next.ID)
	}
}

// --keep given twice keeps both lists rather than the last one (review of
// #107, nit 11).
func TestRenameKeepAccumulates(t *testing.T) {
	path, id := jotted(t)
	to := "_IB-F-" + id[len(id)-4:]
	// An unknown record is refused by name, so each order shows whether both
	// lists arrived.
	err := cmdRename([]string{"--db", path, id + "=" + to, "--keep", "MUS-P-0001", "--keep", "MUS-D-0999"})
	if err == nil || !strings.Contains(err.Error(), "MUS-D-0999") {
		t.Errorf("the second --keep was dropped: %v", err)
	}
	err = cmdRename([]string{"--db", path, id + "=" + to, "--keep", "MUS-D-0999", "--keep", "MUS-P-0001"})
	if err == nil || !strings.Contains(err.Error(), "MUS-D-0999") {
		t.Errorf("the first --keep was dropped: %v", err)
	}
}

func TestRenameRefusesARepointThatIsNotARoutingRecord(t *testing.T) {
	path, id := jotted(t)
	to := "_IB-F-" + id[len(id)-4:]
	for _, c := range []struct{ arg, want string }{
		{"MUS-P-0099=_IB", "no such record"},
		{id + "=_IB", "being renamed"},
		{"MUS-P-0002=IB", "not ROUTING-ID=PREFIX"},
		{"MUS-P-0002", "not ROUTING-ID=PREFIX"},
	} {
		err := cmdRename([]string{"--db", path, id + "=" + to, "--repoint", c.arg, "--apply"})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("--repoint %s gave %v, want %q", c.arg, err, c.want)
		}
	}
	// A record of a kind no jot is routed to.
	s, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	decisions, _ := s.List(context.Background(), "decision")
	s.Close()
	if len(decisions) == 0 {
		t.Fatal("the seed holds no decision to try")
	}
	err = cmdRename([]string{"--db", path, id + "=" + to, "--repoint", decisions[0].ID + "=_IB", "--apply"})
	if err == nil || !strings.Contains(err.Error(), "not a routing record") {
		t.Errorf("repointing a decision gave %v", err)
	}
}
