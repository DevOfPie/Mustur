package account

// An agent's credential, which is not a person's.
//
// Milestone 5b built a gate and then could not turn it on. `--accounts` wraps
// the whole mux, `/mcp` is on that mux, and an MCP call is a POST — so with
// enforcement on, the mandated tool call answered 403 to every agent, measured
// rather than reasoned. An agent holds no passkey: WebAuthn needs a browser, an
// authenticator and a gesture, and an agent has none of the three.
//
// The owner chose a token over exempting `/mcp` inside the guard
// (MUS-Q-0051). The cheaper answer was to let the edge carry that one path,
// since Cloudflare Access is in front; the answer taken was the one that lets
// Mustur stand on its own, because leaning on the edge is precisely what
// standing alone means not doing.
//
// **A token is not an account and cannot become one.** It has no email, holds
// no passkey, and the guard consults it on exactly one path. That is not
// tidiness: a token is a long-lived secret sitting in a process's environment
// or a systemd unit, which is a weaker place than a device's secure element,
// and it should not be able to open a browser surface a person's credential
// opens. Scope is what makes the weaker secret acceptable.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// tokenPrefix marks a Mustur agent token wherever one ends up.
const tokenPrefix = "mus_"

// A Token is what an agent carries. The secret itself is never in here — it is
// returned once by Issue and then exists only in the caller's hands.
type Token struct {
	ID      string
	Label   string
	Project string
	Role    Role
	Created time.Time
	// Revoked is zero while the token works.
	Revoked time.Time
	// Expires is zero when the token never expires, which is the default.
	Expires  time.Time
	LastUsed time.Time
}

// Live reports whether this token would still be accepted, as of now.
func (t Token) Live() bool { return t.LiveAt(time.Now()) }

// LiveAt is Live against a given clock, so a listing and the guard agree.
func (t Token) LiveAt(now time.Time) bool {
	if !t.Revoked.IsZero() {
		return false
	}
	return t.Expires.IsZero() || now.Before(t.Expires)
}

var (
	// ErrNoToken covers every reason a token is not usable and says nothing
	// about which — unknown, revoked, malformed. Same reticence as ErrNoInvite
	// and for the same reason: the alternative tells somebody guessing when
	// they are close.
	ErrNoToken = errors.New("that token is not usable")
	// ErrNoSuchToken is for an operator naming one by id at the command line,
	// where there is nothing to protect and a precise answer is a kindness.
	ErrNoSuchToken = errors.New("no token with that id")
)

// IssueToken mints one and returns the secret exactly once.
//
// life is how long it lasts, and **zero means forever**, which is the default.
// An invitation expires because it is a one-time link in transit; a session
// expires because a browser is borrowed. An agent token is configuration, and a
// credential that stops working at 3am with nobody having decided so is an
// outage rather than a control — so the ordinary token has no expiry and
// revocation is the stop.
//
// A lifetime is still worth having for the cases that want one: an agent
// brought up for a single job, or a token going onto somebody else's machine.
// The owner settled that shape on MUS-Q-0055, against a builder who had argued
// for no lifetime at all and written the argument down instead of asking.
func (s *Store) IssueToken(ctx context.Context, label, project string, role Role, by string, life time.Duration) (secret string, t Token, err error) {
	label = strings.TrimSpace(label)
	project = strings.TrimSpace(project)
	if label == "" || project == "" {
		return "", Token{}, errors.New("a token needs a label and a project")
	}
	if !role.Valid() {
		return "", Token{}, fmt.Errorf("%q is not a role", role)
	}
	secret, hash, err := token()
	if err != nil {
		return "", Token{}, err
	}
	// Hex rather than base64url. An id is typed at a shell — "mustur account
	// revoke <id>" — and a base64url id starting with "-" is parsed as a flag
	// and never reaches the argument, so the tool would print a revoke command
	// that cannot work.
	//
	// About one id in 64: the alphabet is 64 symbols and "-" is one of them.
	// Measured over 200,000 draws, 3,072 began with "-", or 1 in 65.1. An
	// earlier version of this comment said one in thirty-two, which was a
	// number nobody had counted.
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return "", Token{}, err
	}
	id := hex.EncodeToString(raw)
	if life < 0 {
		return "", Token{}, errors.New("a token cannot expire in the past")
	}
	now := s.now().UTC()
	var expires any
	var expiresAt time.Time
	if life > 0 {
		expiresAt = now.Add(life)
		expires = expiresAt.Format(stamp)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_token (id, token_hash, label, project, role, created, created_by, expires)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, hash, label, project, string(role), now.Format(stamp), by, expires); err != nil {
		return "", Token{}, fmt.Errorf("write token: %w", err)
	}
	// The secret is prefixed so an operator who finds one in a log or a unit
	// file knows what they are looking at, and so does a scanner. Required on
	// presentation, not merely tolerated — see ByToken.
	return tokenPrefix + secret, Token{
		ID: id, Label: label, Project: project, Role: role,
		Created: now, Expires: expiresAt,
	}, nil
}

