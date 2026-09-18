package main

import (
	"context"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/store"
)

// The command reads pairs and flags in any order, lists without --apply, and
// writes with it. What the rename does is store.Rename's, and tested there.
func TestRenameListsThenApplies(t *testing.T) {
	path, id := jotted(t)
	to := "_IB-F-" + id[len(id)-4:]
	get := func() (bool, bool) {
		s, err := store.Open(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		_, oldErr := s.Get(context.Background(), id)
		_, newErr := s.Get(context.Background(), to)
		return oldErr == nil, newErr == nil
	}

	if err := cmdRename([]string{"--db", path, id + "=" + to}); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if hasOld, hasNew := get(); !hasOld || hasNew {
		t.Fatalf("a dry run renamed: old %v, new %v", hasOld, hasNew)
	}
	if err := cmdRename([]string{id + "=" + to, "--apply", "--db", path}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if hasOld, hasNew := get(); hasOld || !hasNew {
		t.Fatalf("not renamed: old %v, new %v", hasOld, hasNew)
	}
	if err := cmdRename([]string{"--db", path}); err == nil || !strings.Contains(err.Error(), "OLD=NEW") {
		t.Errorf("no pairs gave %v", err)
	}
	if err := cmdRename([]string{"--db", path, id}); err == nil || !strings.Contains(err.Error(), "not OLD=NEW") {
		t.Errorf("a bare identifier gave %v", err)
	}
}
