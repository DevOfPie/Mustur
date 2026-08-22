package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/session"
	"github.com/DevOfPie/Mustur/internal/store"
	"github.com/coder/websocket"
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
	s := &Sessions{Hub: &session.Hub{Adapter: a}, Adapter: a, Actor: "pie"}
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

// The exception is this surface and no other. The first version of this test
// asserted only that the session page *does* load the client and checked no
// other surface — it could not fail for the reason its name gave.
func TestOnlyTheSessionSurfaceCarriesScript(t *testing.T) {
	srv := serveSessions(t, owned("mustur/Mustur"))
	if !strings.Contains(getFrom(t, srv, "/sessions/Mustur"), "/assets/session.js") {
		t.Error("the session page does not load the client")
	}

	// Every other surface, served from its own handler, must carry none.
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	in := &Intake{Store: st, Project: "MUS", Actor: "pie"}
	q := &Questions{Store: st, Project: "MUS", Actor: "pie"}
	mux := http.NewServeMux()
	q.Routes(mux)
	mux.Handle("/", in.Handler())
	other := httptest.NewServer(mux)
	defer other.Close()

	for _, path := range []string{"/intake", "/questions"} {
		if body := getFrom(t, other, path); strings.Contains(body, "<script") {
			t.Errorf("%s carries script; the exception has become a suggestion", path)
		}
	}
}

// A page with no socket and no composer has nothing for the client to do, and
// loading it there was the exception spreading by accident rather than by
// decision.
func TestAPageWithNoSessionCarriesNoScript(t *testing.T) {
	srv := serveSessions(t, "")
	if body := getFrom(t, srv, "/sessions/nosuchproject"); strings.Contains(body, "<script") {
		t.Error("the no-session page loads the client")
	}
}

// "The bar grows as the rest arrive" is a promise that has to be kept in three
// templates at once. Sessions arrived at 4b and only its own page grew, leaving
// two, two and three tabs in one binary.
func TestEverySurfaceCarriesTheSameBar(t *testing.T) {
	tabs := []string{`href="/sessions"`, `href="/questions"`, `href="/intake"`}

	srv := serveSessions(t, owned("mustur/Mustur"))
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	in := &Intake{Store: st, Project: "MUS", Actor: "pie"}
	q := &Questions{Store: st, Project: "MUS", Actor: "pie"}
	mux := http.NewServeMux()
	q.Routes(mux)
	mux.Handle("/", in.Handler())
	other := httptest.NewServer(mux)
	defer other.Close()

	pages := map[string]string{
		"/sessions/Mustur": getFrom(t, srv, "/sessions/Mustur"),
		"/questions":       getFrom(t, other, "/questions"),
		"/intake":          getFrom(t, other, "/intake"),
	}
	for path, body := range pages {
		for _, tab := range tabs {
			if !strings.Contains(body, tab) {
				t.Errorf("%s is missing %s from its bar", path, tab)
			}
		}
	}
}

// Sub-agent rows on the session surface.
//
// The hook writes a log; this reads it back through the page, so a change to
// either half that the other does not expect shows up here rather than as an
// empty strip on a phone.
func TestTheSessionPageShowsSubagents(t *testing.T) {
	dir := t.TempDir()
	a := &session.Adapter{Run: fakeRunner{listing: owned("mustur/Mustur")}}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	s := &Sessions{
		Hub: &session.Hub{Adapter: a}, Adapter: a, Actor: "pie",
		HookDir: dir, Now: func() time.Time { return now.Add(3 * time.Minute) },
	}
	mux := http.NewServeMux()
	s.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Nothing recorded yet: no strip at all, rather than an empty one.
	if body := getFrom(t, srv, "/sessions/Mustur"); strings.Contains(body, "sub-agent") {
		t.Error("a session that has launched nothing still claims sub-agents")
	}

	rec := func(payload map[string]any, at time.Time) {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		session.RecordHookEvent(dir, "Mustur", b, at)
	}
	rec(map[string]any{
		"hook_event_name": "PreToolUse", "tool_name": "Agent",
		"tool_input": map[string]any{"description": "Contract reviewer", "subagent_type": "general-purpose"},
	}, now)
	rec(map[string]any{"hook_event_name": "SubagentStart", "agent_id": "a1", "agent_type": "general-purpose"}, now)
	rec(map[string]any{"hook_event_name": "PreToolUse", "agent_id": "a1", "tool_name": "Grep"}, now)
	rec(map[string]any{"hook_event_name": "SubagentStart", "agent_id": "a2", "agent_type": "Explore"}, now)
	rec(map[string]any{"hook_event_name": "SubagentStop", "agent_id": "a2", "last_assistant_message": "Nothing found."}, now.Add(time.Minute))

	body := getFrom(t, srv, "/sessions/Mustur")
	for _, want := range []string{
		"2 sub-agents",      // both of them
		"1 running",         // and only one still going
		"Contract reviewer", // the task it was launched with
		"Grep",              // what it is doing now
		"3m",                // how long it has been at it
		"finished",          // the other one
		"Nothing found.",    // and what it said
		"Explore",           // a row with no task falls back to its type
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not show %q", want)
		}
	}

	// The rows are server-rendered. This surface carries one script and it is
	// for the output stream; a sub-agent appearing is not worth a second.
	if strings.Count(body, "<script") != 1 {
		t.Errorf("%d scripts on the page, want the one that drives the socket", strings.Count(body, "<script"))
	}
}

