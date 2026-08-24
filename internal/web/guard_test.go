package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/store"
)

// guarded builds a server with one behind-the-guard handler and returns the
// account store, so a test can invite whoever it needs.
func guarded(t *testing.T) (*httptest.Server, *account.Store) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	accounts := account.New(st.DB())

	mux := http.NewServeMux()
	// Stand-ins for the real surfaces, so this tests the guard rather than any
	// one page's behaviour.
	for _, path := range []string{"/records", "/questions", "/intake", "/sessions", "/sessions/Mustur", "/compose"} {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("through"))
		})
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	auth := &Auth{Accounts: accounts, Origin: "http://127.0.0.1"}
	auth.Routes(mux)
	guard := &Guard{Auth: auth, Project: "MUS"}
	srv := httptest.NewServer(guard.Wrap(mux))
	t.Cleanup(srv.Close)
	return srv, accounts
}

// signedInAs invites somebody, redeems, and returns a client carrying their
// session cookie. The passkey ceremony itself needs a browser; everything after
// it is a cookie, which is what the guard actually reads.
func signedInAs(t *testing.T, srv *httptest.Server, accounts *account.Store, email, project string, role account.Role) *http.Client {
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
	cookie, _, err := accounts.StartSession(ctx, acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	jarClient := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     cookieAdder{cookie: &http.Cookie{Name: SessionCookie, Value: cookie}},
	}
	return jarClient
}

type cookieAdder struct{ cookie *http.Cookie }

func (c cookieAdder) RoundTrip(r *http.Request) (*http.Response, error) {
	r.AddCookie(c.cookie)
	return http.DefaultTransport.RoundTrip(r)
}

func statusOf(t *testing.T, c *http.Client, url string) int {
	t.Helper()
	res, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	return res.StatusCode
}

func postTo(t *testing.T, c *http.Client, url string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	return res.StatusCode
}

// The reason accounts exist: a reader can be invited without inviting somebody
// who can type into an agent.
func TestAReaderReadsAndCannotReachAnAgent(t *testing.T) {
	srv, accounts := guarded(t)
	reader := signedInAs(t, srv, accounts, "reader@example.com", "MUS", account.Reader)

	for _, path := range []string{"/records", "/questions", "/intake"} {
		if code := statusOf(t, reader, srv.URL+path); code != http.StatusOK {
			t.Errorf("a reader got %d on %s, which is a reading surface", code, path)
		}
	}
	// The two that type into a running agent, refused even to read: the session
	// page holds a socket that carries keystrokes back.
	for _, path := range []string{"/sessions", "/sessions/Mustur", "/compose"} {
		if code := statusOf(t, reader, srv.URL+path); code != http.StatusForbidden {
			t.Errorf("a reader got %d on %s; that surface types into an agent", code, path)
		}
	}
	// And nothing that changes anything.
	for _, path := range []string{"/intake", "/questions"} {
		if code := postTo(t, reader, srv.URL+path); code != http.StatusForbidden {
			t.Errorf("a reader posted to %s and got %d", path, code)
		}
	}
}

func TestAnOwnerReachesEverything(t *testing.T) {
	srv, accounts := guarded(t)
	owner := signedInAs(t, srv, accounts, "owner@example.com", "MUS", account.Owner)

	for _, path := range []string{"/records", "/questions", "/intake", "/sessions", "/compose"} {
		if code := statusOf(t, owner, srv.URL+path); code != http.StatusOK {
			t.Errorf("an owner got %d on %s", code, path)
		}
	}
	if code := postTo(t, owner, srv.URL+"/intake"); code != http.StatusOK {
		t.Errorf("an owner could not post: %d", code)
	}
}

// No role on a project is not a lesser role. An account granted nothing here
// cannot read it either.
func TestAnAccountWithNoRoleOnThisProjectIsRefused(t *testing.T) {
	srv, accounts := guarded(t)
	elsewhere := signedInAs(t, srv, accounts, "other@example.com", "IDW", account.Owner)

	if code := statusOf(t, elsewhere, srv.URL+"/records"); code != http.StatusForbidden {
		t.Errorf("an account with a role only on another project read this one: %d", code)
	}
}

// Nobody signed in: a browser is pointed at sign-in, and a write is refused
// rather than redirected — a redirect answering a POST looks like success.
func TestAStrangerIsSentToSignIn(t *testing.T) {
	srv, _ := guarded(t)
	anon := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	res, err := anon.Get(srv.URL + "/records")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Errorf("a stranger got %d rather than a redirect", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/signin" {
		t.Errorf("redirected to %q", loc)
	}
	if code := postTo(t, anon, srv.URL+"/intake"); code != http.StatusForbidden {
		t.Errorf("a stranger's post got %d, want 403 — a redirect would read as success", code)
	}
}

// Sign-in has to be reachable by somebody with no account, or nobody can ever
// get one.
func TestTheSignInSurfaceIsPublic(t *testing.T) {
	srv, accounts := guarded(t)
	anon := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	for _, path := range []string{"/signin", "/assets/auth.js", "/healthz"} {
		if code := statusOf(t, anon, srv.URL+path); code != http.StatusOK {
			t.Errorf("%s is behind the guard at %d; nobody could ever sign in", path, code)
		}
	}

	secret, err := accounts.Invite(context.Background(), "new@example.com", "MUS", account.Reader, "test")
	if err != nil {
		t.Fatal(err)
	}
	if code := statusOf(t, anon, srv.URL+"/invite/"+secret); code != http.StatusOK {
		t.Errorf("an invitation link is behind the guard at %d", code)
	}
	// And an invitation that is not real still renders a page rather than
	// leaking which part was wrong.
	if code := statusOf(t, anon, srv.URL+"/invite/not-a-token"); code != http.StatusOK {
		t.Errorf("a bad invitation gave %d rather than a page saying it cannot be used", code)
	}
}

// A signed-out cookie stops working immediately.
func TestSigningOutEndsIt(t *testing.T) {
	srv, accounts := guarded(t)
	ctx := context.Background()
	secret, err := accounts.Invite(ctx, "bye@example.com", "MUS", account.Owner, "test")
	if err != nil {
		t.Fatal(err)
	}
	acct, _, err := accounts.Redeem(ctx, secret, "")
	if err != nil {
		t.Fatal(err)
	}
	cookie, _, err := accounts.StartSession(ctx, acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	c := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     cookieAdder{cookie: &http.Cookie{Name: SessionCookie, Value: cookie}},
	}
	if code := statusOf(t, c, srv.URL+"/records"); code != http.StatusOK {
		t.Fatalf("not signed in to begin with: %d", code)
	}
	if err := accounts.EndSession(ctx, cookie); err != nil {
		t.Fatal(err)
	}
	if code := statusOf(t, c, srv.URL+"/records"); code != http.StatusSeeOther {
		t.Errorf("a signed-out cookie still reaches a surface: %d", code)
	}
}
