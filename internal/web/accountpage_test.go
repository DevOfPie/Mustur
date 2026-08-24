package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/store"
)

// managed builds the account surface behind the guard, which is how it is
// actually served: the exemption for /account is part of what is under test.
func managed(t *testing.T) (*httptest.Server, *account.Store) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	accounts := account.New(st.DB())

	mux := http.NewServeMux()
	auth := &Auth{Accounts: accounts, Origin: "http://127.0.0.1"}
	auth.Routes(mux)
	manage := &Accounts{Store: accounts, Auth: auth, Project: "MUS"}
	manage.Routes(mux)
	mux.HandleFunc("/records", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("records"))
	})
	guard := &Guard{Auth: auth, Project: "MUS"}
	srv := httptest.NewServer(guard.Wrap(mux))
	t.Cleanup(srv.Close)
	return srv, accounts
}

// person makes an account with one passkey and returns a client holding its
// session, plus the account itself.
func personWith(t *testing.T, accounts *account.Store, email, project string, role account.Role, passkeys ...string) (*http.Client, account.Account) {
	t.Helper()
	ctx := context.Background()
	secret, err := accounts.Invite(ctx, email, project, role, "test")
	if err != nil {
		t.Fatal(err)
	}
	acct, _, err := accounts.Redeem(ctx, secret, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range passkeys {
		if err := accounts.AddCredential(ctx, acct.ID, account.Credential{
			ID: []byte(k), PublicKey: []byte("pk-" + k), Label: k,
		}); err != nil {
			t.Fatal(err)
		}
	}
	cookie, _, err := accounts.StartSession(ctx, acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     cookieAdder{cookie: &http.Cookie{Name: SessionCookie, Value: cookie}},
	}, acct
}

func body(t *testing.T, c *http.Client, url string) string {
	t.Helper()
	res, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b := make([]byte, 32768)
	n, _ := res.Body.Read(b)
	return string(b[:n])
}

func form(t *testing.T, c *http.Client, srv *httptest.Server, path string, v url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(v.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", srv.URL)
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// A reader manages their own passkeys. The guard refuses readers every other
// write, and this is the one place that would lock somebody out of an account
// they are entitled to.
func TestAReaderCanManageTheirOwnPasskeys(t *testing.T) {
	srv, accounts := managed(t)
	reader, _ := personWith(t, accounts, "reader@example.com", "MUS", account.Reader, "phone", "laptop")

	page := body(t, reader, srv.URL+"/account")
	for _, want := range []string{"reader@example.com", "phone", "laptop", "Add a passkey"} {
		if !strings.Contains(page, want) {
			t.Errorf("the account page does not show %q", want)
		}
	}
	// And none of the owner-only machinery.
	for _, unwanted := range []string{"Invite somebody", "People"} {
		if strings.Contains(page, unwanted) {
			t.Errorf("a reader is shown %q", unwanted)
		}
	}

	res := form(t, reader, srv, "/account/passkey/remove", url.Values{"id": {url.QueryEscape("phone")}})
	res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("a reader could not remove their own passkey: %d", res.StatusCode)
	}
	if got := body(t, reader, srv.URL+"/account"); strings.Contains(got, ">phone<") {
		t.Error("the removed passkey is still listed")
	}
}

// The refusal that keeps somebody in their own account.
func TestTheLastPasskeyCannotBeRemoved(t *testing.T) {
	srv, accounts := managed(t)
	only, _ := personWith(t, accounts, "one@example.com", "MUS", account.Owner, "only-key")

	res := form(t, only, srv, "/account/passkey/remove", url.Values{"id": {url.QueryEscape("only-key")}})
	res.Body.Close()
	// The refusal travels in the redirect, so the page has to be fetched where
	// it points rather than at a bare /account.
	page := body(t, only, srv.URL+res.Header.Get("Location"))
	if !strings.Contains(page, "only passkey") {
		t.Error("removing the last passkey was allowed, or said nothing about why not")
	}
	if !strings.Contains(page, "only-key") {
		t.Error("the last passkey was removed")
	}
}

// Somebody else's passkey is the same request with a different identifier, so
// the account is part of the query rather than a check the caller makes.
func TestOnePersonCannotRemoveAnothersPasskey(t *testing.T) {
	srv, accounts := managed(t)
	mine, _ := personWith(t, accounts, "mine@example.com", "MUS", account.Owner, "mine-a", "mine-b")
	_, _ = personWith(t, accounts, "theirs@example.com", "MUS", account.Reader, "theirs-a", "theirs-b")

	res := form(t, mine, srv, "/account/passkey/remove", url.Values{"id": {url.QueryEscape("theirs-a")}})
	res.Body.Close()

	ctx := context.Background()
	all, err := accounts.Accounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, p := range all {
		if p.Email != "theirs@example.com" {
			continue
		}
		found = true
		keys, err := accounts.Credentials(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(keys) != 2 {
			t.Errorf("%d passkeys left on the other account; one was removed by somebody else", len(keys))
		}
	}
	if !found {
		t.Fatal("the other account is missing, so this proved nothing")
	}
}

// An owner sees and can use the machinery a reader does not.
func TestAnOwnerInvitesAndSeesPeople(t *testing.T) {
	srv, accounts := managed(t)
	owner, _ := personWith(t, accounts, "owner@example.com", "MUS", account.Owner, "k")

	page := body(t, owner, srv.URL+"/account")
	for _, want := range []string{"Invite somebody", "People", "owner@example.com"} {
		if !strings.Contains(page, want) {
			t.Errorf("an owner is not shown %q", want)
		}
	}

	res := form(t, owner, srv, "/account/invite", url.Values{
		"email": {"new@example.com"}, "role": {"reader"}, "project": {"MUS"},
	})
	res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("inviting returned %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "invited=") {
		t.Fatalf("the invitation link was not carried back: %q", loc)
	}
	// Shown once, and the page says so, because the secret is not stored and
	// somebody who closes the tab will otherwise go looking for it.
	shown := body(t, owner, srv.URL+loc)
	if !strings.Contains(shown, "shown once") {
		t.Error("the page does not say the link cannot be shown again")
	}
	pending, err := accounts.Pending(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Email != "new@example.com" {
		t.Errorf("the invitation was not recorded: %+v", pending)
	}
}

// A reader cannot reach the owner-only endpoints even by posting to them
// directly, which is what somebody who reads the HTML would try.
func TestAReaderCannotInviteOrChangeRoles(t *testing.T) {
	srv, accounts := managed(t)
	reader, _ := personWith(t, accounts, "r@example.com", "MUS", account.Reader, "k")
	_, victim := personWith(t, accounts, "v@example.com", "MUS", account.Reader, "k2")

	for _, c := range []struct {
		path string
		form url.Values
	}{
		{"/account/invite", url.Values{"email": {"x@example.com"}, "role": {"owner"}}},
		{"/account/role", url.Values{"id": {victim.ID}, "role": {"owner"}}},
		{"/account/disable", url.Values{"id": {victim.ID}}},
	} {
		res := form(t, reader, srv, c.path, c.form)
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("a reader posting to %s got %d, want 403", c.path, res.StatusCode)
		}
	}
	// And nothing happened.
	if role, _ := accounts.RoleFor(context.Background(), victim.ID, "MUS"); role != account.Reader {
		t.Errorf("a reader changed somebody's role to %q", role)
	}
}

// The other lockout: the only owner cannot demote or disable themselves.
func TestTheLastOwnerCannotStandDown(t *testing.T) {
	srv, accounts := managed(t)
	owner, acct := personWith(t, accounts, "solo@example.com", "MUS", account.Owner, "k")

	page := body(t, owner, srv.URL+"/account")
	if !strings.Contains(page, "only owner") {
		t.Error("the page does not warn the only owner")
	}

	res := form(t, owner, srv, "/account/role", url.Values{"id": {acct.ID}, "role": {"reader"}})
	res.Body.Close()
	if role, _ := accounts.RoleFor(context.Background(), acct.ID, "MUS"); role != account.Owner {
		t.Errorf("the only owner demoted themselves to %q", role)
	}

	res = form(t, owner, srv, "/account/disable", url.Values{"id": {acct.ID}})
	res.Body.Close()
	people, err := accounts.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range people {
		if p.ID == acct.ID && p.Disabled {
			t.Error("the only owner disabled themselves, leaving nobody able to administer this")
		}
	}

	// With a second owner, standing down is allowed.
	_, other := personWith(t, accounts, "second@example.com", "MUS", account.Owner, "k2")
	_ = other
	res = form(t, owner, srv, "/account/role", url.Values{"id": {acct.ID}, "role": {"reader"}})
	res.Body.Close()
	if role, _ := accounts.RoleFor(context.Background(), acct.ID, "MUS"); role != account.Reader {
		t.Errorf("with a second owner, standing down was still refused: role is %q", role)
	}
}

// Disabling somebody signs them out rather than leaving their session alive
// until it expires.
func TestDisablingEndsTheirSession(t *testing.T) {
	srv, accounts := managed(t)
	owner, _ := personWith(t, accounts, "boss@example.com", "MUS", account.Owner, "k")
	victimClient, victim := personWith(t, accounts, "them@example.com", "MUS", account.Reader, "k2")

	if code := statusOf(t, victimClient, srv.URL+"/records"); code != http.StatusOK {
		t.Fatalf("the victim was not signed in to begin with: %d", code)
	}
	res := form(t, owner, srv, "/account/disable", url.Values{"id": {victim.ID}})
	res.Body.Close()

	if code := statusOf(t, victimClient, srv.URL+"/records"); code != http.StatusSeeOther {
		t.Errorf("a disabled account's session still works: %d", code)
	}
}

// The account page carries no script: adding a passkey is a link to the auth
// family's own page, so the scripted surfaces stay at three.
func TestTheAccountPageCarriesNoScript(t *testing.T) {
	srv, accounts := managed(t)
	owner, _ := personWith(t, accounts, "quiet@example.com", "MUS", account.Owner, "k")

	page := body(t, owner, srv.URL+"/account")
	if strings.Contains(page, "<script") {
		t.Error("the account page carries script; the count of scripted surfaces was three")
	}
	if !strings.Contains(page, `href="/account/passkey"`) {
		t.Error("no link to the page where a passkey is added")
	}
	// And that page is the scripted one.
	if got := body(t, owner, srv.URL+"/account/passkey"); !strings.Contains(got, "/assets/auth.js") {
		t.Error("the add-a-passkey page does not load the ceremony")
	}
}
