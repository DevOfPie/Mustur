package store

// Scratch: a place to file something that was never meant to be kept.
//
// The owner tested the picture upload twice and it cost two identifiers in the
// idea warehouse — `IDW-F-0002` and `IDW-F-0003`, both of which say "test" in
// their own titles and now sit in the records forever, because an identifier
// here is permanent and the log only ever grows. That is the whole complaint:
// **a test filing should not advance a counter.**
//
// The obvious shape was a record with an expiry, and it is the wrong one. The
// log is insert-only and the exported tree is what a reader checks without
// running the binary (MUS-D-0024); a record that later vanishes puts an
// exception under both, and an exception is what the next one argues from.
//
// So a scratch jot is **not a record**. It never enters the log, never takes an
// identifier, never reaches the export and is never counted. It lives here,
// beside the records rather than among them, and it goes: everything is dropped
// when the process starts, which is the owner's own "or until a restart", and
// swept by age while it runs so a machine left up for a week does not
// accumulate them either.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// ScratchLife is how long a scratch jot survives a running process.
//
// Long enough to file something on a phone and read it back at a desk; short
// enough that nobody starts relying on it. Anything worth keeping is worth an
// identifier, and that is what the idea inbox is for.
const ScratchLife = 24 * time.Hour

// A Scratch is a filing that was never meant to last.
type Scratch struct {
	ID      string
	Text    string
	Created time.Time
	By      string
}

// Scratched writes one. It returns an id that is deliberately not a record
// identifier and will not parse as one — nothing should be able to cite this.
func (s *Store) Scratched(ctx context.Context, text, by string) (Scratch, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Scratch{}, errors.New("nothing to file")
	}
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return Scratch{}, err
	}
	sc := Scratch{
		ID:      "scratch-" + hex.EncodeToString(raw),
		Text:    text,
		Created: s.now().UTC(),
		By:      by,
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO scratch (id, text, created, created_by) VALUES (?, ?, ?, ?)`,
		sc.ID, sc.Text, sc.Created.Format(time.RFC3339), sc.By); err != nil {
		return Scratch{}, err
	}
	return sc, nil
}

// Scratches lists what is still here, newest first.
func (s *Store) Scratches(ctx context.Context) ([]Scratch, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, text, created, created_by FROM scratch ORDER BY created DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Scratch
	for rows.Next() {
		var sc Scratch
		var created string
		if err := rows.Scan(&sc.ID, &sc.Text, &created, &sc.By); err != nil {
			return nil, err
		}
		sc.Created, _ = time.Parse(time.RFC3339, created)
		out = append(out, sc)
	}
	return out, rows.Err()
}

// SweepScratch drops what has aged out, and any pictures attached to it.
//
// Pass a zero cutoff to drop everything, which is what a starting process does.
func (s *Store) SweepScratch(ctx context.Context, olderThan time.Time) (int, error) {
	where, args := "", []any{}
	if !olderThan.IsZero() {
		where = " WHERE created < ?"
		args = append(args, olderThan.UTC().Format(time.RFC3339))
	}
	// The pictures first, and by joining rather than by remembering to: an
	// attachment whose scratch is gone is a file nobody can reach and nobody
	// deletes.
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM attachment WHERE record_id IN (SELECT id FROM scratch`+where+`)`,
		args...); err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM scratch`+where, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
