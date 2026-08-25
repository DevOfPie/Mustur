// Package account answers "who is asking", which nothing in Mustur could do
// before milestone 5b.
//
// Cloudflare Access authenticates a person at the edge and Mustur reads the
// header it sets — for attribution, never for a decision. So every identity the
// policy admitted reached every surface, including the two that type into a
// running agent's stdin. The owner asked for accounts of Mustur's own
// (MUS-D-0103), with Access kept in front until this is ready to stand alone.
//
// **Roles are per project** (MUS-Q-0042). A person may read one project and
// have no business in another, and — the reason this exists at all — a reader
// can be invited without inviting somebody who can type into an agent.
//
// **Nothing here is a record.** A record is an immutable claim about the
// project, exported to markdown and read by strangers; an account is mutable
// operational state. They share a database file and nothing else.
package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Role is what an account may do within one project.
//
// Two, and no more until something needs a third. Owner is what the owner has
// everywhere; Reader is the role milestone 6 wanted and Access could not
// express — records, routing and the decision queue, and nothing that writes.
type Role string

const (
	Owner  Role = "owner"
	Reader Role = "reader"
)

// Valid reports whether r is a role this package knows. A role read back from
// a database that has been edited by hand is not trusted to be one of two.
func (r Role) Valid() bool { return r == Owner || r == Reader }

// CanWrite reports whether this role may reach a surface that changes something
// — filing a jot, answering a question, or typing into a running agent.
//
// One predicate rather than a permission per surface. A list of capabilities
// grows a hole every time a surface is added and nobody remembers to update it;
// a question the surface has to ask cannot be forgotten by a new surface,
// because a new surface has to ask something.
func (r Role) CanWrite() bool { return r == Owner }

// InviteLife is how long an invitation is good for.
//
// A day, because an invite is a way in and the person it was sent to is being
// told about it now. Long enough to survive a night; short enough that one
// forwarded months ago is not a key.
const InviteLife = 24 * time.Hour

// SessionLife is how long a signed-in browser stays signed in.
//
// Thirty days, which is the phone case: this exists so the owner is not
// re-authenticating on a device they carry. It is refreshed on use, so a
// browser in daily use never expires and one abandoned in a drawer does.
const SessionLife = 30 * 24 * time.Hour

// Store holds accounts in the same database as the records, beside them rather
// than among them.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// New wraps a database handle. The handle is the records store's, because the
// accounts live in the same file.
func New(db *sql.DB) *Store { return &Store{db: db, now: time.Now} }

// DB exposes the handle for tests that need to reach past this package's own
// API — to disable an account, or to check that a secret was never written.
// Nothing in the package's callers uses it.
func (s *Store) DB() *sql.DB { return s.db }

// WithClock replaces the clock, for tests that need an expiry to have passed.
func (s *Store) WithClock(now func() time.Time) *Store {
	return &Store{db: s.db, now: now}
}

// An Account is a person Mustur will recognise.
type Account struct {
	ID       string
	Email    string
	Created  time.Time
	Disabled bool
}

// A Grant is one project's role for one account.
type Grant struct {
	Project string
	Role    Role
}

// A Credential is one passkey.
type Credential struct {
	ID        []byte
	PublicKey []byte
	SignCount uint32
	Label     string
	Created   time.Time
	LastUsed  time.Time
}

// An Invitation is a one-time way to become an account with a role.
type Invitation struct {
	Email   string
	Project string
	Role    Role
	Expires time.Time
}

var (
	// ErrNoInvite covers every reason an invitation cannot be used, and says
	// nothing about which. A caller cannot tell a typo from an expiry from one
	// already spent, which is deliberate: the alternative is an oracle that
	// tells somebody guessing tokens when they are close.
	ErrNoInvite = errors.New("that invitation is not usable")
	// ErrNoAccount is returned when nothing is signed in, or the session has
	// expired, or the account behind it is disabled.
	ErrNoAccount = errors.New("not signed in")
	// ErrDisabled refuses an invitation to somebody who has been turned off.
	//
	// Named rather than folded into ErrNoInvite, because this one is told to an
	// owner who already knows the address exists. The reticence ErrNoInvite
	// buys is about strangers guessing tokens; there is nothing to protect from
	// somebody who is looking at the account in a list.
	ErrDisabled = errors.New("that account is disabled; enable it before inviting them again")
)

