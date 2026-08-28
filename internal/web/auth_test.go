package web

// The ceremony itself, against a virtual authenticator.
//
// This is milestone 5b's central clause and it was the one thing no test
// touched: every other test here starts after the passkey, with a session
// cookie minted by hand. What follows drives the real endpoints in the real
// order — invited, registered, signed out, recognised — and the things that
// must fail: a spent challenge, a passkey belonging to another site, a second
// device replacing a lost one.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/store"
)

// ceremonial builds a server that answers the whole authentication family, with
// its origin set to whatever address the test server ended up on — which is
// also the origin a passkey registered here is bound to.
func ceremonial(t *testing.T) (*httptest.Server, *account.Store, *Auth) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	accounts := account.New(st.DB())

	mux := http.NewServeMux()
	auth := &Auth{Accounts: accounts}
	auth.Routes(mux)
	manage := &Accounts{Store: accounts, Auth: auth, Project: "MUS"}
	manage.Routes(mux)
	mux.HandleFunc("/records", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("through"))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	// Only knowable once the listener has a port, which is why it is not in the
	// struct literal above.
	auth.Origin = srv.URL
	return srv, accounts, auth
}

// browser is a client that keeps cookies, because both halves of a ceremony
// depend on one: the challenge is held server-side under a cookie the begin
// call sets.
func browser(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

// send posts as the page's script does: same origin, JSON, cookies carried.
func send(t *testing.T, c *http.Client, srv *httptest.Server, path string, body []byte) (int, []byte) {
	t.Helper()
	if body == nil {
		body = []byte("{}")
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srv.URL)
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, out
}

// register runs an invitation all the way to a signed-in account.
func register(t *testing.T, c *http.Client, srv *httptest.Server, key *authenticator, secret string) {
	t.Helper()
	code, begin := send(t, c, srv, "/invite/"+secret+"/begin", nil)
	if code != http.StatusOK {
		t.Fatalf("beginning registration returned %d: %s", code, begin)
	}
	code, out := send(t, c, srv, "/invite/"+secret+"/finish", key.create(t, srv.URL, begin))
	if code != http.StatusOK {
		t.Fatalf("finishing registration returned %d: %s", code, out)
	}
}

// The milestone's own sentence, run end to end: a person is invited, registers
// a passkey, signs in, and is recognised on their next visit.
func TestAnInvitedPersonRegistersASignsOutAndIsRecognised(t *testing.T) {
	srv, accounts, _ := ceremonial(t)
	ctx := context.Background()
	secret, err := accounts.Invite(ctx, "new@example.com", "MUS", account.Reader, "test")
	if err != nil {
		t.Fatal(err)
	}

	key := newAuthenticator(t)
	c := browser(t)
	register(t, c, srv, key, secret)

	acct, ok := accounts.AccountByEmail(ctx, "new@example.com")
	if !ok {
		t.Fatal("registration signed nobody in")
	}
	creds, err := accounts.Credentials(ctx, acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 1 {
		t.Fatalf("%d passkeys stored after one registration", len(creds))
	}
	// Registering signs you in, which is the point of doing it at all.
	if got := pageOf(t, c, srv.URL+"/account"); !strings.Contains(got, "new@example.com") {
		t.Error("the account page does not know who just registered")
	}

	// Sign out, then back in with nothing typed: the whole reason for
	// discoverable credentials.
	if code, out := send(t, c, srv, "/signout", nil); code != http.StatusSeeOther && code != http.StatusOK {
		t.Fatalf("signing out returned %d: %s", code, out)
	}
	code, begin := send(t, c, srv, "/signin/begin", nil)
	if code != http.StatusOK {
		t.Fatalf("beginning sign-in returned %d: %s", code, begin)
	}
	code, out := send(t, c, srv, "/signin/finish", key.get(t, srv.URL, begin, []byte(acct.ID)))
	if code != http.StatusOK {
		t.Fatalf("signing in with a registered passkey returned %d: %s", code, out)
	}
	if got := pageOf(t, c, srv.URL+"/account"); !strings.Contains(got, "new@example.com") {
		t.Error("signing in did not recognise the account the passkey belongs to")
	}
}

// A challenge is spent once. Replaying a whole response — which is what
// somebody who captured one would have — is refused.
func TestAChallengeCannotBeSpentTwice(t *testing.T) {
	srv, accounts, _ := ceremonial(t)
	ctx := context.Background()
	secret, _ := accounts.Invite(ctx, "once@example.com", "MUS", account.Reader, "test")

	key := newAuthenticator(t)
	c := browser(t)
	register(t, c, srv, key, secret)
	acct, _ := accounts.AccountByEmail(ctx, "once@example.com")

	code, begin := send(t, c, srv, "/signin/begin", nil)
	if code != http.StatusOK {
		t.Fatalf("beginning sign-in returned %d", code)
	}
	assertion := key.get(t, srv.URL, begin, []byte(acct.ID))
	if code, _ := send(t, c, srv, "/signin/finish", assertion); code != http.StatusOK {
		t.Fatalf("the first use of the assertion returned %d", code)
	}
	// The same bytes again. The ceremony was deleted when it was read, so there
	// is no challenge left to match.
	if code, _ := send(t, c, srv, "/signin/finish", assertion); code != http.StatusForbidden {
		t.Errorf("a replayed assertion returned %d, want 403", code)
	}
}

// A passkey is bound to the site it was made for. One signed for a different
// relying party is refused, which is the property that makes a phishing page
// unable to use anything it collects.
func TestAPasskeyForAnotherSiteIsRefused(t *testing.T) {
	srv, accounts, _ := ceremonial(t)
	ctx := context.Background()
	secret, _ := accounts.Invite(ctx, "bound@example.com", "MUS", account.Reader, "test")

	key := newAuthenticator(t)
	c := browser(t)
	register(t, c, srv, key, secret)
	acct, _ := accounts.AccountByEmail(ctx, "bound@example.com")

	code, begin := send(t, c, srv, "/signin/begin", nil)
	if code != http.StatusOK {
		t.Fatalf("beginning sign-in returned %d", code)
	}
	// The authenticator is told it is talking to somewhere else, so it hashes
	// that name into what it signs.
	var opts map[string]any
	if err := json.Unmarshal(begin, &opts); err != nil {
		t.Fatal(err)
	}
	opts["publicKey"].(map[string]any)["rpId"] = "attacker.example"
	elsewhere, err := json.Marshal(opts)
	if err != nil {
		t.Fatal(err)
	}
	code, out := send(t, c, srv, "/signin/finish", key.get(t, srv.URL, elsewhere, []byte(acct.ID)))
	if code != http.StatusForbidden {
		t.Errorf("a passkey signed for another site returned %d, want 403: %s", code, out)
	}

	// The control, so the refusal above is attributable to the one field that
	// differs. The same authenticator, the same browser, the same endpoints,
	// with the relying party the server actually named.
	code, begin = send(t, c, srv, "/signin/begin", nil)
	if code != http.StatusOK {
		t.Fatalf("beginning sign-in returned %d", code)
	}
	if code, out := send(t, c, srv, "/signin/finish", key.get(t, srv.URL, begin, []byte(acct.ID))); code != http.StatusOK {
		t.Fatalf("the control run failed too, so the refusal above proved nothing: %d %s", code, out)
	}
}

// The clause the owner added when they chose passkeys: a device is lost, and
// the account survives it. A fresh invitation to the same address registers a
// second passkey onto the same account rather than making a second one.
func TestALostDeviceIsReplacedWithoutLosingTheAccount(t *testing.T) {
	srv, accounts, _ := ceremonial(t)
	ctx := context.Background()
	secret, _ := accounts.Invite(ctx, "lost@example.com", "MUS", account.Owner, "test")

	old := newAuthenticator(t)
	register(t, browser(t), srv, old, secret)
	first, ok := accounts.AccountByEmail(ctx, "lost@example.com")
	if !ok {
		t.Fatal("the first registration made no account")
	}

	// The device is gone. The owner issues another invitation to the same
	// address, which is the recovery path — there is nothing else to prove.
	again, err := accounts.Invite(ctx, "lost@example.com", "MUS", account.Owner, "test")
	if err != nil {
		t.Fatal(err)
	}
	replacement := newAuthenticator(t)
	c := browser(t)
	register(t, c, srv, replacement, again)

	second, _ := accounts.AccountByEmail(ctx, "lost@example.com")
	if second.ID != first.ID {
		t.Errorf("the replacement made a second account: %q then %q", first.ID, second.ID)
	}
	all, err := accounts.Accounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Errorf("%d accounts exist for one person", len(all))
	}
	// And the new device signs in on its own.
	code, begin := send(t, c, srv, "/signin/begin", nil)
	if code != http.StatusOK {
		t.Fatalf("beginning sign-in returned %d", code)
	}
	if code, out := send(t, c, srv, "/signin/finish", replacement.get(t, srv.URL, begin, []byte(second.ID))); code != http.StatusOK {
		t.Fatalf("the replacement passkey returned %d: %s", code, out)
	}
	// Roles survive too: recovery is not a demotion.
	if role, _ := accounts.RoleFor(ctx, second.ID, "MUS"); role != account.Owner {
		t.Errorf("after recovery the role is %q", role)
	}
}

// Adding a second passkey without losing the first — the same protection
// against a lost device, taken before it is lost.
func TestASecondPasskeyIsAddedFromTheAccountPage(t *testing.T) {
	srv, accounts, _ := ceremonial(t)
	ctx := context.Background()
	secret, _ := accounts.Invite(ctx, "two@example.com", "MUS", account.Reader, "test")

	first := newAuthenticator(t)
	c := browser(t)
	register(t, c, srv, first, secret)
	acct, _ := accounts.AccountByEmail(ctx, "two@example.com")

	second := newAuthenticator(t)
	code, begin := send(t, c, srv, "/account/passkey/begin", nil)
	if code != http.StatusOK {
		t.Fatalf("beginning the second registration returned %d: %s", code, begin)
	}
	if code, out := send(t, c, srv, "/account/passkey/finish", second.create(t, srv.URL, begin)); code != http.StatusOK {
		t.Fatalf("adding a second passkey returned %d: %s", code, out)
	}
	creds, err := accounts.Credentials(ctx, acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 2 {
		t.Fatalf("%d passkeys after adding one to an account that had one", len(creds))
	}
	// Either one gets in.
	for i, key := range []*authenticator{first, second} {
		fresh := browser(t)
		code, begin := send(t, fresh, srv, "/signin/begin", nil)
		if code != http.StatusOK {
			t.Fatalf("beginning sign-in returned %d", code)
		}
		if code, out := send(t, fresh, srv, "/signin/finish", key.get(t, srv.URL, begin, []byte(acct.ID))); code != http.StatusOK {
			t.Errorf("passkey %d could not sign in: %d %s", i+1, code, out)
		}
	}
}

// A ceremony belongs to the browser that began it. Somebody else's response,
// posted with your ceremony cookie, is not your passkey.
func TestAnAddCeremonyCannotBeFinishedByAnotherAccount(t *testing.T) {
	srv, accounts, _ := ceremonial(t)
	ctx := context.Background()

	mine, _ := accounts.Invite(ctx, "mine@example.com", "MUS", account.Reader, "test")
	theirs, _ := accounts.Invite(ctx, "theirs@example.com", "MUS", account.Reader, "test")
	me, them := browser(t), browser(t)
	register(t, me, srv, newAuthenticator(t), mine)
	register(t, them, srv, newAuthenticator(t), theirs)

	// I begin adding a passkey; they finish it, carrying their own session and
	// my ceremony cookie. Handing the cookie over is what makes this a test of
	// the handle check rather than of the cookie — without it the refusal would
	// be "no ceremony" and would prove nothing about whose it was.
	code, begin := send(t, me, srv, "/account/passkey/begin", nil)
	if code != http.StatusOK {
		t.Fatalf("beginning returned %d", code)
	}
	at, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	var handed bool
	for _, c := range me.Jar.Cookies(at) {
		if c.Name == ceremonyCookie {
			them.Jar.SetCookies(at, []*http.Cookie{c})
			handed = true
		}
	}
	if !handed {
		t.Fatal("no ceremony cookie to hand over, so this would have proved nothing")
	}
	stolen := newAuthenticator(t).create(t, srv.URL, begin)
	code, out := send(t, them, srv, "/account/passkey/finish", stolen)
	if code == http.StatusOK {
		t.Errorf("another account finished my ceremony: %s", out)
	}
	// Which refusal, named. Removing Mustur's own check left this passing on the
	// library's session check underneath it — a true outcome proving nothing
	// about the line meant to be under test. The message pins it.
	if !strings.Contains(string(out), "does not belong to this account") {
		t.Errorf("refused by something other than the handle check: %d %s", code, out)
	}
	acct, _ := accounts.AccountByEmail(ctx, "theirs@example.com")
	creds, _ := accounts.Credentials(ctx, acct.ID)
	if len(creds) != 1 {
		t.Errorf("%d passkeys on the other account; one was added by my ceremony", len(creds))
	}
}

func pageOf(t *testing.T, c *http.Client, url string) string {
	t.Helper()
	res, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The script loads where there is a ceremony, and not where there is none.
//
// The tag sat outside every conditional, so it also loaded on the
// invitation-failure page and on the sign-in page of an install with nobody in
// it — two pages holding nothing for it to bind to, counted as scripted for no
// reason.
func TestTheCeremonyScriptLoadsOnlyWhereThereIsOne(t *testing.T) {
	srv, accounts, _ := ceremonial(t)
	ctx := context.Background()
	c := browser(t)

	// Nobody has an account: the page names the command instead of offering a
	// button, so there is nothing to run.
	if got := pageOf(t, c, srv.URL+"/signin"); strings.Contains(got, "auth.js") {
		t.Error("the empty sign-in page loads a ceremony it has no button for")
	}
	secret, err := accounts.Invite(ctx, "somebody@example.com", "MUS", account.Reader, "test")
	if err != nil {
		t.Fatal(err)
	}
	// A live invitation still leaves no accounts, so sign-in keeps naming the
	// command. Only somebody actually registering changes it.
	register(t, browser(t), srv, newAuthenticator(t), secret)
	// Now there is somebody to sign in as, so there is a button.
	if got := pageOf(t, c, srv.URL+"/signin"); !strings.Contains(got, "auth.js") {
		t.Error("sign-in cannot run its ceremony")
	}
	// A bad invitation renders a message and nothing to press.
	if got := pageOf(t, c, srv.URL+"/invite/not-a-token"); strings.Contains(got, "auth.js") {
		t.Error("the invitation-failure page loads a ceremony it has no button for")
	}
}
