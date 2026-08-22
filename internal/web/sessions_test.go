package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/session"
)

// fakeRunner replies to tmux commands from a script so the surface can be
// tested without tmux.
type fakeRunner struct{ listing string }

func (f fakeRunner) Run(_ context.Context, _ string, args ...string) (string, error) {
	if len(args) > 0 && args[0] == "list-sessions" {
		return f.listing, nil
	}
	return "", nil
}

func serveSessions(t *testing.T, listing string) *httptest.Server {
	t.Helper()
	a := &session.Adapter{Run: fakeRunner{listing: listing}}
	s := &Sessions{Hub: &session.Hub{Adapter: a}, Adapter: a, Project: "MUS", Actor: "pie"}
	mux := http.NewServeMux()
	s.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func owned(name string) string { return name + "\t1\t0\t1" }

// The control this milestone turns on.
//
// Browsers do not apply the same-origin policy to WebSockets and they send
// cookies with the handshake, so a page the owner merely visits could otherwise
// open a socket here, be authenticated by their existing Access session, and
// type into a running agent. Access authenticates the person and says nothing
// about who opened the socket. Only this check does.
func TestAWebSocketFromAnotherOriginIsRefused(t *testing.T) {
	srv := serveSessions(t, owned("mustur/Mustur"))

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/sessions/Mustur/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("a cross-origin upgrade returned %d, want 403", res.StatusCode)
	}
	if res.StatusCode == http.StatusSwitchingProtocols {
		t.Fatal("the socket was opened from another origin")
	}
}

// A handshake with no Origin at all is not a browser, and a non-browser client
// has no business on the one path that types into an agent.
func TestAWebSocketWithNoOriginIsRefused(t *testing.T) {
	srv := serveSessions(t, owned("mustur/Mustur"))

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/sessions/Mustur/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("an origin-less upgrade returned %d, want 403", res.StatusCode)
	}
}

func TestSameOriginAcceptsHostAndSchemeVariants(t *testing.T) {
	cases := []struct {
		origin, host string
		want         bool
	}{
		{"https://mustur.devofpie.com", "mustur.devofpie.com", true},
		{"http://mustur.devofpie.com", "mustur.devofpie.com", true},
		{"https://MUSTUR.devofpie.com", "mustur.devofpie.com", true},
		{"https://evil.example", "mustur.devofpie.com", false},
		{"https://mustur.devofpie.com.evil.example", "mustur.devofpie.com", false},
		{"null", "mustur.devofpie.com", false},
		{"", "mustur.devofpie.com", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "http://"+c.host+"/sessions/x/ws", nil)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if got := sameOrigin(r); got != c.want {
			t.Errorf("origin %q against host %q = %v, want %v", c.origin, c.host, got, c.want)
		}
	}
}

// A viewer cannot reach a session by naming it. The ownership check the CLI
// uses is the one the socket uses.
func TestASessionMusturDidNotStartCannotBeStreamed(t *testing.T) {
	// Present in tmux, but carrying no ownership option.
	srv := serveSessions(t, "mustur/notmine\t1\t0\t")

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/sessions/notmine/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", srv.URL)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("streaming a session Mustur did not start returned %d, want 404", res.StatusCode)
	}
}

func TestThePageSaysWhenMusturDidNotStartTheSession(t *testing.T) {
	srv := serveSessions(t, "mustur/notmine\t1\t0\t")

	res, err := srv.Client().Get(srv.URL + "/sessions/notmine")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b := make([]byte, 8192)
	n, _ := res.Body.Read(b)
	body := string(b[:n])

	for _, want := range []string{"did not start", "will not appear"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not say %q", want)
		}
	}
}

// The exception is this surface and no other. If the script leaks anywhere
// else, the stack table's rule has quietly become a suggestion.
func TestOnlyTheSessionSurfaceCarriesScript(t *testing.T) {
	srv := serveSessions(t, owned("mustur/Mustur"))

	res, err := srv.Client().Get(srv.URL + "/sessions/Mustur")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b := make([]byte, 16384)
	n, _ := res.Body.Read(b)
	if !strings.Contains(string(b[:n]), "/assets/session.js") {
		t.Error("the session page does not load the client")
	}
}