// token makes a secret and the hash it is stored under.
//
// Only the hash is written. This database is one backup away from being
// somewhere else, and a live invitation or session token in it is a way in that
// survives the file being copied.
func token() (secret, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	secret = base64.RawURLEncoding.EncodeToString(b)
	return secret, hashOf(secret), nil
}

func hashOf(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

const stamp = time.RFC3339

// Invite creates an invitation and returns the secret exactly once.
//
// The secret is never stored and cannot be recovered: an invitation that goes
// missing is reissued rather than looked up. That is the same posture as the
// session cookie and for the same reason.
func (s *Store) Invite(ctx context.Context, email, project string, role Role, by string) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	project = strings.TrimSpace(project)
	if email == "" || project == "" {
		return "", errors.New("an invitation needs an email and a project")
	}
	if !role.Valid() {
		return "", fmt.Errorf("%q is not a role", role)
	}
	// A disabled account cannot be re-admitted by invitation. Redeeming one
	// would spend the invitation, store a passkey, issue a cookie the guard
	// then refuses, and drop the person back at sign-in with no explanation —
	// which is what it did before a review followed it through. Enabling is a
	// deliberate act with its own control, and this says so rather than
	// performing it as a side effect of an invitation.
	var off string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(disabled, '') FROM account WHERE email = ?`, email).Scan(&off)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if off != "" {
		return "", ErrDisabled
	}
	secret, hash, err := token()
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO invite (token_hash, email, project, role, created, created_by, expires)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		hash, email, project, string(role), now.Format(stamp), by, now.Add(InviteLife).Format(stamp))
	if err != nil {
		return "", fmt.Errorf("write invitation: %w", err)
	}
	return secret, nil
}

// Invitation reads what a secret would grant, without spending it. The sign-in
// page uses it to say who is being invited to what before anything is created.
func (s *Store) Invitation(ctx context.Context, secret string) (Invitation, error) {
	var inv Invitation
	var role, expires, used string
	err := s.db.QueryRowContext(ctx,
		`SELECT email, project, role, expires, COALESCE(used, '') FROM invite WHERE token_hash = ?`,
		hashOf(secret)).Scan(&inv.Email, &inv.Project, &role, &expires, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return Invitation{}, ErrNoInvite
	}
	if err != nil {
		return Invitation{}, err
	}
	if used != "" {
		return Invitation{}, ErrNoInvite
	}
	inv.Role = Role(role)
	if inv.Expires, err = time.Parse(stamp, expires); err != nil {
		return Invitation{}, err
	}
	if s.now().UTC().After(inv.Expires) {
		return Invitation{}, ErrNoInvite
	}
	return inv, nil
}

// Redeem spends an invitation and returns the account it belongs to, creating
// it with newID if this is a first passkey and reusing the existing one — newID
// ignored — if the person already has an account.
//
// Reuse is what makes recovery work: an owner reissues an invitation to the
// same address, and the person registers a new passkey against the account they
// already had rather than a second account with the same email.
//
// Spending and creating happen in one transaction, so an invitation cannot be
// half-used by two requests arriving together.
func (s *Store) Redeem(ctx context.Context, secret, newID string) (Account, Invitation, error) {
	inv, err := s.Invitation(ctx, secret)
	if err != nil {
		return Account{}, Invitation{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Account{}, Invitation{}, err
	}
	defer tx.Rollback()

	now := s.now().UTC()
	res, err := tx.ExecContext(ctx,
		`UPDATE invite SET used = ? WHERE token_hash = ? AND used IS NULL`,
		now.Format(stamp), hashOf(secret))
	if err != nil {
		return Account{}, Invitation{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		// Somebody else spent it between the read and the write.
		return Account{}, Invitation{}, ErrNoInvite
	}

	var id, off string
	err = tx.QueryRowContext(ctx,
		`SELECT id, COALESCE(disabled, '') FROM account WHERE email = ?`, inv.Email).Scan(&id, &off)
	if err == nil && off != "" {
		// Refused before the invitation is spent. An invitation lives a day and
		// the account may have been disabled inside it, so the check at Invite
		// is not enough on its own. The rollback keeps the link usable, which
		// matters if the disabling is itself undone.
		return Account{}, Invitation{}, ErrDisabled
	}
	if errors.Is(err, sql.ErrNoRows) {
		// The caller may name the identifier, because a passkey ceremony
		// commits to a user handle before the account exists: the browser is
		// told who it is registering for at the start, and the account is only
		// created once the authenticator has answered.
		if id = newID; id == "" {
			if id, _, err = token(); err != nil {
				return Account{}, Invitation{}, err
			}
		}
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO account (id, email, created) VALUES (?, ?, ?)`,
			id, inv.Email, now.Format(stamp)); err != nil {
			return Account{}, Invitation{}, err
		}
	} else if err != nil {
		return Account{}, Invitation{}, err
	}

	// The invitation carries the role, so accepting it is not a second
	// decision. A role already held is replaced by the one just accepted.
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO grant_role (account_id, project, role, granted, granted_by)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (account_id, project) DO UPDATE SET
		   role = excluded.role, granted = excluded.granted, granted_by = excluded.granted_by`,
		id, inv.Project, string(inv.Role), now.Format(stamp), "invitation"); err != nil {
		return Account{}, Invitation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Account{}, Invitation{}, err
	}
	return Account{ID: id, Email: inv.Email, Created: now}, inv, nil
}

// AddCredential stores a passkey against an account.
func (s *Store) AddCredential(ctx context.Context, accountID string, c Credential) error {
	if len(c.ID) == 0 || len(c.PublicKey) == 0 {
		return errors.New("a credential needs an id and a public key")
	}
	label := strings.TrimSpace(c.Label)
	if label == "" {
		label = "passkey"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO credential (cred_id, account_id, public_key, sign_count, label, created)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, accountID, c.PublicKey, c.SignCount, label, s.now().UTC().Format(stamp))
	if err != nil {
		return fmt.Errorf("store credential: %w", err)
	}
	return nil
}

// Credentials lists an account's passkeys.
func (s *Store) Credentials(ctx context.Context, accountID string) ([]Credential, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT cred_id, public_key, sign_count, label, created, COALESCE(last_used, '')
		 FROM credential WHERE account_id = ? ORDER BY created`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Credential
	for rows.Next() {
		var c Credential
		var created, last string
		if err := rows.Scan(&c.ID, &c.PublicKey, &c.SignCount, &c.Label, &created, &last); err != nil {
			return nil, err
		}
		c.Created, _ = time.Parse(stamp, created)
		if last != "" {
			c.LastUsed, _ = time.Parse(stamp, last)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ByCredential finds the account a passkey belongs to, which is how a sign-in
// with a discoverable credential identifies somebody without their typing
// anything.
func (s *Store) ByCredential(ctx context.Context, credID []byte) (Account, Credential, error) {
	var a Account
	var c Credential
	var created, disabled, credCreated string
	err := s.db.QueryRowContext(ctx,
		`SELECT a.id, a.email, a.created, COALESCE(a.disabled, ''),
		        c.cred_id, c.public_key, c.sign_count, c.label, c.created
		 FROM credential c JOIN account a ON a.id = c.account_id
		 WHERE c.cred_id = ?`, credID).
		Scan(&a.ID, &a.Email, &created, &disabled, &c.ID, &c.PublicKey, &c.SignCount, &c.Label, &credCreated)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, Credential{}, ErrNoAccount
	}
	if err != nil {
		return Account{}, Credential{}, err
	}
	a.Created, _ = time.Parse(stamp, created)
	a.Disabled = disabled != ""
	c.Created, _ = time.Parse(stamp, credCreated)
	if a.Disabled {
		return Account{}, Credential{}, ErrNoAccount
	}
	return a, c, nil
}

// UsedCredential records a passkey's new signature counter.
//
// The counter is the one anti-cloning signal WebAuthn offers, and plenty of
// authenticators never increment it. It is stored so the check can be made
// where it means something, and its absence is not treated as an attack.
func (s *Store) UsedCredential(ctx context.Context, credID []byte, signCount uint32) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE credential SET sign_count = ?, last_used = ? WHERE cred_id = ?`,
		signCount, s.now().UTC().Format(stamp), credID)
	return err
}

// ErrLastPasskey refuses the removal that would lock somebody out of their own
// account. The owner chose passkeys knowing device loss was the risk; deleting
// the only one left is that risk arriving by the front door.
var ErrLastPasskey = errors.New("that is the only passkey on this account")

// RemoveCredential deletes one of an account's passkeys.
//
// The account id is part of the query rather than checked by the caller: a
// check the caller can skip is not a check, and removing somebody else's
// passkey is otherwise the same request with a different identifier.
func (s *Store) RemoveCredential(ctx context.Context, accountID string, credID []byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Whether this passkey is theirs, before whether it is their last. The
	// other order answered "that is your only passkey" to somebody removing a
	// passkey that was not theirs from an account holding one — a true sentence
	// about the wrong account, and a small oracle about somebody else's.
	var mine int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM credential WHERE account_id = ? AND cred_id = ?`,
		accountID, credID).Scan(&mine); err != nil {
		return err
	}
	if mine == 0 {
		return errors.New("no such passkey on this account")
	}
	var n int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM credential WHERE account_id = ?`, accountID).Scan(&n); err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastPasskey
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM credential WHERE account_id = ? AND cred_id = ?`, accountID, credID); err != nil {
		return err
	}
	return tx.Commit()
}

