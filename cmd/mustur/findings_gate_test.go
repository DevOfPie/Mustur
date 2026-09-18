package main

// The finding state gate reads the store (MUS-D-0196), like the question gate
// (MUS-D-0183), and says out loud when there is nothing to check against.

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/seed"
	"github.com/DevOfPie/Mustur/internal/status"
	"github.com/DevOfPie/Mustur/internal/store"
)

// captured runs f with standard output taken, and returns what it printed.
func captured(f func() error) (string, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	was := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	ferr := f()
	os.Stdout = was
	w.Close()
	return <-done, ferr
}

func findingsStore(t *testing.T, rs ...record.Record) string {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "t.db")
	s, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, r := range rs {
		if err := s.Append(ctx, r, "create", "test"); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func mus(words ...string) record.Record {
	r := record.Record{ID: "MUS-P-0001", Kind: "project", Title: "Mustur", At: "2026-09-18",
		Data: []record.Field{{Key: status.PrefixField, Value: "MUS"}}}
	for _, w := range words {
		r.Data = append(r.Data, record.Field{Key: status.WordField, Value: w})
	}
	return r
}

func aFinding(id, word, state string) record.Record {
	r := record.Record{ID: id, Kind: "finding", Title: "t", At: "2026-09-18"}
	if word != "" {
		r.Data = append(r.Data, record.Field{Key: status.StatusField, Value: word})
	}
	if state != "" {
		r.Data = append(r.Data, record.Field{Key: status.StateField, Value: state})
	}
	return r
}

func TestTheFindingGatePassesAStoreWhoseFindingsAreAllPlaced(t *testing.T) {
	path := findingsStore(t,
		mus("open = open :: work remains", "fixed = done :: repaired"),
		aFinding("MUS-F-0001", "open", status.Open),
		aFinding("MUS-F-0002", "fixed", status.Done))
	out, err := captured(func() error { return cmdVerify([]string{"--findings", "--db", path}) })
	if err != nil || !strings.Contains(out, "ok    2 finding(s)") {
		t.Errorf("err %v, out %q", err, out)
	}
}

func TestTheFindingGateFailsOnAFindingItCannotPlace(t *testing.T) {
	path := findingsStore(t,
		mus("open = open :: work remains", "fixed = done :: repaired"),
		aFinding("MUS-F-0001", "open", ""),
		aFinding("MUS-F-0002", "resolved", status.Done),
		aFinding("MUS-F-0003", "open", "closed"))
	out, err := captured(func() error { return cmdVerify([]string{"--findings", "--db", path}) })
	if err == nil {
		t.Fatalf("passed:\n%s", out)
	}
	for _, want := range []string{"MUS-F-0001: it has no State", `MUS-F-0002: Status "resolved" is not a word`, `MUS-F-0003: State "closed" is not`} {
		if !strings.Contains(out, "FAIL  "+want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// A fresh seed predates the lists: it is skipped out loud, never passed as if
// it had been checked and never failed for being fresh.
func TestTheFindingGateSaysItDidNotRunOnAStoreWithNoLists(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "t.db")
	s, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Apply(ctx, s); err != nil {
		t.Fatal(err)
	}
	s.Close()
	out, err := captured(func() error { return cmdVerify([]string{"--findings", "--db", path}) })
	if err != nil || !strings.Contains(out, "skip  finding state gate did not run") {
		t.Errorf("err %v, out %q", err, out)
	}
}

// Scoped to a project, the gate reads that project's findings and list only:
// the store is shared, and another project's finding is not this branch's to
// fail on.
func TestTheFindingGateScopedToAProjectIgnoresTheOthers(t *testing.T) {
	hrd := record.Record{ID: "MUS-P-0003", Kind: "project", Title: "Hoard", At: "2026-09-18",
		Data: []record.Field{{Key: status.PrefixField, Value: "HRD"}, {Key: status.WordField, Value: "not a word"}}}
	path := findingsStore(t,
		mus("open = open :: work remains"),
		hrd,
		aFinding("MUS-F-0001", "open", status.Open),
		aFinding("HRD-F-0001", "prose, not a word", "closed"))
	out, err := captured(func() error { return cmdVerify([]string{"--findings", "--db", path, "--project", "MUS"}) })
	if err != nil || !strings.Contains(out, "ok    1 MUS finding(s)") {
		t.Errorf("scoped: err %v, out %q", err, out)
	}
	out, err = captured(func() error { return cmdVerify([]string{"--findings", "--db", path}) })
	if err == nil || !strings.Contains(out, "HRD-F-0001") || !strings.Contains(out, "MUS-P-0003") {
		t.Errorf("store-wide: err %v, out %q", err, out)
	}
}

// A missing store is reported, and not created by being looked for.
func TestTheFindingGateLeavesNoStoreBehind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.db")
	if err := cmdVerify([]string{"--findings", "--db", path}); err == nil || !strings.Contains(err.Error(), "no store at") {
		t.Errorf("err = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("looking for a store made one: %v", err)
	}
}

func TestTheFindingGateNeedsAStore(t *testing.T) {
	if err := cmdVerify([]string{"--findings"}); err == nil || !strings.Contains(err.Error(), "--db") {
		t.Errorf("err = %v", err)
	}
}
