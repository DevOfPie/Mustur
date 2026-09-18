package store

import (
	"errors"
	"strings"
	"testing"
)

func guardSQL(t *testing.T, s *Store) string {
	t.Helper()
	var sql string
	if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = 'record_event_no_update'`).Scan(&sql); err != nil {
		t.Fatalf("the insert-only guard is gone: %v", err)
	}
	return sql
}

// A failure after the guard is dropped and the log rewritten rolls all of it
// back: the trigger is there, word for word, and no event moved.
func TestARenameThatFailsAfterLiftingTheGuardLeavesItInPlace(t *testing.T) {
	s, ctx := renameFixture(t)
	guard, before := guardSQL(t, s), dump(t, s)
	boom := errors.New("injected")
	_, err := s.Rename(ctx, intakeBox, RenameOptions{
		Keep: []string{"MUS-D-0192"}, Apply: true,
		failAfterLift: func() error { return boom },
	})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want the injected failure", err)
	}
	if got := guardSQL(t, s); got != guard {
		t.Errorf("the guard came back as %q, was %q", got, guard)
	}
	if dump(t, s) != before {
		t.Error("the rolled-back rename left events rewritten")
	}
	if _, err := s.db.Exec(`UPDATE record_event SET actor = 'x'`); err == nil || !strings.Contains(err.Error(), "insert-only") {
		t.Errorf("the log is not guarded after the rollback: %v", err)
	}
}

// A payload that is not what MarshalPayload writes cannot be rewritten without
// restating bytes the rename does not name, so the rename refuses it.
func TestARenameRefusesAPayloadThatDoesNotRoundTrip(t *testing.T) {
	s, ctx := renameFixture(t)
	if _, err := s.db.Exec(`INSERT INTO record_event (record_id, kind, op, at, actor, payload, written_at)
		VALUES ('MUS-F-0200', 'finding', 'create', '2026-09-05', 'test',
		'{"id": "MUS-F-0200", "kind": "finding", "title": "spaced", "at": "2026-09-05", "body": "IDW-F-0001"}', 'now')`); err != nil {
		t.Fatal(err)
	}
	before := dump(t, s)
	for _, apply := range []bool{false, true} {
		_, err := s.Rename(ctx, intakeBox, RenameOptions{Keep: []string{"MUS-D-0192"}, Apply: apply})
		if err == nil || !strings.Contains(err.Error(), "does not round-trip") {
			t.Errorf("apply=%v gave %v", apply, err)
		}
	}
	if dump(t, s) != before {
		t.Error("a refused rename wrote")
	}
}