// Disable turns an account off, or back on.
//
// Not a delete. What the account did stays attributed to it, and a person who
// left and came back is the same person rather than a second one — which is the
// same reason a reissued invitation reuses an account.
func (s *Store) Disable(ctx context.Context, accountID string, undo bool) error {
	var when any
	if !undo {
		when = s.now().UTC().Format(stamp)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE account SET disabled = ? WHERE id = ?`, when, accountID)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows != 1 {
		return errors.New("no such account")
	}
	if !undo {
		// Sessions end with the account rather than lingering until they
		// expire: disabling somebody who is signed in should sign them out.
		_, _ = s.db.ExecContext(ctx, `DELETE FROM auth_session WHERE account_id = ?`, accountID)
	}
	return nil
}

// Grants lists what an account may do, by project.
func (s *Store) Grants(ctx context.Context, accountID string) ([]Grant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT project, role FROM grant_role WHERE account_id = ? ORDER BY project`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Grant
	for rows.Next() {
		var g Grant
		var role string
		if err := rows.Scan(&g.Project, &role); err != nil {
			return nil, err
		}
		g.Role = Role(role)
		out = append(out, g)
	}
	return out, rows.Err()
}

// RoleFor returns an account's role in one project, and false when it has none.
//
// No role is not the same as Reader. A project an account was never granted
// anything on is one it cannot see at all, which is what "a person may read one
// project and have no business in another" means.
func (s *Store) RoleFor(ctx context.Context, accountID, project string) (Role, bool) {
	var role string
	err := s.db.QueryRowContext(ctx,
		`SELECT role FROM grant_role WHERE account_id = ? AND project = ?`,
		accountID, project).Scan(&role)
	if err != nil {
		return "", false
	}
	r := Role(role)
	if !r.Valid() {
		return "", false
	}
	return r, true
}

// Grant sets a role directly, which is what the command line does when there is
// nobody yet to send an invitation to.
func (s *Store) Grant(ctx context.Context, accountID, project string, role Role, by string) error {
	if !role.Valid() {
		return fmt.Errorf("%q is not a role", role)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO grant_role (account_id, project, role, granted, granted_by)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (account_id, project) DO UPDATE SET
		   role = excluded.role, granted = excluded.granted, granted_by = excluded.granted_by`,
		accountID, project, string(role), s.now().UTC().Format(stamp), by)
	return err
}

// Accounts lists everybody Mustur knows.
func (s *Store) Accounts(ctx context.Context) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, created, COALESCE(disabled, '') FROM account ORDER BY created`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		var created, disabled string
		if err := rows.Scan(&a.ID, &a.Email, &created, &disabled); err != nil {
			return nil, err
		}
		a.Created, _ = time.Parse(stamp, created)
		a.Disabled = disabled != ""
		out = append(out, a)
	}
	return out, rows.Err()
}

// A Pending invitation is one issued and not yet used. The secret is not among
// its fields and cannot be: only the hash was written.
type Pending struct {
	Email   string
	Project string
	Role    Role
	Expires time.Time
}

// Pending lists invitations still open, so somebody who has just issued one is
// not told "nobody yet" and does not issue a second.
func (s *Store) Pending(ctx context.Context) ([]Pending, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT email, project, role, expires FROM invite
		 WHERE used IS NULL AND expires > ? ORDER BY created`,
		s.now().UTC().Format(stamp))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Pending
	for rows.Next() {
		var p Pending
		var role, expires string
		if err := rows.Scan(&p.Email, &p.Project, &role, &expires); err != nil {
			return nil, err
		}
		p.Role = Role(role)
		p.Expires, _ = time.Parse(stamp, expires)
		out = append(out, p)
	}
	return out, rows.Err()
}

// StartSession signs an account in and returns the cookie value once.
func (s *Store) StartSession(ctx context.Context, accountID string) (string, time.Time, error) {
	secret, hash, err := token()
	if err != nil {
		return "", time.Time{}, err
	}
	now := s.now().UTC()
	expires := now.Add(SessionLife)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO auth_session (token_hash, account_id, created, expires, last_seen)
		 VALUES (?, ?, ?, ?, ?)`,
		hash, accountID, now.Format(stamp), expires.Format(stamp), now.Format(stamp))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("start session: %w", err)
	}
	return secret, expires, nil
}