// A surface with no hook directory is the sessions Mustur started before this
// milestone, and the ones started by hand after it. They show output and no
// rows, rather than an error.
func TestASessionWithoutTheHookShowsNoRows(t *testing.T) {
	srv := serveSessions(t, owned("mustur/Mustur"))
	if body := getFrom(t, srv, "/sessions/Mustur"); strings.Contains(body, "sub-agent") {
		t.Error("a session with no hook directory claims sub-agents")
	}
}

// Live sub-agent rows, over the socket, against real tmux.
//
// The owner chose this over a page reload on MUS-Q-0029, against the
// recommendation, so it is tested the way the rest of the socket is: a real
// session, a real handshake, and a frame that has to arrive.
func TestSubagentRowsArriveOverTheSocket(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux on PATH; this test only means something against the real thing")
	}
	dir := t.TempDir()
	a := &session.Adapter{HookDir: dir}
	project := "zzWebAgents"
	if _, err := a.Start(context.Background(), project, t.TempDir(), "sh -c 'sleep 6'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Stop(context.Background(), project) })

	hub := &session.Hub{Adapter: a, Dir: t.TempDir()}
	t.Cleanup(hub.Shutdown)
	s := &Sessions{Hub: hub, Adapter: a, Actor: "pie", HookDir: dir}
	mux := http.NewServeMux()
	s.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+
		"/sessions/"+project+"/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{srv.URL}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()

	// A sub-agent appears after the socket is already open, which is the case
	// the reload could not serve.
	now := time.Now()
	for _, p := range []map[string]any{
		{"hook_event_name": "PreToolUse", "tool_name": "Agent",
			"tool_input": map[string]any{"description": "Contract reviewer", "subagent_type": "general-purpose"}},
		{"hook_event_name": "SubagentStart", "agent_id": "a1", "agent_type": "general-purpose"},
		{"hook_event_name": "PreToolUse", "agent_id": "a1", "tool_name": "Grep"},
	} {
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		session.RecordHookEvent(dir, project, b, now)
		now = now.Add(time.Second)
	}

	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("no agents frame arrived: %v", err)
		}
		var f struct {
			T      string `json:"t"`
			Agents []struct {
				Title   string `json:"title"`
				State   string `json:"state"`
				Started int64  `json:"started"`
				For     string `json:"for"`
			} `json:"agents"`
			Running int `json:"running"`
		}
		if json.Unmarshal(data, &f) != nil || f.T != "agents" {
			continue
		}
		if len(f.Agents) != 1 {
			t.Fatalf("%d rows in the frame, want 1", len(f.Agents))
		}
		if f.Agents[0].Title != "Contract reviewer" || f.Agents[0].State != "Grep" {
			t.Errorf("row is %+v", f.Agents[0])
		}
		if f.Running != 1 {
			t.Errorf("running %d, want 1", f.Running)
		}
		if f.Agents[0].Started == 0 {
			t.Error("no start stamp; the client counts the age from it")
		}
		if f.Agents[0].For != "" {
			t.Error("a rendered age was sent as well as the stamp, so the two can disagree")
		}
		return
	}
}
