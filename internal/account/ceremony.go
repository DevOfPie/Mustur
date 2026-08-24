package account

// A WebAuthn ceremony spans two requests: one issues a challenge, the other
// verifies what the authenticator signed. The challenge has to survive between
// them unmodified, which is the whole security property — a challenge the
// browser could choose is not a challenge.
//
// It lives in the database rather than in memory. In memory it would be lost by
// a restart mid-ceremony, which fails as a mysterious rejection rather than as
// anything a reader could diagnose; and it would tie the surface to one process
// for no reason. The cookie the browser holds carries only the row's id.

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// CeremonyLife is how long a half-finished ceremony is good for.
//
// Five minutes: long enough to find the phone, unlock it and approve, and short
// enough that a challenge left in a closed tab is not still answerable. The
// library's own timeouts are longer; this is the outer bound.
const CeremonyLife = 5 * time.Minute

// ErrNoCeremony is every reason a ceremony cannot be resumed — unknown, spent,
// expired — and says nothing about which.
var ErrNoCeremony = errors.New("that sign-in attempt is no longer valid")

// A Ceremony is one registration or sign-in in progress.
type Ceremony struct {
	// Purpose is "register" or "signin", checked on resume so a challenge
	// issued for one cannot be spent on the other.
	Purpose string
	// Data is the library's session data, as JSON, byte for byte as issued.
	Data []byte
	// Handle is the user handle a registration committed to before the account
	// existed. Empty for a sign-in.
	Handle []byte
	// Secret is the invitation being redeemed. Empty for a sign-in.
	Secret string
}

// BeginCeremony stores a challenge and returns the id to put in a cookie.
func (s *Store) BeginCeremony(ctx context.Context, c Ceremony) (string, error) {
	id, _, err := token()
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO webauthn_ceremony (id, purpose, data, handle, secret, created, expires)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, c.Purpose, string(c.Data), c.Handle, c.Secret,
		now.Format(stamp), now.Add(CeremonyLife).Format(stamp))
	if err != nil {
		return "", err
	}
	// Opportunistic sweep. Nothing else deletes these, and a table of dead
	// challenges is a slow leak rather than a bug — but it is still a leak, and
	// each row holds a user handle.
	_, _ = s.db.ExecContext(ctx, `DELETE FROM webauthn_ceremony WHERE expires <= ?`, now.Format(stamp))
	return id, nil
}

// TakeCeremony resumes a ceremony and removes it in the same breath.
//
// Removed whether or not the verification that follows succeeds: a challenge is
// one attempt, and one that could be retried is one an attacker can grind
// against. The caller starts again rather than trying twice.
func (s *Store) TakeCeremony(ctx context.Context, id, purpose string) (Ceremony, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Ceremony{}, err
	}
	defer tx.Rollback()

	var c Ceremony
	var data, expires string
	err = tx.QueryRowContext(ctx,
		`SELECT purpose, data, COALESCE(handle, x''), COALESCE(secret, ''), expires
		 FROM webauthn_ceremony WHERE id = ?`, id).
		Scan(&c.Purpose, &data, &c.Handle, &c.Secret, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return Ceremony{}, ErrNoCeremony
	}
	if err != nil {
		return Ceremony{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM webauthn_ceremony WHERE id = ?`, id); err != nil {
		return Ceremony{}, err
	}
	if err := tx.Commit(); err != nil {
		return Ceremony{}, err
	}

	end, err := time.Parse(stamp, expires)
	if err != nil {
		return Ceremony{}, err
	}
	if s.now().UTC().After(end) || c.Purpose != purpose {
		return Ceremony{}, ErrNoCeremony
	}
	c.Data = []byte(data)
	return c, nil
}

// AccountByEmail finds an existing account, so a ceremony can commit to the
// user handle it already has rather than inventing a second one for the same
// person.
func (s *Store) AccountByEmail(ctx context.Context, email string) (Account, bool) {
	var a Account
	var created, disabled string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, created, COALESCE(disabled, '') FROM account WHERE email = ?`,
		email).Scan(&a.ID, &a.Email, &created, &disabled)
	if err != nil {
		return Account{}, false
	}
	a.Created, _ = time.Parse(stamp, created)
	a.Disabled = disabled != ""
	return a, true
}