// Session resolves a cookie value to the account it signs in, refreshing its
// expiry so a browser in daily use never has to sign in again and one abandoned
// in a drawer does.
func (s *Store) Session(ctx context.Context, secret string) (Account, error) {
	if strings.TrimSpace(secret) == "" {
		return Account{}, ErrNoAccount
	}
	hash := hashOf(secret)
	var a Account
	var created, disabled, expires string
	err := s.db.QueryRowContext(ctx,
		`SELECT a.id, a.email, a.created, COALESCE(a.disabled, ''), s.expires
		 FROM auth_session s JOIN account a ON a.id = s.account_id
		 WHERE s.token_hash = ?`, hash).
		Scan(&a.ID, &a.Email, &created, &disabled, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNoAccount
	}
	if err != nil {
		return Account{}, err
	}
	end, err := time.Parse(stamp, expires)
	if err != nil {
		return Account{}, err
	}
	now := s.now().UTC()
	if now.After(end) {
		return Account{}, ErrNoAccount
	}
	if disabled != "" {
		return Account{}, ErrNoAccount
	}
	a.Created, _ = time.Parse(stamp, created)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE auth_session SET last_seen = ?, expires = ? WHERE token_hash = ?`,
		now.Format(stamp), now.Add(SessionLife).Format(stamp), hash); err != nil {
		return Account{}, err
	}
	return a, nil
}

// EndSession signs one browser out. Signing out elsewhere is a different verb
// nobody has asked for.
func (s *Store) EndSession(ctx context.Context, secret string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM auth_session WHERE token_hash = ?`, hashOf(secret))
	return err
}

// Empty reports whether Mustur knows anybody at all.
//
// The first-run case: with no accounts there is nobody to invite the first
// owner, so the command line does it, and the surfaces have to be able to tell
// "nobody has signed up yet" from "you are not signed in".
func (s *Store) Empty(ctx context.Context) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM account`).Scan(&n); err != nil {
		return false, err
	}
	return n == 0, nil
}
