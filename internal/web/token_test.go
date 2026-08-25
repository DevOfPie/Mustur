package web

// Milestone 5c's clauses, each one against a running guard.
//
// The defect this exists to close was found by measuring rather than reading:
// with `--accounts` on, `POST /mcp` answered 403 to every caller without a
// session cookie, which is every agent. So these tests carry real headers to a
// real mux with the guard wrapped round it, and the negative cases assert the
// status an agent would actually see.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/mcpsrv"
	"github.com/DevOfPie/Mustur/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// tokenGuarded builds the same shape the server builds: /mcp on the guarded
// mux, alongside the browser surfaces.
func tokenGuarded(t *testing.T) (*httptest.Server, *account.Store) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	accounts := account.New(st.DB())

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpsrv.Handler(st))
	for _, path := range []string{"/records", "/questions", "/intake", "/sessions", "/compose"} {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("through"))
		})
	}
	auth := &Auth{Accounts: accounts, Origin: "http://127.0.0.1"}
	auth.Routes(mux)
	guard := &Guard{Auth: auth, Project: "MUS"}
	srv := httptest.NewServer(guard.Wrap(mux))
	t.Cleanup(srv.Close)
	return srv, accounts
}

// bearing carries one header and follows no redirects. The second half matters:
// the guard answers an unauthenticated browser GET with a 303 to /signin, and a
// client that follows it reports 200 on a public page — which made the first
// version of the scope test below pass for entirely the wrong reason.
type bearing struct{ header string }

func (b bearing) RoundTrip(r *http.Request) (*http.Response, error) {
	if b.header != "" {
		r.Header.Set("Authorization", b.header)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func agentClient(bearer string) *http.Client {
	return &http.Client{
		Transport:     bearing{header: bearer},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// call posts to a path with whatever Authorization header is given, and no
// cookie at all — which is what an agent has.
func call(t *testing.T, srv *httptest.Server, method, path, bearer string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	res, err := agentClient(bearer).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body := make([]byte, 4096)
	n, _ := res.Body.Read(body)
	return res.StatusCode, string(body[:n])
}

// The milestone's own sentence: an agent reaches the mandated tool call while
// accounts are enforced, carrying a token and no session cookie.
func TestAnAgentReachesTheToolCallWithATokenAndNoCookie(t *testing.T) {
	srv, accounts := tokenGuarded(t)
	ctx := context.Background()

	// Before: exactly the failure 5b shipped.
	if code, _ := call(t, srv, http.MethodPost, "/mcp", ""); code != http.StatusForbidden {
		t.Fatalf("an agent with no credential got %d; the guard is not on", code)
	}

	secret, tok, err := accounts.IssueToken(ctx, "claude-code on this machine", "MUS", account.Reader, "test")
	if err != nil {
		t.Fatal(err)
	}
	// A real MCP client through the real guard, because a bare POST is refused
	// by the protocol before the guard is even the question: tools/list is
	// invalid before initialize.
	client := mcp.NewClient(&mcp.Implementation{Name: "test-agent"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   srv.URL + "/mcp",
		HTTPClient: agentClient("Bearer " + secret),
	}, nil)
	if err != nil {
		t.Fatalf("an agent carrying a token could not connect: %v", err)
	}
	defer session.Close()

	// The whole point: the mandated call, answered.
	out, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "mustur_route",
		Arguments: map[string]any{"repository": "Mustur", "task": "milestone 5c"},
	})
	if err != nil {
		t.Fatalf("the mandated tool call failed: %v", err)
	}
	if out.IsError {
		t.Fatalf("the mandated tool call returned an error: %+v", out.Content)
	}

	// The same connection with a lowercase scheme, because clients differ.
	lower := mcp.NewClient(&mcp.Implementation{Name: "test-agent"}, nil)
	lowerSession, err := lower.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   srv.URL + "/mcp",
		HTTPClient: agentClient("bearer " + secret),
	}, nil)
	if err != nil {
		t.Errorf("a lowercase scheme was refused: %v", err)
	} else {
		lowerSession.Close()
	}
	// Using it is recorded, which is what makes revoking the other two safe.
	after, err := accounts.Tokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].ID != tok.ID || after[0].LastUsed.IsZero() {
		t.Errorf("the token was not marked as used: %+v", after)
	}
}

