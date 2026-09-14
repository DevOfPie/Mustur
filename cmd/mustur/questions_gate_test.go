package main

// The gate reads the store (MUS-D-0183).
//
// It used to read the exported tree, through a --records flag the Makefile
// passed (MUS-D-0050). Since the export is committed on main only, a branch's
// tree does not hold the questions that branch raised, so the gate passed
// around them. These drive cmdQuestions against a store and nothing else.

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/DevOfPie/Mustur/internal/question"
	"github.com/DevOfPie/Mustur/internal/seed"
	"github.com/DevOfPie/Mustur/internal/store"
)

// asked gives a store holding one open MUS question that was never surfaced.
func asked(t *testing.T) (string, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "t.db")
	s, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Apply(ctx, s); err != nil {
		t.Fatal(err)
	}
	before, err := s.List(ctx, question.Kind)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	if err := cmdAsk([]string{"--db", path, "--title", "a question nobody was shown", "--actor", "test"}); err != nil {
		t.Fatal(err)
	}

	s, err = store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	after, err := s.List(ctx, question.Kind)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("ask left %d question(s) in the store, want %d", len(after), len(before)+1)
	}
	seen := map[string]bool{}
	for _, r := range before {
		seen[r.ID] = true
	}
	for _, r := range after {
		if !seen[r.ID] {
			return path, r.ID
		}
	}
	t.Fatal("ask wrote no new question")
	return "", ""
}

func TestGateFailsOnAnUnsurfacedQuestionInTheStore(t *testing.T) {
	path, id := asked(t)

	if err := cmdQuestions([]string{"--gate", "--db", path, "--project", "MUS"}); err == nil {
		t.Fatalf("the gate passed with %s open and never surfaced in the store", id)
	}

	// The control: the same store, the question surfaced, and nothing else
	// changed. Without it the failure above could be the store refusing to
	// open rather than the gate reading what is in it.
	if err := cmdSurfaced([]string{id, "--db", path, "--actor", "test"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdQuestions([]string{"--gate", "--db", path, "--project", "MUS"}); err != nil {
		t.Errorf("the gate still failed once %s was surfaced: %v", id, err)
	}
}

// The flag that read the exported tree is gone, not ignored. An unknown flag
// is refused, so a caller still passing it finds out rather than silently
// gating against the default store.
func TestGateNoLongerTakesTheExportedTree(t *testing.T) {
	path, _ := asked(t)
	if err := cmdQuestions([]string{"--gate", "--db", path, "--records", t.TempDir()}); err == nil {
		t.Error("questions accepted --records; the gate reads only the store (MUS-D-0183)")
	}
}
