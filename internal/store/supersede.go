package store

// Writes that are only right if nothing changed the record since it was read.
//
// A reroute reads a jot, decides it is not already corrected, files its
// replacement and retires it. Done as separate statements, two presses of the
// same Move button both passed the check and both filed a replacement: two
// records at the destination correcting one jot, the stub naming only one of
// them, and its pictures on whichever moved them last (review of PR 108,
// reproduced 30 of 30). SetMaxOpenConns(1) serialises statements, not the
// read-decide-write between them.
//
// So the read carries the record's version — the sequence number of its latest
// event — and the write happens in one transaction that first checks the
// version is still that. A caller that loses gets ErrChanged and reads again.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
)

// ErrChanged is returned when a record was written by somebody else between
// being read and being written.
var ErrChanged = errors.New("the record changed since it was read")

// GetVersioned returns the latest state of one record and its version.
func (s *Store) GetVersioned(ctx context.Context, id string) (record.Record, int64, error) {
	if !ident.Valid(id) {
		return record.Record{}, 0, fmt.Errorf("%q is not an identifier", id)
	}
	var payload string
	var seq int64
	err := s.db.QueryRowContext(ctx, `SELECT payload, seq FROM record_latest WHERE record_id = ?`, id).Scan(&payload, &seq)
	if errors.Is(err, sql.ErrNoRows) {
		return record.Record{}, 0, fmt.Errorf("%s: %w", id, ErrNotFound)
	}
	if err != nil {
		return record.Record{}, 0, fmt.Errorf("read %s: %w", id, err)
	}
	r, err := record.UnmarshalPayload([]byte(payload))
	return r, seq, err
}

// checkVersion is the compare half of compare-and-set, inside the transaction.
func checkVersion(ctx context.Context, tx *sql.Tx, id string, want int64) error {
	var seq int64
	err := tx.QueryRowContext(ctx, `SELECT seq FROM record_latest WHERE record_id = ?`, id).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s: %w", id, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", id, err)
	}
	if seq != want {
		return fmt.Errorf("%s: %w", id, ErrChanged)
	}
	return nil
}

func (s *Store) insertEvent(ctx context.Context, tx *sql.Tx, r record.Record, op, actor string) error {
	if err := r.Validate(); err != nil {
		return err
	}
	payload, err := r.MarshalPayload()
	if err != nil {
		return fmt.Errorf("record %s: %w", r.ID, err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO record_event (record_id, kind, op, at, actor, payload, written_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Kind, op, r.At, actor, string(payload), s.now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("%s %s: %w", op, r.ID, err)
	}
	return nil
}

// AmendIf amends a record provided its version is still version.
func (s *Store) AmendIf(ctx context.Context, r record.Record, version int64, actor string) error {
	if actor == "" {
		return fmt.Errorf("record %s: no actor", r.ID)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := checkVersion(ctx, tx, r.ID, version); err != nil {
		return err
	}
	if err := s.insertEvent(ctx, tx, r, "amend", actor); err != nil {
		return err
	}
	return tx.Commit()
}

// Supersede files fresh as a new record under project and role, amends the
// record oldID to what retire returns given the new identifier, and moves
// oldID's pictures to the new record — all in one transaction, and only if
// oldID's version is still version. It returns the record as filed and how
// many pictures moved.
func (s *Store) Supersede(ctx context.Context, fresh record.Record, project string, role ident.Role,
	oldID string, version int64, retire func(freshID string) record.Record, actor string) (record.Record, int, error) {
	if actor == "" {
		return record.Record{}, 0, fmt.Errorf("no actor")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return record.Record{}, 0, err
	}
	defer tx.Rollback()
	if err := checkVersion(ctx, tx, oldID, version); err != nil {
		return record.Record{}, 0, err
	}
	id, err := nextID(ctx, tx, project, role)
	if err != nil {
		return record.Record{}, 0, err
	}
	fresh.ID = id
	if err := s.insertEvent(ctx, tx, fresh, "create", actor); err != nil {
		return record.Record{}, 0, err
	}
	old := retire(id)
	if old.ID != oldID {
		return record.Record{}, 0, fmt.Errorf("retiring %s returned %s", oldID, old.ID)
	}
	if err := s.insertEvent(ctx, tx, old, "amend", actor); err != nil {
		return record.Record{}, 0, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE attachment SET record_id = ? WHERE record_id = ?`, id, oldID)
	if err != nil {
		return record.Record{}, 0, fmt.Errorf("move attachments: %w", err)
	}
	moved, err := res.RowsAffected()
	if err != nil {
		return record.Record{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return record.Record{}, 0, err
	}
	return fresh, int(moved), nil
}