// Revocation is immediate, not at the next restart.
func TestRevokingATokenStopsItAtOnce(t *testing.T) {
	srv, accounts := tokenGuarded(t)
	ctx := context.Background()
	secret, tok, err := accounts.IssueToken(ctx, "an agent", "MUS", account.Reader, "test")
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := call(t, srv, http.MethodPost, "/mcp", "Bearer "+secret); code != http.StatusOK {
		t.Fatalf("the token did not work to begin with: %d", code)
	}
	if err := accounts.RevokeToken(ctx, tok.ID); err != nil {
		t.Fatal(err)
	}
	// Same process, same server, no restart.
	if code, _ := call(t, srv, http.MethodPost, "/mcp", "Bearer "+secret); code != http.StatusForbidden {
		t.Errorf("a revoked token still works: %d", code)
	}
	// Revoking twice says so rather than silently succeeding.
	if err := accounts.RevokeToken(ctx, tok.ID); err == nil {
		t.Error("revoking an already-revoked token reported success")
	}
	// The row survives, so somebody investigating can still see it existed.
	all, err := accounts.Tokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Live() {
		t.Errorf("the revoked token is gone or still live: %+v", all)
	}
}

// A token is not a passkey. It opens one path and cannot sign in.
func TestATokenOpensTheToolCallAndNothingElse(t *testing.T) {
	srv, accounts := tokenGuarded(t)
	ctx := context.Background()
	secret, _, err := accounts.IssueToken(ctx, "an agent", "MUS", account.Owner, "test")
	if err != nil {
		t.Fatal(err)
	}
	// An owner role on the token, so this is scope refusing rather than the
	// role refusing.
	for _, path := range []string{"/records", "/questions", "/intake", "/sessions", "/compose"} {
		code, _ := call(t, srv, http.MethodGet, path, "Bearer "+secret)
		// 303 to /signin for a readable surface, 403 for the two that type into
		// an agent. Either way not through, and never 200.
		if code != http.StatusSeeOther && code != http.StatusForbidden {
			t.Errorf("a token got %d on %s, which is a browser surface", code, path)
		}
	}
	for _, path := range []string{"/intake", "/questions"} {
		if code, _ := call(t, srv, http.MethodPost, path, "Bearer "+secret); code == http.StatusOK {
			t.Errorf("a token wrote to %s", path)
		}
	}
	// And it is not a session: nothing about it makes Whoever return anybody.
	if code, _ := call(t, srv, http.MethodGet, "/account", "Bearer "+secret); code == http.StatusOK {
		t.Errorf("a token reached the account surface: %d", code)
	}
}

// A token is scoped to a project, like every other role here.
func TestATokenForAnotherProjectIsRefused(t *testing.T) {
	srv, accounts := tokenGuarded(t)
	ctx := context.Background()
	secret, _, err := accounts.IssueToken(ctx, "an agent elsewhere", "IDW", account.Owner, "test")
	if err != nil {
		t.Fatal(err)
	}
	if code, body := call(t, srv, http.MethodPost, "/mcp", "Bearer "+secret); code != http.StatusForbidden {
		t.Errorf("a token for another project reached this one: %d %s", code, body)
	}
}

// Nonsense in the header falls through to the browser rules rather than being
// treated as anything. Each of these is a 403, not a panic and not a bypass.
func TestRubbishInTheHeaderIsJustRefused(t *testing.T) {
	srv, accounts := tokenGuarded(t)
	ctx := context.Background()
	secret, _, err := accounts.IssueToken(ctx, "real", "MUS", account.Reader, "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{
		"Bearer ",
		"Bearer not-a-token",
		"Basic " + secret,
		secret,
		"Bearer mus_",
		"B",
		"Bearer " + secret + "x",
	} {
		if code, _ := call(t, srv, http.MethodPost, "/mcp", h); code != http.StatusForbidden {
			t.Errorf("header %q got %d, want 403", h, code)
		}
	}
}
