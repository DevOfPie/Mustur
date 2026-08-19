package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

func open(t *testing.T) (*Store, context.Context, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	ctx := context.Background()
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, ctx, path
}

func decision(id, title string) record.Record {
	return record.Record{ID: id, Kind: "decision", Title: title, At: "2026-08-19"}
}

func TestAppendAndGet(t *testing.T) {
	s, ctx, _ := open(t)
	if err := s.Append(ctx, decision("MUS-D-0001", "first"), "create", "test"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "MUS-D-0001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "first" {
		t.Fatalf("title %q", got.Title)
	}
}

func TestCreateTwiceIsRefused(t *testing.T) {
	s, ctx, _ := open(t)
	if err := s.Append(ctx, decision("MUS-D-0001", "first"), "create", "test"); err != nil {
		t.Fatal(err)
	}
	err := s.Append(ctx, decision("MUS-D-0001", "again"), "create", "test")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second create gave %v", err)
	}
}

func TestAmendUnknownIsRefused(t *testing.T) {
	s, ctx, _ := open(t)
	err := s.Append(ctx, decision("MUS-D-0001", "first"), "amend", "test")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("amend of an unknown record gave %v", err)
	}
}

func TestAmendKeepsTheEarlierEvent(t *testing.T) {
	s, ctx, _ := open(t)
	if err := s.Append(ctx, decision("MUS-D-0001", "first"), "create", "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(ctx, decision("MUS-D-0001", "corrected"), "amend", "test"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "MUS-D-0001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "corrected" {
		t.Fatalf("latest title %q", got.Title)
	}
	history, err := s.History(ctx, "MUS-D-0001")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Record.Title != "first" {
		t.Fatalf("history %+v", history)
	}
}

// The store's own API exposes no update and no delete. This asserts the other
// half: that the database refuses them to a connection that never heard of the
// store package.
func TestLogRefusesUpdateAndDelete(t *testing.T) {
	s, ctx, path := open(t)
	if err := s.Append(ctx, decision("MUS-D-0001", "first"), "create", "test"); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()

	if _, err := raw.ExecContext(ctx, `UPDATE record_event SET payload = '{}'`); err == nil {
		t.Error("update succeeded and should have been refused")
	} else if !strings.Contains(err.Error(), "insert-only") {
		t.Errorf("update refused for the wrong reason: %v", err)
	}
	if _, err := raw.ExecContext(ctx, `DELETE FROM record_event`); err == nil {
		t.Error("delete succeeded and should have been refused")
	} else if !strings.Contains(err.Error(), "insert-only") {
		t.Errorf("delete refused for the wrong reason: %v", err)
	}
}

// The materialized latest is a cache and is deliberately unprotected. Its
// authority rests on this: that a rebuild repairs it from the log.
func TestRebuildRepairsACorruptedCache(t *testing.T) {
	s, ctx, path := open(t)
	if err := s.Append(ctx, decision("MUS-D-0001", "first"), "create", "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(ctx, decision("MUS-D-0002", "second"), "create", "test"); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `UPDATE record_latest SET payload = '{"id":"MUS-D-0001","kind":"decision","title":"wrong","at":"2026-08-19"}'`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `DELETE FROM record_latest WHERE record_id = 'MUS-D-0002'`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	if err := s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "MUS-D-0001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "first" {
		t.Errorf("rebuilt title %q, want %q", got.Title, "first")
	}
	if _, err := s.Get(ctx, "MUS-D-0002"); err != nil {
		t.Errorf("record deleted from the cache did not come back: %v", err)
	}
}

func TestValidationRefusesAMismatchedRole(t *testing.T) {
	s, ctx, _ := open(t)
	bad := record.Record{ID: "MUS-D-0001", Kind: "finding", Title: "wrong role", At: "2026-08-19"}
	if err := s.Append(ctx, bad, "create", "test"); err == nil {
		t.Fatal("a decision identifier carrying kind finding was accepted")
	}
}
