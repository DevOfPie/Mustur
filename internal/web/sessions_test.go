package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/record"
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
	// The whole body, not the first 8KB of it. Reading a fixed window made this
	// test a measure of how long the stylesheet above the message happened to
	// be: adding the drawer's CSS pushed the message past 8192 bytes and this
	// failed without anything about the page being wrong.
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)

	for _, want := range []string{"did not start", "will not appear"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not say %q", want)
		}
	}
}

// Two surfaces carry script, and the rest carry none.
//
// The name of this test used to be TestOnlyTheSessionSurfaceCarriesScript, and
// it went on passing after the composer was built — because it constructed a
// mux without the composer in it and asserted about the two surfaces that were
// never going to have script. A test that cannot fail for the reason its name
// gives is worse than no test: `MUS-W-0017` cited this one as proof of a claim
// the tree had stopped making.
func TestExactlyTwoSurfacesCarryScript(t *testing.T) {
	srv := serveSessions(t, owned("mustur/Mustur"))
	if !strings.Contains(getFrom(t, srv, "/sessions/Mustur"), "/assets/session.js") {
		t.Error("the session page does not load the client")
	}

	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// A destination, so the composer renders a form at all: with nowhere to
	// send it renders a notice and loads nothing, which is correct and would
	// make this test pass for the wrong reason.
	inbox := record.Record{
		ID: "MUS-P-0002", Kind: "project", Title: "Idea inbox", At: "2026-08-20",
		Data: []record.Field{{Key: "Intake", Value: "default"}, {Key: "Prefix", Value: "IDW"}},
	}
	if err := st.Append(ctx, inbox, "create", "test"); err != nil {
		t.Fatal(err)
	}

	in := &Intake{Store: st, Project: "MUS", Actor: "pie"}
	q := &Questions{Store: st, Project: "MUS", Actor: "pie"}
	// The composer is in this mux, which is the whole point: the surface that
	// went missing from the old version is the one that changed the answer.
	comp := &Compose{Store: st, Project: "MUS", Actor: "pie"}
	mux := http.NewServeMux()
	q.Routes(mux)
	comp.Routes(mux)
	mux.Handle("/", in.Handler())
	other := httptest.NewServer(mux)
	defer other.Close()

	if body := getFrom(t, other, "/compose"); !strings.Contains(body, "/assets/compose.js") {
		t.Error("the composer does not load its client layer")
	}
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
	// Four, which is MUS-D-0041's number: the owner chose four against a
	// recommendation of three, and Records was the one that did not exist yet.
	tabs := []string{`href="/sessions"`, `href="/questions"`, `href="/intake"`, `href="/records"`}

	srv := serveSessions(t, owned("mustur/Mustur"))
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	in := &Intake{Store: st, Project: "MUS", Actor: "pie", ShowSessions: true}
	q := &Questions{Store: st, Project: "MUS", Actor: "pie", ShowSessions: true}
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

// And a server that does not serve sessions offers no tab to them.
//
// The session surface can type into a running agent, so it is served only when
// asked for. A bar that pointed at it anyway would be the same defect as a tab
// for an unbuilt surface: something described as reachable that 404s.
func TestASurfaceThatIsNotServedGetsNoTab(t *testing.T) {
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
	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, path := range []string{"/questions", "/intake"} {
		body := getFrom(t, srv, path)
		if strings.Contains(body, `href="/sessions"`) {
			t.Errorf("%s offers a tab to a surface this server does not serve", path)
		}
		// The others are unaffected by the switch.
		for _, tab := range []string{`href="/questions"`, `href="/intake"`, `href="/records"`} {
			if !strings.Contains(body, tab) {
				t.Errorf("%s lost %s when sessions were turned off", path, tab)
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

	// Nothing recorded yet: the badge is absent and the drawer's count is
	// empty, rather than either of them reading zero.
	//
	// This used to search the whole page for the string "sub-agent", which made
	// it a test of every word on the surface: a resize grip whose label
	// mentioned sub-agents turned it red without anything about the page being
	// wrong. Ask the two elements that carry the claim.
	if body := getFrom(t, srv, "/sessions/Mustur"); !claimsNoSubagents(body) {
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
		"2",                 // both of them, on the badge and in the drawer's count
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

	// Identified in the list, read in the sheet (MUS-F-0038).
	//
	// The check above cannot tell the difference: it asks whether the final
	// message is anywhere on the page, and it was just as true when the whole
	// message was printed into the list and grew that box to 8,211px. So ask
	// where it is, not whether it is.
	row := between(body, `<button type="button" class="agent" data-id="a2">`, "</button>")
	if row == "" {
		t.Fatal("no row for the finished sub-agent, so nothing can open it")
	}
	if strings.Contains(row, "Nothing found.") {
		t.Error("the row still carries the final message; that is the list doing the reading")
	}
	for _, want := range []string{"finished", "Explore"} {
		if !strings.Contains(row, want) {
			t.Errorf("the row does not identify the sub-agent: no %q", want)
		}
	}
	if !strings.Contains(body, `<div class="say" data-for="a2">Nothing found.</div>`) {
		t.Error("the final message is not where the reading pane gets it from")
	}
	// And there is a drawer for a row to open into.
	for _, want := range []string{`id="drawer"`, `role="dialog"`, `id="dread"`, `id="dlist"`} {
		if !strings.Contains(body, want) {
			t.Errorf("no drawer to open: %q missing", want)
		}
	}
	// Shut on arrival. It is the whole point of moving the list off the page.
	if !strings.Contains(body, `<div class="drawer" id="drawer" hidden>`) {
		t.Error("the drawer is not shut on arrival")
	}
	// The rows are inside it, not in the column with the terminal.
	list := between(body, `<div class="dlist" id="dlist">`, "</div>\n    <div class=\"dread\"")
	if !strings.Contains(list, `data-id="a2"`) {
		t.Error("the sub-agent rows are not inside the drawer")
	}
	// The badge says what is running, and the ring turns because one is.
	if !strings.Contains(body, `class="ring live"`) {
		t.Error("a running sub-agent does not light the ring")
	}
	if !strings.Contains(body, `class="badge" id="badge">1<`) {
		t.Error("the badge does not count the running sub-agent")
	}
	// A running sub-agent has said nothing, so it has nothing to read from.
	if strings.Contains(body, `class="say" data-for="a1"`) {
		t.Error("a running sub-agent was given a final message it has not sent")
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
	if body := getFrom(t, srv, "/sessions/Mustur"); !claimsNoSubagents(body) {
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

	hub := &session.Hub{Adapter: a}
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

// The composer is a composer, not a chat box.
//
// Milestone 5's clause is multi-line, spell-checked text composed on a phone.
// A single-line input is none of those things, and the browser will not
// spell-check a field unless it is told to.
func TestTheComposerIsMultiLineAndSpellChecked(t *testing.T) {
	srv := serveSessions(t, owned("mustur/Mustur"))
	body := getFrom(t, srv, "/sessions/Mustur")

	if strings.Contains(body, `<input type="text" id="text"`) {
		t.Error("the composer is still a single-line input")
	}
	if !strings.Contains(body, `<textarea id="text"`) {
		t.Error("no textarea; multi-line is the milestone")
	}
	for _, want := range []string{
		`spellcheck="true"`,          // the browser does not do this unasked
		`autocapitalize="sentences"`, // a phone keyboard, writing prose
		`autocorrect="on"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the composer is missing %s", want)
		}
	}
}

// Thought first, destination second: the page says where what you are writing
// is going, and offers a way to change it that is not "lose the draft".
func TestTheComposerNamesItsDestination(t *testing.T) {
	srv := serveSessions(t, owned("mustur/Mustur"))
	body := getFrom(t, srv, "/sessions/Mustur")

	if !strings.Contains(body, `id="dest"`) {
		t.Error("the composer does not say where it is sending")
	}
	if !strings.Contains(body, "Send to Mustur") {
		t.Error("the destination line does not name the session")
	}
	// Changing the destination is the composer's job, not this box's: the
	// owner declined a reply box that pretends to be a router on MUS-Q-0034,
	// so this one names where it is going and links to the screen where that
	// can be changed.
	if !strings.Contains(body, `href="/compose"`) {
		t.Error("no route from the reply box to the composer")
	}
	if !strings.Contains(body, `id="kept"`) {
		t.Error("nothing tells the owner a draft is being kept, which is the whole reason this is not a chat box")
	}
}

// The draft is one thought, not one per session.
//
// The client keeps it under a single key so that changing your mind about the
// destination mid-sentence does not cost the sentence. A key that varied by
// project would lose it at exactly the moment the design exists to protect.
func TestTheDraftIsNotKeyedPerSession(t *testing.T) {
	srv := serveSessions(t, owned("mustur/Mustur"))
	js := getFrom(t, srv, "/assets/session.js")

	if !strings.Contains(js, `"mustur.draft"`) {
		t.Fatal("no draft key in the client")
	}
	// The key is a constant. If project ever gets concatenated into it, the
	// draft stops following the owner between sessions.
	for _, bad := range []string{`DRAFT + project`, `"mustur.draft." + project`, `DRAFT+project`} {
		if strings.Contains(js, bad) {
			t.Errorf("the draft key is per-session (%s), so switching sessions loses the draft", bad)
		}
	}
	// It has to survive a backgrounded phone, which never guarantees another
	// event — so it is written as the owner types, not on unload.
	if !strings.Contains(js, `text.addEventListener("input"`) {
		t.Error("the draft is not saved on input, so a backgrounded phone can lose it")
	}
	if strings.Contains(js, `"beforeunload"`) {
		t.Error("the draft relies on beforeunload, which a backgrounded phone need never fire")
	}
}

// The milestone's own sentence, minus the parts only a person can do.
//
// "The owner composes multi-line, spell-checked text from the phone, off the
// home network, without a terminal, and it reaches the intended session." The
// phone, the network and the spell-checker are the browser's and the owner's.
// What this asserts is the rest of it: multi-line text leaving the composer
// over the real socket and arriving in the intended session's input, whole.
func TestMultiLineFromTheComposerReachesTheSession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux on PATH; this test only means something against the real thing")
	}
	dir := t.TempDir()
	got := filepath.Join(dir, "received.txt")
	a := &session.Adapter{}
	project := "zzComposeE2E"

	// The hub's cleanup is registered before the session's so that it runs
	// after it: t.Cleanup is last-in-first-out, and Shutdown waits on a reader
	// whose pane is still alive, so stopping the session second hangs the test
	// rather than failing it.
	hub := &session.Hub{Adapter: a}
	t.Cleanup(hub.Shutdown)

	// Plain cat, and not `timeout N cat`: timeout puts the child in its own
	// process group without the terminal, so cat is stopped on SIGTTIN the
	// moment it reads and receives nothing at all. That cost an hour and is
	// worth a sentence. cat also ends on EOF rather than on a newline, so a
	// message that arrived in pieces still lands every piece in the file and
	// cannot pass by looking finished.
	if _, err := a.Start(context.Background(), project, dir, "sh -c 'cat > "+got+"'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Stop(context.Background(), project) })
	s := &Sessions{Hub: hub, Adapter: a, Actor: "pie"}
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

	msg := "first line of the draft\nsecond line, which a chat box would have sent already\nthird line"
	frame, err := json.Marshal(map[string]string{"t": "input", "text": msg})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Write(ctx, websocket.MessageText, frame); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(20 * time.Second)
	var received string
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(got); err == nil {
			received = string(b)
			if strings.Contains(received, "third line") {
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	for _, want := range []string{
		"first line of the draft",
		"second line, which a chat box would have sent already",
		"third line",
	} {
		if !strings.Contains(received, want) {
			t.Errorf("%q never reached the session; it received %q", want, received)
		}
	}
	// Order and the newlines themselves, because "all three arrived" is also
	// true of three lines arriving backwards or run together on one. The work
	// unit credited this test with both checks before it made either.
	if i, j := strings.Index(received, "first line"), strings.Index(received, "third line"); i < 0 || j < 0 || i > j {
		t.Errorf("the lines arrived out of order: %q", received)
	}
	if !strings.Contains(received, "draft\nsecond") && !strings.Contains(received, "draft\r\nsecond") {
		t.Errorf("the newline between lines did not survive: %q", received)
	}
}

// between returns what sits between the first open and the next close, or "".
func between(s, open, close string) string {
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	rest := s[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

// The page and the script beside it have to agree, so the page is never kept.
//
// This is the whole of MUS-F-0041. The script is revalidated on every load and
// the page was not, so a deploy could leave a reader holding markup from before
// it beside a script from after it — which is exactly how sub-agent rows became
// un-openable: rows drawn by the old markup carry no identifier, and the
// delegated handler looking for one silently finds nothing.
//
// no-store, not no-cache, because it also keeps the page out of the
// back/forward cache. bfcache restores a whole live document, script state and
// all, and a phone returning to a backgrounded tab is the common way to meet it.
func TestTheSessionPageIsNeverCached(t *testing.T) {
	srv := serveSessions(t, owned("mustur/Mustur"))
	for _, path := range []string{"/sessions", "/sessions/Mustur"} {
		res, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if got := res.Header.Get("Cache-Control"); !strings.Contains(got, "no-store") {
			t.Errorf("%s says Cache-Control: %q; a cached page outlives the script it agrees with", path, got)
		}
	}
}

// The picker's button appears only with scripting off, and the browser is what
// decides that.
//
// A GET form cannot build a path segment, so the select posts a query and
// /sessions turns it into a path; with script the change event navigates first
// and the button is not wanted, without it the button is the only way to
// submit. The first version drew it always and hid it from the script, and that
// was the defect: a control the server draws and the script removes can fail
// visible, and it did — on a stale page carrying new markup beside old script,
// at full size under the dropdown, having never been in the wireframes.
//
// noscript has neither half of that. It is resolved at parse time from whether
// scripting is enabled, so a page whose script is stale, blocked or missing
// still gets exactly the control it needs.
func TestThePickerButtonIsOnlyThereWithoutScript(t *testing.T) {
	dir := t.TempDir()
	a := &session.Adapter{Run: fakeRunner{listing: owned("mustur/Mustur")}}
	s := &Sessions{Hub: &session.Hub{Adapter: a}, Adapter: a, Actor: "pie", HookDir: dir}
	mux := http.NewServeMux()
	s.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	body := getFrom(t, srv, "/sessions/Mustur")
	if !strings.Contains(body, `</select><noscript><button type="submit" class="go">Go</button></noscript>`) {
		t.Error("the button is not inside a noscript beside the select")
	}
	// Not hidden, not conditional on anything the server knows: those are the
	// two shapes that produced the defect.
	if strings.Contains(body, `class="go" hidden`) || strings.Contains(body, `id="go"`) {
		t.Error("the button is shipped hidden or given a handle for the script to grab")
	}

	// Nothing may set display on the noscript.
	//
	// It had display: contents, so the button would be a flex item of the row
	// rather than the noscript being one. That override also cancels the rule
	// every browser applies with scripting enabled — noscript { display: none }
	// — and the contents of a noscript with scripting on are its own markup as
	// text. The row rendered the literal opening button tag beside the
	// dropdown, in every browser, for anyone with script.
	if strings.Contains(body, ".pick noscript {") {
		t.Error("the noscript's display is overridden, which shows its markup as text when scripting is on")
	}

	// And the script does not reach for it. If the server renders it, the
	// server — or here, the browser's own noscript — decides whether it is
	// there.
	js, err := os.ReadFile("assets/session.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{`getElementById("go")`, "go.hidden", `querySelector(".go")`} {
		if strings.Contains(string(js), banned) {
			t.Errorf("the script still touches the button: %q", banned)
		}
	}

	// The picker form has to undo the bare form rule written for the composer.
	//
	// That rule is a bare element selector — form { display: flex;
	// flex-direction: column } with its own padding — so it reshapes every form
	// added after it. This one inherited column and came out stacked and
	// centred inside 69px of nothing, which is precisely the giant button under
	// the dropdown that was reported. Overriding display alone is not enough,
	// because .pick never mentioned direction or padding at all.
	at := strings.Index(body, ".pick { display: flex;")
	if at < 0 {
		t.Fatal("no .pick rule")
	}
	rule := body[at : at+strings.Index(body[at:], "}")]
	for _, want := range []string{"flex-direction: row", "padding: 0"} {
		if !strings.Contains(rule, want) {
			t.Errorf("the picker row does not reset %q, so the composer's form rule reshapes it:\n%s", want, rule)
		}
	}
	// What actually puts the button beside the select is the row being a flex
	// row, asserted above. An earlier version of this test demanded
	// `display: contents` on the noscript as well, which is the thing that made
	// its markup show as text — a test holding a defect in place.

	// The query it submits has to land on the session it names, or the button
	// is decoration for the one reader who needs it.
	res, err := srv.Client().Get(srv.URL + "/sessions?p=Mustur")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.Request.URL.Path != "/sessions/Mustur" {
		t.Errorf("?p= landed on %q, not the session it names", res.Request.URL.Path)
	}
}

// The quiet timer measures the session, not the tab.
//
// The web layer declared a quiet counter, left it at zero and sent it — and
// omitempty meant zero was not sent at all, so the browser's "if the server
// told me" branch never ran and it started counting from whenever the tab
// happened to attach. Opening a second tab on a session silent for an hour said
// "quiet 0s" (MUS-F-0042). Stream.Quiet had existed since the stream did and
// nothing reached it.
//
// So this attaches to a session that has been silent for a beat and asks what
// the hello frame says. A second is enough: the defect reported zero forever,
// not a value one tick out.
func TestTheHelloFrameSaysHowLongTheSessionHasBeenQuiet(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux on PATH; this test only means something against the real thing")
	}
	dir := t.TempDir()
	a := &session.Adapter{HookDir: dir}
	project := "zzQuiet"
	if _, err := a.Start(context.Background(), project, t.TempDir(), "sh -c 'echo working; sleep 5'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Stop(context.Background(), project) })

	hub := &session.Hub{Adapter: a}
	t.Cleanup(hub.Shutdown)
	s := &Sessions{Hub: hub, Adapter: a, Actor: "pie", HookDir: dir}
	mux := http.NewServeMux()
	s.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dial := func() frame {
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
		_, b, err := c.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var f frame
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatal(err)
		}
		if f.T != "hello" {
			t.Fatalf("first frame is %q, not the hello this reads", f.T)
		}
		return f
	}

	// Quiet before anybody is watching, which is the case this is for and the
	// case the first version of this test did not cover.
	//
	// That version dialled once to open the reader, waited, and dialled again —
	// so the stream had seen output and lastAt was set. It passed while the
	// live surface still read "quiet 0s" on a session silent for days, because
	// a reader that has just opened has seen nothing and lastAt was zero. The
	// first viewer is the one who needs the answer.
	time.Sleep(1500 * time.Millisecond)

	first := dial()
	if first.Quiet < 1 {
		t.Errorf("the first viewer of a session idle for over a second is told quiet=%d; nothing seeded it from tmux", first.Quiet)
	}

	// And it keeps working once the reader is running, which is the half that
	// already worked.
	time.Sleep(1200 * time.Millisecond)
	second := dial()
	if second.Quiet < first.Quiet {
		t.Errorf("quiet went backwards, %d then %d", first.Quiet, second.Quiet)
	}
}

// claimsNoSubagents reports whether the strip and the drawer both say there are
// none — the badge absent rather than zero, and the drawer's count empty.
func claimsNoSubagents(body string) bool {
	if !strings.Contains(body, `id="badge" hidden`) {
		return false
	}
	if !strings.Contains(body, `<small class="count" id="dcount"></small>`) {
		return false
	}
	// And no rows to open.
	return !strings.Contains(body, `class="agent" data-id=`)
}

// The drawer can be dragged wider, and the handle is a control.
//
// IDW-F-0004, from the owner: the drawer can take more space on a laptop and
// dragging it wider would be nice when wanted. The behaviour itself is measured
// in a browser — a drag is not something markup can prove — so what this holds
// is the part that quietly rots: that the handle stays reachable without a
// pointer, and that it does not appear on a phone where there is nothing to
// widen into.
func TestTheDrawerHasAResizeHandleThatIsNotPointerOnly(t *testing.T) {
	dir := t.TempDir()
	a := &session.Adapter{Run: fakeRunner{listing: owned("mustur/Mustur")}}
	s := &Sessions{Hub: &session.Hub{Adapter: a}, Adapter: a, Actor: "pie", HookDir: dir}
	mux := http.NewServeMux()
	s.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	body := getFrom(t, srv, "/sessions/Mustur")
	// A button, so it is focusable and reachable by keyboard without anything
	// being added to make it so. A div with a pointer handler is the shape this
	// is deliberately not.
	if !strings.Contains(body, `<button type="button" class="grip" id="grip" role="separator"`) {
		t.Error("the resize handle is not a focusable control")
	}
	for _, want := range []string{`aria-orientation="vertical"`, `aria-label="Resize the drawer"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the handle does not say what it is: %q missing", want)
		}
	}

	// Hidden by default and shown only above the breakpoint. On a phone the
	// drawer is most of the screen already.
	at := strings.Index(body, ".grip { display: none; }")
	if at < 0 {
		t.Error("the handle is not hidden below the breakpoint")
	}
	wide := strings.Index(body, "@media (min-width: 60rem)")
	if wide < 0 || wide < at {
		t.Error("the handle is not brought back inside the wide-screen block")
	}
	if !strings.Contains(body[wide:], "cursor: col-resize") {
		t.Error("the handle does not read as draggable on a wide screen")
	}
}

// The first frame carries the screen, rendered, and no escape reaches the page.
//
// This is what replaced the backlog. There is no offset to resume from and no
// replay to distinguish, because the unit is a whole screen: hello carries the
// pane as it stands, and every frame after it carries the pane again
// (MUS-Q-0060).
func TestTheSocketSendsARenderedScreen(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux on PATH; this test only means something against the real thing")
	}
	dir := t.TempDir()
	a := &session.Adapter{HookDir: dir}
	project := "zzScreen"
	if _, err := a.Start(context.Background(), project, t.TempDir(),
		"sh -c 'printf \"\\033[31mred-and-angry\\033[0m\\n\"; sleep 12'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Stop(context.Background(), project) })

	hub := &session.Hub{Adapter: a}
	t.Cleanup(hub.Shutdown)
	s := &Sessions{Hub: hub, Adapter: a, Actor: "pie", HookDir: dir}
	mux := http.NewServeMux()
	s.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	time.Sleep(900 * time.Millisecond)

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

	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var f frame
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	if f.T != "hello" {
		t.Fatalf("the first frame is %q, not hello", f.T)
	}
	if !strings.Contains(f.Screen, "red-and-angry") {
		t.Errorf("hello carries no screen:\n%s", f.Screen)
	}
	// Rendered rather than raw. An escape on the page is the whole defect.
	if strings.Contains(f.Screen, "\x1b") || strings.Contains(f.Screen, "[31m") {
		t.Errorf("an escape survived into the frame:\n%q", f.Screen)
	}
	// The colour became markup rather than being dropped or printed.
	if !strings.Contains(f.Screen, "<span style=\"color:") {
		t.Errorf("the colour was lost rather than rendered:\n%s", f.Screen)
	}
	// And nothing about a stream is left in the protocol.
	if strings.Contains(string(b), `"seq"`) || strings.Contains(string(b), `"replay"`) ||
		strings.Contains(string(b), `"lostBytes"`) {
		t.Errorf("the frame still describes a byte stream:\n%s", b)
	}
}

// Enter sends, and a phone keeps its newline.
//
// MUS-F-0067 asked for Enter to send and Shift+Enter to break the line.
// MUS-Q-0067 settled what that means where there is no shift key: on a touch
// screen Enter stays a newline and the Send button is the submit, because a
// soft keyboard has no modifier and Enter-sends-everywhere would take
// multi-line off the surface this box exists for.
func TestEnterSendsOnlyWhereThereIsAShiftKeyToHold(t *testing.T) {
	srv := serveSessions(t, owned("mustur/Mustur"))
	js := getFrom(t, srv, "/assets/session.js")

	if !strings.Contains(js, `"(hover: hover) and (pointer: fine)"`) {
		t.Fatal("nothing asks whether a physical keyboard is present, so Enter behaves the same on a phone as on a desktop")
	}
	// Read at each keystroke, not cached: a tablet that gains a keyboard
	// should change with it.
	if !strings.Contains(js, "deskKeys.matches") {
		t.Error("the query result is not read at the keystroke")
	}
	if !strings.Contains(js, "e.shiftKey") {
		t.Error("Shift+Enter is not let through, so a desktop cannot write a second line")
	}
	// The modifier shortcut predates this and still works everywhere, which is
	// the only way to send from a touch screen without reaching for the button.
	if !strings.Contains(js, "e.metaKey || e.ctrlKey") {
		t.Error("Cmd/Ctrl+Enter no longer sends")
	}
	// An IME candidate is chosen with Enter. Sending on it eats the word.
	if !strings.Contains(js, "e.isComposing") {
		t.Error("an IME's Enter would send a half-typed word")
	}
	// The button is not conditional on anything: a control that comes and goes
	// with a media query is a control nobody trusts.
	body := getFrom(t, srv, "/sessions/Mustur")
	if !strings.Contains(body, `<button type="submit">Send</button>`) {
		t.Error("the Send button is gone or conditional; the owner's answer was that it is always present")
	}
}

// The turning ring goes around the word, not across it.
//
// MUS-F-0068: the status pill sat inside .ring with no position and no
// background, so the conic gradient -- absolutely positioned, and 12.5% alpha
// showing through a transparent fill -- painted over the text as its two bright
// arms came round. The sub-agent toggle beside it was opaque and positioned and
// never had the fault, which is why it showed up on one control and not both.
func TestTheTurningRingDoesNotPaintOverTheStatusPill(t *testing.T) {
	srv := serveSessions(t, owned("mustur/Mustur"))
	body := getFrom(t, srv, "/sessions/Mustur")

	// Above the gradient. An unpositioned box loses to an absolutely
	// positioned pseudo-element whatever the DOM order.
	if !strings.Contains(body, "header .ring > .pill { position: relative;") {
		t.Error("the status pill is not stacked above the ring's gradient")
	}
	// And opaque, or the light shows through the fill as well as over it.
	if !strings.Contains(body, "background: var(--paper); }") {
		t.Error("the status pill has no opaque background, so the gradient comes through it")
	}
	// The on state is the one that turns, so it is the one that must not go
	// back to a bare translucent token.
	if !strings.Contains(body, "header .ring > .pill.on { background:") {
		t.Error("the running pill still takes --accent-soft alone, which is 12.5% alpha")
	}
}

