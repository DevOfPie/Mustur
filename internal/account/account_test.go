package account

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/store"
)

func open(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st.DB()), ctx
}

// The ordinary path: invited, redeemed, a passkey stored, signed in, recognised.
func TestAnInvitationBecomesAnAccountThatCanSignIn(t *testing.T) {
	s, ctx := open(t)

	secret, err := s.Invite(ctx, "Reader@Example.com", "MUS", Reader, "pie")
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" {
		t.Fatal("no invitation secret")
	}

	inv, err := s.Invitation(ctx, secret)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Email != "reader@example.com" {
		t.Errorf("email %q; an address is stored lower-cased so one person is one account", inv.Email)
	}
	if inv.Role != Reader || inv.Project != "MUS" {
		t.Errorf("the invitation does not carry its role and project: %+v", inv)
	}

	acct, _, err := s.Redeem(ctx, secret, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddCredential(ctx, acct.ID, Credential{
		ID: []byte("cred-1"), PublicKey: []byte("key-1"), Label: "phone",
	}); err != nil {
		t.Fatal(err)
	}

	// A passkey identifies its account with nothing typed.
	found, cred, err := s.ByCredential(ctx, []byte("cred-1"))
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != acct.ID || cred.Label != "phone" {
		t.Errorf("the passkey did not identify its account: %+v %+v", found, cred)
	}

	cookie, expires, err := s.StartSession(ctx, acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !expires.After(time.Now()) {
		t.Error("the session expires in the past")
	}
	back, err := s.Session(ctx, cookie)
	if err != nil || back.ID != acct.ID {
		t.Fatalf("the session did not resolve: %v %+v", err, back)
	}

	if role, ok := s.RoleFor(ctx, acct.ID, "MUS"); !ok || role != Reader {
		t.Errorf("role %q ok=%v, want reader", role, ok)
	}
	// A project it was never granted anything on is not readable either.
	if _, ok := s.RoleFor(ctx, acct.ID, "IDW"); ok {
		t.Error("an account has a role on a project it was never granted")
	}
}

// An invitation is one use, and every reason it cannot be used looks the same
// from outside.
func TestAnInvitationIsSpentOnce(t *testing.T) {
	s, ctx := open(t)
	secret, err := s.Invite(ctx, "one@example.com", "MUS", Owner, "pie")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Redeem(ctx, secret, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Redeem(ctx, secret, ""); err != ErrNoInvite {
		t.Errorf("a spent invitation was redeemed again: %v", err)
	}
	if _, err := s.Invitation(ctx, secret); err != ErrNoInvite {
		t.Errorf("a spent invitation still reads as usable: %v", err)
	}
	if _, err := s.Invitation(ctx, "not-a-real-token"); err != ErrNoInvite {
		t.Errorf("an unknown token gave %v; every failure must look the same", err)
	}
}

// Expiry, from the outside, is indistinguishable from a bad token.
func TestAnExpiredInvitationIsRefused(t *testing.T) {
	s, ctx := open(t)
	secret, err := s.Invite(ctx, "late@example.com", "MUS", Reader, "pie")
	if err != nil {
		t.Fatal(err)
	}
	later := s.WithClock(func() time.Time { return time.Now().Add(InviteLife + time.Minute) })
	if _, err := later.Invitation(ctx, secret); err != ErrNoInvite {
		t.Errorf("an expired invitation was accepted: %v", err)
	}
	if _, _, err := later.Redeem(ctx, secret, ""); err != ErrNoInvite {
		t.Errorf("an expired invitation was redeemed: %v", err)
	}
}

// The recovery the owner asked for: a device is lost, a new invitation is
// issued to the same address, and the person keeps the account they had.
//
// If this created a second account the old passkeys, the roles and everything
// attributed to them would be somebody else's.
func TestRecoveryReusesTheAccountRatherThanMakingASecond(t *testing.T) {
	s, ctx := open(t)

	first, err := s.Invite(ctx, "owner@example.com", "MUS", Owner, "pie")
	if err != nil {
		t.Fatal(err)
	}
	acct, _, err := s.Redeem(ctx, first, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddCredential(ctx, acct.ID, Credential{
		ID: []byte("old-phone"), PublicKey: []byte("k"), Label: "old phone",
	}); err != nil {
		t.Fatal(err)
	}

	// The phone is gone. Somebody with an owner role reissues.
	second, err := s.Invite(ctx, "OWNER@example.com", "MUS", Owner, "pie")
	if err != nil {
		t.Fatal(err)
	}
	again, _, err := s.Redeem(ctx, second, "")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != acct.ID {
		t.Fatalf("recovery made a second account: %s then %s", acct.ID, again.ID)
	}
	if err := s.AddCredential(ctx, again.ID, Credential{
		ID: []byte("new-phone"), PublicKey: []byte("k2"), Label: "new phone",
	}); err != nil {
		t.Fatal(err)
	}

	creds, err := s.Credentials(ctx, acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 2 {
		t.Fatalf("%d passkeys, want the old one and the new one", len(creds))
	}
	accounts, err := s.Accounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 {
		t.Errorf("%d accounts for one person", len(accounts))
	}
}

// A role that only reads cannot reach anything that writes. One predicate, so a
// surface added later cannot forget to be on a list.
func TestOnlyAnOwnerCanWrite(t *testing.T) {
	if !Owner.CanWrite() {
		t.Error("an owner cannot write")
	}
	if Reader.CanWrite() {
		t.Error("a reader can write, which is the whole thing this exists to prevent")
	}
	if Role("admin").Valid() || Role("").Valid() {
		t.Error("a role read from a hand-edited database is trusted")
	}
	if Role("admin").CanWrite() {
		t.Error("an unknown role can write")
	}
}

// Sessions end, and a disabled account's session stops working immediately
// rather than at its next expiry.
func TestASessionEndsAndADisabledAccountIsRefused(t *testing.T) {
	s, ctx := open(t)
	secret, err := s.Invite(ctx, "gone@example.com", "MUS", Owner, "pie")
	if err != nil {
		t.Fatal(err)
	}
	acct, _, err := s.Redeem(ctx, secret, "")
	if err != nil {
		t.Fatal(err)
	}
	cookie, _, err := s.StartSession(ctx, acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Session(ctx, cookie); err != nil {
		t.Fatal(err)
	}
	if err := s.EndSession(ctx, cookie); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Session(ctx, cookie); err != ErrNoAccount {
		t.Errorf("a signed-out cookie still works: %v", err)
	}

	cookie2, _, err := s.StartSession(ctx, acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(ctx,
		`UPDATE account SET disabled = ? WHERE id = ?`, "2026-08-24T00:00:00Z", acct.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Session(ctx, cookie2); err != ErrNoAccount {
		t.Errorf("a disabled account's live session still works: %v", err)
	}
}

// An empty store is distinguishable from a stranger, because the first owner
// has to come from somewhere.
func TestEmptySaysWhenNobodyExists(t *testing.T) {
	s, ctx := open(t)
	empty, err := s.Empty(ctx)
	if err != nil || !empty {
		t.Fatalf("a new store is not empty: %v %v", empty, err)
	}
	secret, err := s.Invite(ctx, "first@example.com", "MUS", Owner, "cli")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Redeem(ctx, secret, ""); err != nil {
		t.Fatal(err)
	}
	if empty, err := s.Empty(ctx); err != nil || empty {
		t.Errorf("a store with an account reads as empty: %v %v", empty, err)
	}
}

// Only the hash is stored. This file is one backup away from being somewhere
// else, and a live token in it is a way in that survives being copied.
func TestSecretsAreNotStored(t *testing.T) {
	s, ctx := open(t)
	secret, err := s.Invite(ctx, "hash@example.com", "MUS", Reader, "pie")
	if err != nil {
		t.Fatal(err)
	}
	acct, _, err := s.Redeem(ctx, secret, "")
	if err != nil {
		t.Fatal(err)
	}
	cookie, _, err := s.StartSession(ctx, acct.ID)
	if err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{"invite", "auth_session"} {
		rows, err := s.DB().QueryContext(ctx, `SELECT token_hash FROM `+table)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var stored string
			if err := rows.Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if stored == secret || stored == cookie {
				t.Errorf("%s holds a live secret rather than its hash", table)
			}
			if strings.Contains(stored, secret) || strings.Contains(stored, cookie) {
				t.Errorf("%s holds something containing a live secret", table)
			}
		}
		rows.Close()
	}
}

// enrolled invites somebody and redeems it, which is the shortest way to an
// account that exists.
func enrolled(t *testing.T, s *Store, ctx context.Context, email string, role Role) Account {
	t.Helper()
	secret, err := s.Invite(ctx, email, "MUS", role, "test")
	if err != nil {
		t.Fatal(err)
	}
	acct, _, err := s.Redeem(ctx, secret, "")
	if err != nil {
		t.Fatal(err)
	}
	return acct
}

// A disabled account is not re-admitted by an invitation.
//
// Found by following what happened rather than by reading it: the invitation
// was spent, a passkey stored, a cookie issued and then refused, and the person
// landed back at sign-in knowing nothing. The invitation was gone.
func TestADisabledAccountCannotBeInvitedBackIn(t *testing.T) {
	s, ctx := open(t)
	acct := enrolled(t, s, ctx, "gone@example.com", Reader)
	if err := s.Disable(ctx, acct.ID, false); err != nil {
		t.Fatal(err)
	}

	// The owner cannot issue one at all.
	if _, err := s.Invite(ctx, "gone@example.com", "MUS", Reader, "test"); !errors.Is(err, ErrDisabled) {
		t.Errorf("inviting a disabled account returned %v, want ErrDisabled", err)
	}

	// One issued before the disabling is refused without being spent, so
	// undoing the disabling leaves it usable.
	other := enrolled(t, s, ctx, "early@example.com", Reader)
	second, err := s.Invite(ctx, "early@example.com", "MUS", Owner, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Disable(ctx, other.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Redeem(ctx, second, ""); !errors.Is(err, ErrDisabled) {
		t.Errorf("redeeming onto a disabled account returned %v, want ErrDisabled", err)
	}
	if err := s.Disable(ctx, other.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Redeem(ctx, second, ""); err != nil {
		t.Errorf("the refused invitation had been spent anyway: %v", err)
	}
}

// Removing a passkey answers about the passkey named, not about the account.
//
// The two checks were the other way round, so removing somebody else's passkey
// from an account holding one was told "that is your only passkey" — true about
// an account that was not theirs, and a small oracle about somebody else's.
func TestRemovingSomebodyElsesPasskeySaysSo(t *testing.T) {
	s, ctx := open(t)
	mine := enrolled(t, s, ctx, "mine@example.com", Owner)
	theirs := enrolled(t, s, ctx, "theirs@example.com", Reader)

	for _, id := range []string{"mine-a", "mine-b"} {
		if err := s.AddCredential(ctx, mine.ID, Credential{ID: []byte(id), PublicKey: []byte("k")}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddCredential(ctx, theirs.ID, Credential{ID: []byte("theirs-only"), PublicKey: []byte("k")}); err != nil {
		t.Fatal(err)
	}

	err := s.RemoveCredential(ctx, mine.ID, []byte("theirs-only"))
	if err == nil {
		t.Fatal("removed a passkey belonging to another account")
	}
	if errors.Is(err, ErrLastPasskey) {
		t.Error("answered about the other account's passkey count instead of saying this one is not theirs")
	}
	creds, err := s.Credentials(ctx, theirs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 1 {
		t.Errorf("%d passkeys left on the other account", len(creds))
	}
}

// A role cannot be granted to an account that does not exist.
//
// A review reported this as broken on the grounds that nothing switches foreign
// keys on. Measured: modernc's driver does, and the constraint fires with no
// pragma in the tree at all. The test stays because the guarantee is worth
// pinning wherever it comes from — the finding does not.
func TestARoleNeedsAnAccountThatExists(t *testing.T) {
	s, ctx := open(t)
	if err := s.Grant(ctx, "no-such-account", "MUS", Owner, "test"); err == nil {
		t.Error("granted a role to an account id that does not exist")
	}
}
