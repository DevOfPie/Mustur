// Package store is Mustur's SQLite store: an append-only event log with a
// materialized latest, opened by one process at a time.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
	_ "modernc.org/sqlite" // Pure Go: the plan's static binary rules out cgo.
)

//go:embed schema.sql
var schema string

// ErrNotFound is returned for an identifier the store has never seen.
var ErrNotFound = errors.New("no record with that identifier")

// Store is an open database.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open opens or creates the store at path and applies the schema. Applying it
// to an existing store is a no-op.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	// One writer. The store is a single long-lived process's, and a second
	// writer would interleave sequence numbers the export depends on.
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema to %s: %w", path, err)
	}
	return &Store{db: db, now: time.Now}, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// Append writes one event. op is "create" for an identifier the store has not
// seen and "amend" for one it has; passing the wrong one is an error rather
// than a correction, because the caller knowing which it is means the caller
// checked.
func (s *Store) Append(ctx context.Context, r record.Record, op, actor string) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if op != "create" && op != "amend" {
		return fmt.Errorf("record %s: op %q is not create or amend", r.ID, op)
	}
	if actor == "" {
		return fmt.Errorf("record %s: no actor", r.ID)
	}
	existing, err := s.exists(ctx, r.ID)
	if err != nil {
		return err
	}
	switch {
	case existing && op == "create":
		return fmt.Errorf("record %s already exists: amend it or choose a new identifier", r.ID)
	case !existing && op == "amend":
		return fmt.Errorf("record %s does not exist: nothing to amend", r.ID)
	}
	payload, err := r.MarshalPayload()
	if err != nil {
		return fmt.Errorf("record %s: %w", r.ID, err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO record_event (record_id, kind, op, at, actor, payload, written_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Kind, op, r.At, actor, string(payload), s.now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("append %s: %w", r.ID, err)
	}
	return nil
}

func (s *Store) exists(ctx context.Context, id string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM record_latest WHERE record_id = ?`, id).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("look up %s: %w", id, err)
	}
	return n > 0, nil
}

// Get returns the latest state of one record.
func (s *Store) Get(ctx context.Context, id string) (record.Record, error) {
	if !ident.Valid(id) {
		return record.Record{}, fmt.Errorf("%q is not an identifier", id)
	}
	var payload string
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM record_latest WHERE record_id = ?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return record.Record{}, fmt.Errorf("%s: %w", id, ErrNotFound)
	}
	if err != nil {
		return record.Record{}, fmt.Errorf("read %s: %w", id, err)
	}
	return record.UnmarshalPayload([]byte(payload))
}

// List returns the latest state of every record of a kind, sorted. An empty
// kind returns every record.
func (s *Store) List(ctx context.Context, kind string) ([]record.Record, error) {
	query := `SELECT payload FROM record_latest ORDER BY record_id`
	args := []any{}
	if kind != "" {
		query = `SELECT payload FROM record_latest WHERE kind = ? ORDER BY record_id`
		args = append(args, kind)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list %q: %w", kind, err)
	}
	defer rows.Close()
	var out []record.Record
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		r, err := record.UnmarshalPayload([]byte(payload))
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	record.Sort(out)
	return out, nil
}

// Event is one entry in the log.
type Event struct {
	Seq    int64
	Op     string
	At     string
	Actor  string
	Record record.Record
}

// History returns every event for one identifier, oldest first.
func (s *Store) History(ctx context.Context, id string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT seq, op, at, actor, payload FROM record_event WHERE record_id = ? ORDER BY seq`, id)
	if err != nil {
		return nil, fmt.Errorf("history of %s: %w", id, err)
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var payload string
		if err := rows.Scan(&e.Seq, &e.Op, &e.At, &e.Actor, &payload); err != nil {
			return nil, err
		}
		if e.Record, err = record.UnmarshalPayload([]byte(payload)); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Count returns how many records the store holds.
func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM record_latest`).Scan(&n)
	return n, err
}

// Rebuild re-derives record_latest from the log. It is the answer to a wrong
// materializing trigger, and the reason the cache needs no protection.
func (s *Store) Rebuild(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM record_latest`); err != nil {
		return fmt.Errorf("clear materialized latest: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO record_latest (record_id, kind, seq, at, payload)
		SELECT e.record_id, e.kind, e.seq, e.at, e.payload
		FROM record_event e
		WHERE e.seq = (SELECT max(seq) FROM record_event x WHERE x.record_id = e.record_id)`)
	if err != nil {
		return fmt.Errorf("replay log: %w", err)
	}
	return tx.Commit()
}

// NextID allocates the next identifier for a project and role: one past the
// highest serial the log has ever carried, not one past the count.
//
// The distinction matters and is the reason this reads the event log rather
// than the materialized latest. Serials are never reused. A record that was
// created and later corrected still occupies its number, and a numbering
// scheme that filled gaps would make an identifier written in a report today
// point at a different record next year.
func (s *Store) NextID(ctx context.Context, project string, role ident.Role) (string, error) {
	prefix := fmt.Sprintf("%s-%s-", project, role)
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT record_id FROM record_event WHERE record_id LIKE ? ORDER BY record_id`, prefix+"%")
	if err != nil {
		return "", fmt.Errorf("find the highest %s serial: %w", prefix, err)
	}
	defer rows.Close()
	highest := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		parsed, err := ident.Parse(id)
		if err != nil {
			continue // Not ours to interpret; NextID only counts what it understands.
		}
		if parsed.Serial > highest {
			highest = parsed.Serial
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return ident.ID{Project: project, Role: role, Serial: highest + 1}.String(), nil
}
