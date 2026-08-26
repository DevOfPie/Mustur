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
	// SQLite parses REFERENCES and then ignores it unless this is on, per
	// connection. modernc's driver already switches it on, which a review missed
	// and this comment nearly repeated: measured with no pragma at all, a grant
	// to a nonexistent account id still fails with SQLITE_CONSTRAINT_FOREIGNKEY.
	// The line stays so the guarantee is this schema's rather than a driver
	// default that could change under it. The record tables declare no
	// references, so it changes nothing for them.
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys on %s: %w", path, err)
	}
	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema to %s: %w", path, err)
	}
	if err := addMissingColumns(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return &Store{db: db, now: time.Now}, nil
}

// addMissingColumns is the whole of this repository's migration story.
//
// `CREATE TABLE IF NOT EXISTS` builds a table that is missing and then says
// nothing about one that exists with the wrong shape, so a column added to
// schema.sql after a store was created never appears in it. That is not a
// theoretical gap: MUS-F-0029 was a column this file did not have, and the
// store that needed it was the owner's live one.
//
// Deliberately the smallest thing that works. Columns are added, never dropped
// or retyped, and every addition carries a default so existing rows stay valid.
// Anything a column cannot express — a table split, a value that has to be
// recomputed — is a bigger decision than a helper should take on its own.
//
// The record tables are not listed here and are not expected to be. They are
// insert-only and their shape is the export's contract; changing one is a
// decision, not a migration.
func addMissingColumns(ctx context.Context, db *sql.DB) error {
	wanted := []struct{ table, column, spec string }{
		// MUS-F-0029: without these, every synced passkey registers and then
		// fails to sign in, because WebAuthn asks the relying party to notice a
		// change in backup eligibility and an unstored flag reads as false.
		{"credential", "backup_eligible", "INTEGER NOT NULL DEFAULT 0"},
		{"credential", "backup_state", "INTEGER NOT NULL DEFAULT 0"},
		// MUS-Q-0055: a token's optional lifetime.
		{"agent_token", "expires", "TEXT"},
	}
	for _, w := range wanted {
		has, err := hasColumn(ctx, db, w.table, w.column)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		// Table names and column specs are literals above, never input.
		if _, err := db.ExecContext(ctx,
			"ALTER TABLE "+w.table+" ADD COLUMN "+w.column+" "+w.spec); err != nil {
			return fmt.Errorf("add %s.%s: %w", w.table, w.column, err)
		}
	}
	return nil
}

func hasColumn(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// DB exposes the handle so accounts can live in the same file.
//
// Deliberately narrow: the account tables are mutable operational state and
// have no business in the record log, but two SQLite files would be two things
// to back up and keep consistent. Nothing that writes a *record* uses this —
// records go through Append, which is where the insert-only rule lives.
func (s *Store) DB() *sql.DB { return s.db }

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
	return nextID(ctx, s.db, project, role)
}

// querier is what both a database and a transaction can do. Allocation and
// insertion have to be able to run inside one transaction, and outside one for
// a plain read.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func nextID(ctx context.Context, q querier, project string, role ident.Role) (string, error) {
	prefix := fmt.Sprintf("%s-%s-", project, role)
	rows, err := q.QueryContext(ctx,
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

// Create allocates an identifier and writes the record under it, in one
// transaction.
//
// Not two calls. Allocating and inserting separately lets two writers read the
// same highest serial and both claim it: one insert wins, the other is refused
// or — worse, before this — silently overwrote the first in the materialized
// latest while both callers were told their record was filed. Demonstrated with
// twelve concurrent filings, two of which were issued the same identifier and
// one of which then existed nowhere a reader would look.
//
// The transaction is what makes the read and the write one act. Nothing else
// here can restore a jot that was accepted and lost.
func (s *Store) Create(ctx context.Context, r record.Record, project string, role ident.Role, actor string) (record.Record, error) {
	if actor == "" {
		return record.Record{}, fmt.Errorf("no actor")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return record.Record{}, err
	}
	defer tx.Rollback()

	id, err := nextID(ctx, tx, project, role)
	if err != nil {
		return record.Record{}, err
	}
	r.ID = id
	if err := r.Validate(); err != nil {
		return record.Record{}, err
	}
	payload, err := r.MarshalPayload()
	if err != nil {
		return record.Record{}, fmt.Errorf("record %s: %w", r.ID, err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO record_event (record_id, kind, op, at, actor, payload, written_at)
		 VALUES (?, ?, 'create', ?, ?, ?, ?)`,
		r.ID, r.Kind, r.At, actor, string(payload), s.now().UTC().Format(time.RFC3339))
	if err != nil {
		return record.Record{}, fmt.Errorf("create %s: %w", r.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return record.Record{}, err
	}
	return r, nil
}

// Since returns the records created since a moment, newest first.
//
// It reads the log's own written_at rather than the records' dates. A record
// carries the date its content was true, which is not the same as when it was
// written and is only accurate to the day — so a surface asking "what did I
// file in the last hour" cannot be answered from it. The log has the answer;
// this is the only place that distinction is worth making, and making it
// anywhere else would put a wall clock into the record.
func (s *Store) Since(ctx context.Context, kind string, since time.Time) ([]record.Record, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.payload
		FROM record_latest l
		JOIN (SELECT record_id, min(written_at) AS first_written
		      FROM record_event WHERE op = 'create' GROUP BY record_id) e
		  ON e.record_id = l.record_id
		WHERE (? = '' OR l.kind = ?) AND e.first_written >= ?
		ORDER BY e.first_written DESC`,
		kind, kind, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("list %q since %s: %w", kind, since, err)
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
	return out, rows.Err()
}
