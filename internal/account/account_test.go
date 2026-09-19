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

// An invitation to something that is not an address goes nowhere and says it
// went. `mustur account invite --email not-an-address` was accepted, printed a
// link, and listed a pending invitation nobody could ever accept (MUS-F-0059).
//
// The rule is the least that can be checked without refusing a real address, so
// the cases that must still pass are the point of this test as much as the
// cases that must not.
func TestAnInvitationNeedsAnAddressSomebodyCouldBeReachedAt(t *testing.T) {
	for _, ok := range []string{
		"a@b.com",
		"first.last+tag@sub.example.co.uk",
		"someone@localhost",
		"x@y",
		"UPPER@Example.COM",
	} {
		if !addressable(strings.ToLower(ok)) {
			t.Errorf("%q is a real address and was refused", ok)
		}
	}
	for _, bad := range []string{
		"not-an-address",
		"@b.com",
		"a@",
		"",
		"two@at@signs.com",
		"has a space@b.com",
	} {
		if addressable(bad) {
			t.Errorf("%q was accepted as an address", bad)
		}
	}

	// And through the door somebody actually walks in by. Invite trims and
	// lowercases first, so surrounding whitespace is not what is being
	// refused here — the thing between the spaces is.
	s, ctx := open(t)
	if _, err := s.Invite(ctx, "  not-an-address  ", "MUS", Reader, "test"); err == nil {
		t.Error("an invitation was issued to something nobody could receive")
	}
	if _, err := s.Invite(ctx, "  Someone@Example.com  ", "MUS", Reader, "test"); err != nil {
		t.Errorf("a real address surrounded by spaces was refused: %v", err)
	}
}

// A role can be taken away, and who took it is written down (MUS-F-0166,
// MUS-D-0188). Before Ungrant nothing removed a role once granted.
func TestUngrantRemovesARoleAndSaysWhoDidIt(t *testing.T) {
	s, ctx := open(t)
	redeemed(t, s, ctx, "owner@example.com", "LNK", Owner)
	reader := redeemed(t, s, ctx, "reader@example.com", "LNK", Reader)

	if err := s.Ungrant(ctx, reader.ID, "LNK", "owner@example.com"); err != nil {
		t.Fatal(err)
	}
	if role, ok := s.RoleFor(ctx, reader.ID, "LNK"); ok {
		t.Errorf("the role is still there: %q", role)
	}
	var by, role string
	if err := s.DB().QueryRowContext(ctx,
		`SELECT removed_by, role FROM grant_removed WHERE account_id = ? AND project = 'LNK'`,
		reader.ID).Scan(&by, &role); err != nil {
		t.Fatalf("the removal was not recorded: %v", err)
	}
	if by != "owner@example.com" || role != "reader" {
		t.Errorf("recorded %q removing %q", by, role)
	}
	// A role not held is an error, so a mistyped prefix does not pass as done.
	if err := s.Ungrant(ctx, reader.ID, "LNK", "owner@example.com"); err == nil {
		t.Error("removing a role nobody holds reported success")
	}
}