// ByToken resolves a secret an agent presented.
//
// A revoked token is refused here rather than filtered by the caller, because
// a check the caller can forget is not a check.
func (s *Store) ByToken(ctx context.Context, secret string) (Token, error) {
	secret = strings.TrimSpace(secret)
	// The prefix is required rather than trimmed if present. It exists so a
	// secret found in a log or a unit file is identifiable — by a person and by
	// a scanner — and a prefix the server does not insist on is one an agent
	// can be configured without, at which point it identifies nothing.
	rest, ok := strings.CutPrefix(secret, tokenPrefix)
	if !ok || rest == "" {
		return Token{}, ErrNoToken
	}
	secret = rest

	var t Token
	var role, created, revoked, expires, lastUsed string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, label, project, role, created,
		        COALESCE(revoked, ''), COALESCE(expires, ''), COALESCE(last_used, '')
		   FROM agent_token WHERE token_hash = ?`,
		hashOf(secret)).Scan(&t.ID, &t.Label, &t.Project, &role, &created, &revoked, &expires, &lastUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return Token{}, ErrNoToken
	}
	if err != nil {
		return Token{}, err
	}
	t.Role = Role(role)
	t.Created, _ = time.Parse(stamp, created)
	t.Revoked, _ = time.Parse(stamp, revoked)
	t.Expires, _ = time.Parse(stamp, expires)
	t.LastUsed, _ = time.Parse(stamp, lastUsed)
	// Revoked and expired are one answer here, deliberately: the caller learns
	// that the token is not usable and not which of the two, for the same
	// reason ErrNoInvite says nothing about why.
	if !t.LiveAt(s.now()) {
		return Token{}, ErrNoToken
	}
	return t, nil
}

// UsedToken records that a token was accepted.
//
// Best effort and deliberately not in the accept path's error handling: a
// failure to write "last used" is not a reason to refuse a valid credential.
// It exists so `mustur account tokens` can show which of several is the live
// one, which is what makes revoking the other two safe to do.
func (s *Store) UsedToken(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE agent_token SET last_used = ? WHERE id = ?`,
		s.now().UTC().Format(stamp), id)
	return err
}

// RevokeToken stops one immediately.
//
// The only stop there is. A token belongs to no account, so disabling or
// removing a person does not touch tokens they minted — deliberate, since an
// agent's credential outliving the person who set it up is usually what you
// want, but it means offboarding somebody means reading `mustur account tokens`
// as well as `mustur account list`.
//
// A timestamp rather than a delete, so a listing can still say the token
// existed and when it stopped — which is what somebody investigating wants,
// and a row that vanished cannot tell them.
func (s *Store) RevokeToken(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE agent_token SET revoked = ? WHERE id = ? AND revoked IS NULL`,
		s.now().UTC().Format(stamp), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		// Either there is no such token, or it was already revoked. Both mean
		// the same to the operator: it is not working now.
		return ErrNoSuchToken
	}
	return nil
}

// Tokens lists every token, revoked ones included.
func (s *Store) Tokens(ctx context.Context) ([]Token, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, label, project, role, created,
		        COALESCE(revoked, ''), COALESCE(expires, ''), COALESCE(last_used, '')
		   FROM agent_token ORDER BY created`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Token
	for rows.Next() {
		var t Token
		var role, created, revoked, expires, lastUsed string
		if err := rows.Scan(&t.ID, &t.Label, &t.Project, &role, &created, &revoked, &expires, &lastUsed); err != nil {
			return nil, err
		}
		t.Role = Role(role)
		t.Created, _ = time.Parse(stamp, created)
		t.Revoked, _ = time.Parse(stamp, revoked)
		t.Expires, _ = time.Parse(stamp, expires)
		t.LastUsed, _ = time.Parse(stamp, lastUsed)
		out = append(out, t)
	}
	return out, rows.Err()
}
