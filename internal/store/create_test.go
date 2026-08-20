package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
)

// The test that fails on the code this replaced. Allocating an identifier and
// writing under it were two calls, so two writers could read the same highest
// serial and both claim it — one insert winning and the other silently
// overwriting it in the materialized latest, with both callers told their
// record was filed.
func TestConcurrentCreatesDoNotCollide(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const writers = 16
	var wg sync.WaitGroup
	ids := make([]string, writers)
	errs := make([]error, writers)
	start := make(chan struct{})
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			r, err := s.Create(ctx, record.Record{
				Kind: "finding", Title: titleFor(i), At: "2026-08-20",
			}, "MUS", ident.Finding, "test")
			ids[i], errs[i] = r.ID, err
		}(i)
	}
	close(start)
	wg.Wait()

	seen := map[string]bool{}
	for i, id := range ids {
		if errs[i] != nil {
			t.Fatalf("writer %d failed: %v", i, errs[i])
		}
		if seen[id] {
			t.Fatalf("%s was issued twice", id)
		}
		seen[id] = true
	}

	// Every record that was accepted is readable, with the title its writer
	// gave it. A jot answered "filed" and then not there is the failure this
	// exists to stop.
	all, err := s.List(ctx, "finding")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != writers {
		t.Fatalf("%d records for %d accepted writes", len(all), writers)
	}
	titles := map[string]bool{}
	for _, r := range all {
		titles[r.Title] = true
	}
	for i := 0; i < writers; i++ {
		if !titles[titleFor(i)] {
			t.Errorf("writer %d was told it was filed and its record is not there", i)
		}
	}
}

func titleFor(i int) string {
	return string(rune('a'+i)) + " concurrent jot"
}

func TestCreateRefusesARecordItCannotValidate(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// A kind that disagrees with the role letter it would be allocated under.
	if _, err := s.Create(ctx, record.Record{Kind: "decision", Title: "wrong", At: "2026-08-20"},
		"MUS", ident.Finding, "test"); err == nil {
		t.Fatal("a mismatched kind was accepted")
	}
	n, err := s.Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d records written by a refused create", n)
	}
}