// The only owner of a project cannot be removed or demoted — in every project,
// not only the install's, which is the only one the surface used to check.
func TestTheLastOwnerOfEachProjectStays(t *testing.T) {
	s, ctx := open(t)
	a := redeemed(t, s, ctx, "a@example.com", "MUS", Owner)
	b := redeemed(t, s, ctx, "b@example.com", "LNK", Owner)
	// a owns HRD alone; b is MUS's second owner.
	if err := s.Grant(ctx, a.ID, "HRD", Owner, "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.Grant(ctx, b.ID, "MUS", Owner, "test"); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		who     Account
		project string
	}{{b, "LNK"}, {a, "HRD"}} {
		if err := s.Ungrant(ctx, c.who.ID, c.project, "test"); !errors.Is(err, ErrLastOwner) {
			t.Errorf("removing the only owner of %s: err = %v, want ErrLastOwner", c.project, err)
		}
		if err := s.Grant(ctx, c.who.ID, c.project, Reader, "test"); !errors.Is(err, ErrLastOwner) {
			t.Errorf("demoting the only owner of %s: err = %v, want ErrLastOwner", c.project, err)
		}
		if role, _ := s.RoleFor(ctx, c.who.ID, c.project); role != Owner {
			t.Errorf("the only owner of %s is now %q", c.project, role)
		}
	}

	// MUS has two owners, so one may stand down.
	if err := s.Grant(ctx, a.ID, "MUS", Reader, "test"); err != nil {
		t.Errorf("with a second owner, demotion was refused: %v", err)
	}
	// And now b is the only one, so b may not go.
	if err := s.Ungrant(ctx, b.ID, "MUS", "test"); !errors.Is(err, ErrLastOwner) {
		t.Errorf("removing the remaining owner of MUS: err = %v", err)
	}

	// A disabled owner is not the other owner: it cannot sign in to administer.
	c := redeemed(t, s, ctx, "c@example.com", "LNK", Owner)
	if err := s.Disable(ctx, c.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Ungrant(ctx, b.ID, "LNK", "test"); !errors.Is(err, ErrLastOwner) {
		t.Errorf("a disabled owner counted as the other owner of LNK: err = %v", err)
	}
}

func redeemed(t *testing.T, s *Store, ctx context.Context, email, project string, role Role) Account {
	t.Helper()
	secret, err := s.Invite(ctx, email, project, role, "test")
	if err != nil {
		t.Fatal(err)
	}
	acct, _, err := s.Redeem(ctx, secret, "")
	if err != nil {
		t.Fatal(err)
	}
	return acct
}

// A store written before grant_removed existed gains it on opening, so the
// first removal on the live store is recorded rather than failing (MUS-D-0188,
// the review on PR 102). Modelled on PR 103's test for held_jot.
func TestAnOlderStoreGainsTheRemovalTable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `DROP TABLE grant_removed`); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = st.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE name = 'grant_removed'`).Scan(&n)
	st.Close()
	if n != 0 {
		t.Fatal("the table was not dropped, so this tests nothing")
	}

	st, err = store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := New(st.DB())
	redeemed(t, s, ctx, "owner@example.com", "MUS", Owner)
	reader := redeemed(t, s, ctx, "reader@example.com", "MUS", Reader)
	if err := s.Ungrant(ctx, reader.ID, "MUS", "owner@example.com"); err != nil {
		t.Fatalf("an older store could not record a removal after opening: %v", err)
	}
	if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM grant_removed`).Scan(&n); err != nil || n != 1 {
		t.Errorf("grant_removed holds %d rows after one removal: %v", n, err)
	}
}

