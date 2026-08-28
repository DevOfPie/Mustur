package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/store"
)

// held stands in for the one request that does not end by itself.
//
// The real one is MCP's server-to-client stream, a GET the SDK holds open until
// its context is done. This is that shape and nothing else: it blocks on the
// context the guard handed it and reports when it was let go. Testing the guard
// against the SDK's stream would measure the SDK; what changed is the guard.
type held struct{ ended chan struct{} }

func (h held) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	<-r.Context().Done()
	close(h.ended)
}

func holding(t *testing.T) (*httptest.Server, *account.Store, held) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	accounts := account.New(st.DB())
	stream := held{ended: make(chan struct{})}
	mux := http.NewServeMux()
	mux.Handle("/mcp", stream)
	auth := &Auth{Accounts: accounts, Origin: "http://127.0.0.1"}
	auth.Routes(mux)
	srv := httptest.NewServer((&Guard{Auth: auth, Project: "MUS"}).Wrap(mux))
	t.Cleanup(srv.Close)
	return srv, accounts, stream
}

// open starts a request that stays open, and gives back the way to end it.
//
// The hang-up matters more than it looks. httptest's Close waits for every
// handler to return, and this handler returns only when its context is done —
// so a test that never disconnects deadlocks in cleanup rather than failing.
// Registering the hang-up after holding() puts it before Close in the LIFO
// cleanup order, which is the only ordering that unwedges it.
func open(t *testing.T, srv *httptest.Server, secret string) {
	t.Helper()
	ctx, hangUp := context.WithCancel(context.Background())
	t.Cleanup(hangUp)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		res, err := agentClient("Bearer " + secret).Do(req)
		if err == nil {
			res.Body.Close()
		}
	}()
}

// Revoking a token closes a stream already running under it.
//
// MUS-F-0028: enforcement was per request, and the one request that matters has
// no end. `mustur account revoke` says a token stops working immediately, and
// that was true of every call and false of the connection already open — which
// was measured still running three seconds after.
func TestRevokingATokenEndsAStreamAlreadyOpen(t *testing.T) {
	was := TokenRecheck
	TokenRecheck = 20 * time.Millisecond
	t.Cleanup(func() { TokenRecheck = was })

	srv, accounts, stream := holding(t)
	ctx := context.Background()
	secret, tok, err := accounts.IssueToken(ctx, "a stream", "MUS", account.Reader, "test", 0)
	if err != nil {
		t.Fatal(err)
	}
	open(t, srv, secret)

	// It has to be running before revoking it, or the test would pass on a
	// request the guard refused at the door — which is the old behaviour.
	select {
	case <-stream.ended:
		t.Fatal("the stream ended before the token was revoked")
	case <-time.After(150 * time.Millisecond):
	}

	if err := accounts.RevokeToken(ctx, tok.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stream.ended:
	case <-time.After(2 * time.Second):
		t.Fatal("the stream outlived its token")
	}
}

// The other half, and the one a recheck could quietly break: a token nobody
// revoked holds its stream open across many ticks.
func TestAStreamSurvivesWhileItsTokenDoes(t *testing.T) {
	was := TokenRecheck
	TokenRecheck = 10 * time.Millisecond
	t.Cleanup(func() { TokenRecheck = was })

	srv, accounts, stream := holding(t)
	secret, _, err := accounts.IssueToken(context.Background(), "a stream", "MUS", account.Reader, "test", 0)
	if err != nil {
		t.Fatal(err)
	}
	open(t, srv, secret)

	select {
	case <-stream.ended:
		t.Fatal("a stream was cut while its token was still good")
	case <-time.After(300 * time.Millisecond):
	}
}
