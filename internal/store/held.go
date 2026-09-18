package store

// Held jots: what a reader sent through intake, waiting for an owner.
//
// The owner chose on MUS-Q-0138 that a reader may file, and that what they file
// is held until an owner approves or discards it; MUS-Q-0143 approved the plan
// that says how (MUS-D-0189). Held is not a record state. It is a row in its own
// table, beside the records rather than among them, for the reason scratch is:
// the log is insert-only and exported, and nothing a reader has sent should
// reach it, or any reading of it, until somebody who owns the destination has
// looked.
//
// **A row is acted on once.** Approve and Discard both remove the row in one
// statement's worth of transaction, so two presses — two tabs, or a phone
// resending a POST — find it gone the second time rather than filing twice.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// HoldRetry is how long the same text from the same account is taken to be the
// same send. It is intake.Window's reasoning applied to the held table: a phone
// on a flaky connection sends one POST three times, and post/redirect/get only
// protects a reload after the redirect.
const HoldRetry = time.Minute

// A Held jot is one reader's line, waiting.
type Held struct {
	ID   string
	Text string
	// To is the routing identifier the reader chose, and empty for "Route it
	// for me".
	To        string
	AccountID string
	// Email is the account's, joined at read time, and empty for an account
	// that no longer exists.
	Email   string
	Created time.Time
}

// ErrHeldGone says a held jot is no longer held: approved, discarded, or never
// there. A second press is the usual cause and not worth alarming anybody over.
var ErrHeldGone = errors.New("that jot is no longer waiting: it was already filed or discarded")

// Hold keeps a reader's jot for an owner.
func (s *Store) Hold(ctx context.Context, text, to, accountID string) (Held, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Held{}, errors.New("nothing to send")
	}
	if accountID == "" {
		return Held{}, errors.New("a held jot needs the account that sent it")
	}
	to = strings.TrimSpace(to)
	now := s.now().UTC()
	// A retry of the same send, not a second jot. Compared in the query rather
	// than by listing, and on the stored text, which is already trimmed.
	//
	// The destination is not part of what makes it the same send, which is
	// intake.Window's rule: same text, same filer, inside the minute. What
	// differs is that a filed record cannot be changed and a held row can, so
	// a resend with a different Where moves the row there rather than being
	// dropped in favour of the first. The last thing the reader chose is where
	// they meant it to go, and one line stays one row (MUS-Q-0151).
	var existing Held
	var created string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, text, destination, account_id, created FROM held_jot
		 WHERE account_id = ? AND text = ? AND created >= ?
		 ORDER BY created DESC LIMIT 1`,
		accountID, text, now.Add(-HoldRetry).Format(time.RFC3339)).
		Scan(&existing.ID, &existing.Text, &existing.To, &existing.AccountID, &created)
	switch {
	case err == nil:
		existing.Created, _ = time.Parse(time.RFC3339, created)
		if existing.To != to {
			// Keyed on the id, so a row approved or discarded since the read
			// above is simply not there to move.
			if _, err := s.db.ExecContext(ctx,
				`UPDATE held_jot SET destination = ? WHERE id = ?`, to, existing.ID); err != nil {
				return Held{}, err
			}
			existing.To = to
		}
		return existing, nil
	case !errors.Is(err, sql.ErrNoRows):
		return Held{}, err
	}
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return Held{}, err
	}
	h := Held{
		// Deliberately not a record identifier, like a scratch id: nothing
		// should be able to cite a jot nobody has approved.
		ID:        "held-" + hex.EncodeToString(raw),
		Text:      text,
		To:        to,
		AccountID: accountID,
		Created:   now,
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO held_jot (id, text, destination, account_id, created) VALUES (?, ?, ?, ?, ?)`,
		h.ID, h.Text, h.To, h.AccountID, h.Created.Format(time.RFC3339)); err != nil {
		return Held{}, err
	}
	return h, nil
}

const heldColumns = `SELECT h.id, h.text, h.destination, h.account_id, COALESCE(a.email, ''), h.created
	FROM held_jot h LEFT JOIN account a ON a.id = h.account_id`

// HeldJots lists what is waiting, oldest first so an owner meets them in the
// order they were sent. An empty accountID lists everybody's; otherwise only
// that account's, which is what a reader is shown.
func (s *Store) HeldJots(ctx context.Context, accountID string) ([]Held, error) {
	q, args := heldColumns, []any{}
	if accountID != "" {
		q += ` WHERE h.account_id = ?`
		args = append(args, accountID)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY h.created, h.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Held
	for rows.Next() {
		h, err := scanHeld(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// HeldJot reads one, or ErrHeldGone.
func (s *Store) HeldJot(ctx context.Context, id string) (Held, error) {
	h, err := scanHeld(s.db.QueryRowContext(ctx, heldColumns+` WHERE h.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Held{}, ErrHeldGone
	}
	return h, err
}

type scanner interface{ Scan(...any) error }

func scanHeld(row scanner) (Held, error) {
	var h Held
	var created string
	if err := row.Scan(&h.ID, &h.Text, &h.To, &h.AccountID, &h.Email, &created); err != nil {
		return Held{}, err
	}
	h.Created, _ = time.Parse(time.RFC3339, created)
	return h, nil
}

// Approve takes a held jot out of the table and hands it to file, which is what
// turns it into a record.
//
// The row is claimed first — read and deleted in one transaction — so a second
// Approve, or a Discard racing this one, finds nothing and files nothing. file
// runs after the claim has committed rather than inside it, because it writes
// the record through this same store and the store has one connection: a
// transaction held across it would wait on itself.
//
// If file fails the row is put back as it was, so a refused filing — an
// unknown destination, a store error — leaves the jot waiting rather than lost.
func (s *Store) Approve(ctx context.Context, id string, file func(Held) error) error {
	h, err := s.claim(ctx, id)
	if err != nil {
		return err
	}
	if err := file(h); err != nil {
		if _, rerr := s.db.ExecContext(ctx,
			`INSERT INTO held_jot (id, text, destination, account_id, created) VALUES (?, ?, ?, ?, ?)`,
			h.ID, h.Text, h.To, h.AccountID, h.Created.UTC().Format(time.RFC3339)); rerr != nil {
			return errors.Join(err, rerr)
		}
		return err
	}
	return nil
}

// Discard removes a held jot and leaves nothing behind (MUS-Q-0143): it was
// never a record, so there is nothing in the log to delete or to mark.
func (s *Store) Discard(ctx context.Context, id string) error {
	_, err := s.claim(ctx, id)
	return err
}

func (s *Store) claim(ctx context.Context, id string) (Held, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Held{}, err
	}
	defer tx.Rollback()
	h, err := scanHeld(tx.QueryRowContext(ctx, heldColumns+` WHERE h.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Held{}, ErrHeldGone
	}
	if err != nil {
		return Held{}, err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM held_jot WHERE id = ?`, id)
	if err != nil {
		return Held{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return Held{}, ErrHeldGone
	}
	return h, tx.Commit()
}
