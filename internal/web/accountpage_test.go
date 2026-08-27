package web

import (
	"context"
	"io"
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
	manage := &Accounts{Store: accounts, Auth: auth, Project: "MUS", ShowSessions: true}
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

	// The first screen is theirs, and carries the way to the second. A screen
	// drawn, described and unreachable is how the first attempt at this shipped.
	page := body(t, owner, srv.URL+"/account")
	if !strings.Contains(page, "owner@example.com") {
		t.Error("the account page does not say whose it is")
	}
	if !strings.Contains(page, `href="/account/people"`) {
		t.Error("an owner has no way to reach the people screen")
	}
	if strings.Contains(page, "Invite somebody") {
		t.Error("the invite form is on the account screen; it belongs on the second one")
	}

	people := body(t, owner, srv.URL+"/account/people")
	for _, want := range []string{"Invite somebody", "owner@example.com"} {
		if !strings.Contains(people, want) {
			t.Errorf("the people screen does not show %q", want)
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
	if !strings.HasPrefix(loc, "/account/people") {
		t.Errorf("inviting returned to %q rather than the screen it was done on", loc)
	}
	// Shown once, whole, and the page says so: the secret is not stored, so a
	// truncated link and a closed tab cost the same — another invitation.
	shown := body(t, owner, srv.URL+loc)
	if !strings.Contains(shown, "Shown once") {
		t.Error("the page does not say the link cannot be shown again")
	}
	if strings.Contains(shown, "\u2026") || strings.Contains(shown, "&hellip;") {
		t.Error("the invitation link is elided; a truncated one-time secret is a destroyed one")
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

	// No standing warning: the page said "you are the only owner" above controls
	// that still looked usable, which reads as a caption rather than a rule
	// (MUS-Q-0048). The refusal carries the reason instead.
	page := body(t, owner, srv.URL+"/account/people")
	if strings.Contains(page, "only owner") {
		t.Error("the page warns in advance; the refusal is supposed to say it")
	}

	res := form(t, owner, srv, "/account/role", url.Values{"id": {acct.ID}, "role": {"reader"}})
	res.Body.Close()
	if role, _ := accounts.RoleFor(context.Background(), acct.ID, "MUS"); role != account.Owner {
		t.Errorf("the only owner demoted themselves to %q", role)
	}
	if said := body(t, owner, srv.URL+res.Header.Get("Location")); !strings.Contains(said, "only owner") {
		t.Error("the refusal does not say why it refused")
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

// Adding a passkey happens where the passkeys are listed. It had a page of its
// own — a heading and one button — which a review named for what it was.
func TestAddingAPasskeyHappensOnTheAccountPage(t *testing.T) {
	srv, accounts := managed(t)
	owner, _ := personWith(t, accounts, "quiet@example.com", "MUS", account.Owner, "k")

	page := body(t, owner, srv.URL+"/account")
	if !strings.Contains(page, "/assets/auth.js") {
		t.Error("the account page does not load the ceremony, so no passkey can be added from it")
	}
	if !strings.Contains(page, `id="addkey"`) {
		t.Error("nothing on the account page starts the ceremony")
	}
	// The page it replaced is gone rather than orphaned.
	if code := statusOf(t, owner, srv.URL+"/account/passkey"); code != http.StatusNotFound {
		t.Errorf("the add-a-passkey page still answers with %d", code)
	}
	// Everything else still works without script: the role select keeps its
	// Save button behind a <noscript>, and every action is a form.
	people := body(t, owner, srv.URL+"/account/people")
	if !strings.Contains(people, "<noscript>") {
		t.Error("the role control has no scriptless path")
	}
}

// The second screen is the owner's. A reader is refused rather than shown an
// empty one, which would read as "nobody is here" instead of "not yours".
func TestAReaderHasNoPeopleScreen(t *testing.T) {
	srv, accounts := managed(t)
	reader, _ := personWith(t, accounts, "r2@example.com", "MUS", account.Reader, "k")

	page := body(t, reader, srv.URL+"/account")
	if strings.Contains(page, `href="/account/people"`) {
		t.Error("a reader is offered a screen they cannot open")
	}
	// And their own screen is whole: there is nothing on it they should not
	// see, so it is not trimmed (MUS-Q-0046).
	if !strings.Contains(page, "r2@example.com") || !strings.Contains(page, `id="addkey"`) {
		t.Error("a reader's own account screen is missing part of itself")
	}
	if code := statusOf(t, reader, srv.URL+"/account/people"); code != http.StatusForbidden {
		t.Errorf("a reader reached the people screen: %d", code)
	}
}

// The account page can be left again.
//
// Every other surface carries the same four-tab row. This one carried three:
// Records, Decisions, Intake, and no Sessions — so an owner who reached their
// account from a session had no way back to it but the browser's own history
// (MUS-F-0040). The header link goes one way only.
//
// Gated on ShowSessions like everywhere else, because a build served without
// --sessions has no such surface and a dead tab is worse than an absent one
// (MUS-Q-0052).
func TestTheAccountPageOffersTheWayBackToASession(t *testing.T) {
	srv, accounts := managed(t)
	client, _ := personWith(t, accounts, "pie@example.com", "MUS", account.Owner, "k1")

	for _, path := range []string{"/account", "/account/people"} {
		res, err := client.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		page := string(body)
		if !strings.Contains(page, `<a href="/sessions" aria-label="Sessions">`) {
			t.Errorf("%s has no way back to a session", path)
		}
		// The word is still in the markup even where the bar hides it, and the
		// tab names itself for anyone who cannot see the drawing.
		if !strings.Contains(page, `<span>Sessions</span>`) {
			t.Errorf("%s dropped the word rather than hiding it", path)
		}
		// And the row is the same one every other surface has, in the same
		// order, so the tabs do not move as you cross between them.
		for _, want := range []string{`href="/questions"`, `href="/intake"`, `href="/records"`} {
			if !strings.Contains(page, want) {
				t.Errorf("%s is missing %s from the nav", path, want)
			}
		}
		if at, sessions := strings.Index(page, `href="/sessions"`), strings.Index(page, `href="/questions"`); at > sessions {
			t.Errorf("%s puts Sessions after Decisions; every other surface leads with it", path)
		}
	}
}

// A build with no session surface offers no tab to it, rather than a dead one.
func TestTheAccountPageHidesSessionsWhenThereIsNoSuchSurface(t *testing.T) {
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
	// ShowSessions left off, which is what a server without --sessions is.
	(&Accounts{Store: accounts, Auth: auth, Project: "MUS"}).Routes(mux)
	guard := &Guard{Auth: auth, Project: "MUS"}
	srv := httptest.NewServer(guard.Wrap(mux))
	t.Cleanup(srv.Close)

	client, _ := personWith(t, accounts, "pie@example.com", "MUS", account.Owner, "k1")
	res, err := client.Get(srv.URL + "/account")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if strings.Contains(string(body), `href="/sessions"`) {
		t.Error("a build with no session surface still offers a tab to it")
	}
}