// An invitation never leaves a project with no owner (MUS-D-0188, the review on
// PR 102): one that would demote the only owner is refused when it is issued,
// and again when it is accepted, for an owner who became the only one after it
// was issued. The refusal leaves the invitation unspent, as ErrDisabled does.
func TestAnInvitationCannotDemoteTheOnlyOwner(t *testing.T) {
	s, ctx := open(t)
	solo := redeemed(t, s, ctx, "solo@example.com", "LNK", Owner)
	other := redeemed(t, s, ctx, "other@example.com", "LNK", Owner)

	// Issued while there are two owners, so Invite lets it through.
	secret, err := s.Invite(ctx, "solo@example.com", "LNK", Reader, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Ungrant(ctx, other.ID, "LNK", "test"); err != nil {
		t.Fatal(err)
	}

	_, _, err = s.Redeem(ctx, secret, "")
	var last *LastOwnerError
	if !errors.As(err, &last) || last.Project != "LNK" {
		t.Errorf("accepting a reader invitation as the only owner: err = %v, want a LastOwnerError naming LNK", err)
	}
	if role, _ := s.RoleFor(ctx, solo.ID, "LNK"); role != Owner {
		t.Errorf("the only owner of LNK is now %q", role)
	}
	if _, err := s.Invitation(ctx, secret); err != nil {
		t.Errorf("the refused invitation was spent: %v", err)
	}

	// Issuing one now is refused at once.
	if _, err := s.Invite(ctx, "solo@example.com", "LNK", Reader, "test"); !errors.As(err, &last) || last.Project != "LNK" {
		t.Errorf("inviting the only owner of LNK as a reader: err = %v", err)
	}
	// An owner invitation is still how a lost passkey is recovered.
	if _, _, err := s.Redeem(ctx, mustInvite(t, s, ctx, "solo@example.com", "LNK", Owner), ""); err != nil {
		t.Errorf("an owner invitation to the only owner was refused: %v", err)
	}
	// With a second owner, accepting a reader invitation demotes, as before.
	if err := s.Grant(ctx, other.ID, "LNK", Owner, "test"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Redeem(ctx, secret, ""); err != nil {
		t.Fatalf("with a second owner, the reader invitation was refused: %v", err)
	}
	if role, _ := s.RoleFor(ctx, solo.ID, "LNK"); role != Reader {
		t.Errorf("the accepted reader invitation left the role at %q", role)
	}
}

func mustInvite(t *testing.T, s *Store, ctx context.Context, email, project string, role Role) string {
	t.Helper()
	secret, err := s.Invite(ctx, email, project, role, "test")
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

// Disabling the only enabled owner of any project is refused, and names the
// project: a disabled owner cannot sign in, so it is the same lockout as
// removing their role (MUS-D-0188, the review on PR 102).
func TestDisablingTheOnlyOwnerOfAnyProjectIsRefused(t *testing.T) {
	s, ctx := open(t)
	mus := redeemed(t, s, ctx, "mus@example.com", "MUS", Owner)
	lnk := redeemed(t, s, ctx, "lnk@example.com", "LNK", Owner)
	disabled := func(a Account) bool {
		var off string
		_ = s.DB().QueryRowContext(ctx,
			`SELECT COALESCE(disabled, '') FROM account WHERE id = ?`, a.ID).Scan(&off)
		return off != ""
	}

	// Somebody else's only project: an owner of MUS disabling LNK's only owner.
	err := s.Disable(ctx, lnk.ID, false)
	var last *LastOwnerError
	if !errors.As(err, &last) || last.Project != "LNK" || !errors.Is(err, ErrLastOwner) {
		t.Errorf("disabling LNK's only owner: err = %v, want a LastOwnerError naming LNK", err)
	}
	if disabled(lnk) {
		t.Error("LNK's only owner was disabled")
	}

	// Yourself, as the only owner of a project that is not the first you own.
	if err := s.Grant(ctx, lnk.ID, "MUS", Owner, "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.Grant(ctx, mus.ID, "HRD", Owner, "test"); err != nil {
		t.Fatal(err)
	}
	err = s.Disable(ctx, mus.ID, false)
	if !errors.As(err, &last) || last.Project != "HRD" {
		t.Errorf("disabling HRD's only owner: err = %v, want a LastOwnerError naming HRD", err)
	}
	if disabled(mus) {
		t.Error("HRD's only owner was disabled")
	}

	// With another enabled owner of every project it owns, it goes.
	if err := s.Grant(ctx, lnk.ID, "HRD", Owner, "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.Disable(ctx, mus.ID, false); err != nil {
		t.Errorf("with a second owner of MUS and HRD, disabling was refused: %v", err)
	}
	if !disabled(mus) {
		t.Error("the disable reported success and did nothing")
	}
	// And lnk is now every project's only enabled owner, so it stays.
	if err := s.Disable(ctx, lnk.ID, false); !errors.Is(err, ErrLastOwner) {
		t.Errorf("the one owner left of everything was disabled: err = %v", err)
	}
	// Enabling is never refused.
	if err := s.Disable(ctx, mus.ID, true); err != nil || disabled(mus) {
		t.Errorf("enabling again: err = %v", err)
	}
}

// A project nobody owns goes to the install's owners; one somebody owns stays
// theirs, and adopting again changes nothing.
func TestAdoptGivesOnlyUnownedProjects(t *testing.T) {
	s, ctx := open(t)
	a := redeemed(t, s, ctx, "a@example.com", "MUS", Owner)
	b := redeemed(t, s, ctx, "b@example.com", "LNK", Owner)
	if err := s.Grant(ctx, a.ID, "HRD", Reader, "test"); err != nil {
		t.Fatal(err)
	}

	gave, err := s.Adopt(ctx, "MUS", []string{"MUS", "LNK", "HRD", "IDW"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(gave, ",") != "HRD,IDW" {
		t.Errorf("adopted %v, want HRD and IDW", gave)
	}
	for _, p := range []string{"HRD", "IDW"} {
		if role, _ := s.RoleFor(ctx, a.ID, p); role != Owner {
			t.Errorf("the install's owner is %q on %s", role, p)
		}
	}
	if _, ok := s.RoleFor(ctx, a.ID, "LNK"); ok {
		t.Error("a project with an owner was adopted")
	}
	if role, _ := s.RoleFor(ctx, b.ID, "LNK"); role != Owner {
		t.Errorf("LinkCtrl's owner is now %q", role)
	}
	if again, err := s.Adopt(ctx, "MUS", []string{"HRD", "IDW"}, "test"); err != nil || len(again) != 0 {
		t.Errorf("adopting again gave %v, %v", again, err)
	}
}
