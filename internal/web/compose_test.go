package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/session"
	"github.com/DevOfPie/Mustur/internal/store"
)

// activeFake reports sessions with activity timestamps, so the ordering the
// composer's default depends on can be tested without waiting on real sessions
// to go quiet.
type activeFake struct{ listing string }

func (f activeFake) Run(_ context.Context, _ string, args ...string) (string, error) {
	if len(args) > 0 && args[0] == "list-sessions" {
		return f.listing, nil
	}
	return "", nil
}

// active builds the five-column line List parses, with an activity epoch.
func active(name string, at time.Time) string {
	return name + "\t1\t0\t1\t" + itoa(at.Unix())
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// post sends a form the way a browser does, with an Origin. The surface refuses
// a post that will not say where it came from, for the reason the session
// socket does: this path types into a running agent.
func post(t *testing.T, srv *httptest.Server, form url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/compose", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", srv.URL)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func serveCompose(t *testing.T, listing string, withInbox bool) (*httptest.Server, *store.Store) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if withInbox {
		inbox := record.Record{
			ID: "MUS-P-0002", Kind: "project", Title: "Idea inbox", At: "2026-08-20",
			Data: []record.Field{{Key: "Intake", Value: "default"}, {Key: "Prefix", Value: "IDW"}},
		}
		if err := s.Append(ctx, inbox, "create", "test"); err != nil {
			t.Fatal(err)
		}
	}
	a := &session.Adapter{Run: activeFake{listing: listing}}
	c := &Compose{
		Adapter: a, Store: s, Project: "MUS", Actor: "pie",
		Now: func() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) },
	}
	mux := http.NewServeMux()
	c.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, s
}

// The clause milestone 5 first shipped without: MUS-D-0013 asks the route row
// to default to the last active session, and the first build defaulted to
// whichever sorted first by name.
func TestTheDefaultDestinationIsTheLastActiveSession(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	// Alphabetically Aardvark comes first; by activity it is the stalest.
	srv, _ := serveCompose(t, strings.Join([]string{
		active("mustur/Aardvark", now.Add(-3*time.Hour)),
		active("mustur/Zebra", now.Add(-2*time.Minute)),
		active("mustur/Middle", now.Add(-30*time.Minute)),
	}, "\n"), false)

	body := getFrom(t, srv, "/compose")

	first := strings.Index(body, "Zebra")
	if first < 0 {
		t.Fatal("the most recently active session is not offered")
	}
	if a := strings.Index(body, "Aardvark"); a >= 0 && a < first {
		t.Error("the destinations are in name order; the default is alphabetical, not last-active")
	}
	// And it is the one pre-selected.
	chosen := body[:first]
	if strings.Count(chosen, "checked") != 0 {
		t.Error("something before the last-active session is checked")
	}
	if !strings.Contains(body[first:], "checked") {
		t.Error("the last active session is not the pre-selected destination")
	}
	if !strings.Contains(body, "active 2m ago") {
		t.Error("the row does not say how long ago the session was active")
	}
}

// The other dropped clause: the idea inbox is a route like any session, so one
// box captures both a jot and a message rather than keeping two capture
// muscles.
func TestTheIdeaInboxIsADestination(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	srv, _ := serveCompose(t, active("mustur/Mustur", now.Add(-time.Minute)), true)

	body := getFrom(t, srv, "/compose")
	if !strings.Contains(body, "Idea inbox") {
		t.Error("the idea inbox is not offered as a destination")
	}
	if !strings.Contains(body, "MUS-P-0002") {
		t.Error("the inbox route does not post its routing identifier")
	}
	// A session is still the default when one is running: the inbox is a route,
	// not a preference.
	if i, j := strings.Index(body, "Mustur"), strings.Index(body, "Idea inbox"); i > j {
		t.Error("the inbox is offered before the running sessions")
	}
}

// Composing to the inbox files a record rather than typing into anything, and
// it lands under the destination's own prefix.
func TestComposingToTheInboxFilesARecord(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	srv, st := serveCompose(t, active("mustur/Mustur", now), true)

	res := post(t, srv, url.Values{
		"text": {"a thought with no obvious home\nand a second line"},
		"to":   {"MUS-P-0002"},
	})
	defer res.Body.Close()

	findings, err := st.List(context.Background(), "finding")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("%d findings filed, want 1", len(findings))
	}
	if !strings.HasPrefix(findings[0].ID, "IDW-") {
		t.Errorf("filed as %s; a jot to the idea inbox carries the inbox's prefix", findings[0].ID)
	}
	if !strings.Contains(findings[0].Body, "and a second line") {
		t.Error("the second line did not survive into the record")
	}
}

// Thought first: the box comes before the destination row in the document, and
// the destination is a choice made after writing rather than a page you had to
// arrive on.
func TestTheBoxComesBeforeTheDestination(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	srv, _ := serveCompose(t, active("mustur/Mustur", now), true)
	body := getFrom(t, srv, "/compose")

	box := strings.Index(body, "<textarea")
	routes := strings.Index(body, `class="routes"`)
	if box < 0 || routes < 0 {
		t.Fatal("the composer has no box or no destination row")
	}
	if box > routes {
		t.Error("the destination row comes before the box; that is destination-first")
	}
	for _, want := range []string{`spellcheck="true"`, `autocapitalize="sentences"`, `autocorrect="on"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the composer is missing %s", want)
		}
	}
}

// It works with the script blocked. The draft is the only thing script buys,
// and a composer that cannot send without it would be a worse surface than the
// form it replaced.
func TestTheComposerPostsWithoutScript(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	srv, _ := serveCompose(t, active("mustur/Mustur", now), true)
	body := getFrom(t, srv, "/compose")

	if !strings.Contains(body, `<form method="post" action="/compose"`) {
		t.Error("the composer is not a form, so a browser without script cannot send")
	}
	if !strings.Contains(body, `type="radio" name="to"`) {
		t.Error("the destination is not a radio group, so it does not post without script")
	}
	if !strings.Contains(body, `<button type="submit"`) {
		t.Error("Send is not a submit button")
	}
}

// A failed send hands the text back rather than eating it, on the no-script
// path as well as the scripted one.
func TestAFailedSendReturnsTheDraft(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	// A listing with no sessions at all: the destination cannot be reached.
	srv, _ := serveCompose(t, "", true)

	res := post(t, srv, url.Values{
		"text": {"words that must not be lost"},
		"to":   {"NoSuchSession"},
	})
	defer res.Body.Close()

	body := make([]byte, 16384)
	n, _ := res.Body.Read(body)
	page := string(body[:n])

	// Rendered, not redirected: the draft used to travel in the Location URL,
	// where it lands in browser history and in the edge's logs.
	if loc := res.Header.Get("Location"); strings.Contains(loc, "draft=") {
		t.Errorf("the draft is in a redirect URL: %q", loc)
	}
	if !strings.Contains(page, "words that must not be lost") {
		t.Error("a failed send did not put the text back in the box")
	}
	if !strings.Contains(page, "Not sent:") {
		t.Error("a failed send did not say why")
	}
	_ = now
}

// A post that will not say where it came from is refused, as it is on the
// socket. This path writes into a running agent's stdin.
func TestComposePostRefusesACrossOriginForm(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	srv, _ := serveCompose(t, active("mustur/Mustur", now), true)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/compose",
		strings.NewReader(url.Values{"text": {"x"}, "to": {"Mustur"}}.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://evil.example")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("a cross-origin post returned %d, want 403", res.StatusCode)
	}
}

// A destination that is no longer there does not quietly become another one.
func TestAVanishedDestinationIsNamedRatherThanReplaced(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	srv, _ := serveCompose(t, active("mustur/Mustur", now), true)

	body := getFrom(t, srv, "/compose?to=GoneAway")
	if !strings.Contains(body, "GoneAway is no longer a destination") {
		t.Error("a destination that vanished was replaced without saying so")
	}
}

// A project name with a hyphen is a session, not a routing identifier.
//
// `strings.Contains(to, "-")` decided that, and project names admit hyphens, so
// a session called TradeShop-Support rendered as a destination and could never
// be sent to.
func TestAHyphenatedProjectIsASession(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	srv, _ := serveCompose(t, active("mustur/TradeShop-Support", now), true)

	body := getFrom(t, srv, "/compose")
	if !strings.Contains(body, "TradeShop-Support") {
		t.Fatal("a hyphenated project is not offered as a destination")
	}

	res := post(t, srv, url.Values{"text": {"a message"}, "to": {"TradeShop-Support"}})
	defer res.Body.Close()
	page := make([]byte, 16384)
	n, _ := res.Body.Read(page)
	// The fake runner cannot deliver, so this fails — but it must fail as a
	// session that could not be reached, never as an unknown routing record.
	if strings.Contains(string(page[:n]), "is not a destination this registry holds") {
		t.Error("a hyphenated project name was taken for a routing identifier")
	}
}

// Nowhere to send is its own page, not a form that always fails.
func TestNowhereToSendSaysSo(t *testing.T) {
	srv, _ := serveCompose(t, "", false)
	body := getFrom(t, srv, "/compose")
	if !strings.Contains(body, "Nowhere to send") {
		t.Error("a composer with no destinations still renders a form")
	}
	if strings.Contains(body, "<textarea") {
		t.Error("a box that cannot send anywhere is offered anyway")
	}
	if loads(body, "/assets/compose.js") {
		t.Error("the client layer loads on a page with nothing for it to do")
	}
	// The bar is still a bar, and its count is still live (MUS-Q-0078). What
	// this test is about is the composer's own client, not every script.
	if !loads(body, "/assets/bar.js") {
		t.Error("a page with no destinations still shows a bar, and its count cannot move")
	}
}

// The composer's own path into a running session, against real tmux.
//
// Nothing exercised it. `Plan.md` and the work unit both said "multi-line left
// the composer over a real WebSocket" — but the composer has no socket, it is a
// form POST, and the test they meant drives the session view's reply box. The
// composer's delivery was credited with another surface's proof, which is the
// first review pass's own finding reproduced inside the record rewritten to fix
// it.
func TestMultiLineFromTheComposeSurfaceReachesTheSession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux on PATH; this test only means something against the real thing")
	}
	dir := t.TempDir()
	got := filepath.Join(dir, "received.txt")
	a := &session.Adapter{}
	project := "zzComposeSurface"

	if _, err := a.Start(context.Background(), project, dir, "sh -c 'cat > "+got+"'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Stop(context.Background(), project) })

	c := &Compose{Adapter: a, Project: "MUS", Actor: "pie"}
	mux := http.NewServeMux()
	c.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// The destination has to be offered before it can be chosen: this is the
	// whole surface, not just its send path.
	if body := getFrom(t, srv, "/compose"); !strings.Contains(body, project) {
		t.Fatalf("the running session is not offered as a destination")
	}

	res := post(t, srv, url.Values{
		"text": {"first line from the compose surface\nsecond line a chat box would have sent\nthird line"},
		"to":   {project},
	})
	res.Body.Close()

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
		"first line from the compose surface",
		"second line a chat box would have sent",
		"third line",
	} {
		if !strings.Contains(received, want) {
			t.Errorf("%q never reached the session; it received %q", want, received)
		}
	}
	if i, j := strings.Index(received, "first line"), strings.Index(received, "third line"); i < 0 || j < 0 || i > j {
		t.Errorf("the lines arrived out of order: %q", received)
	}
	if !strings.Contains(received, "surface\nsecond") && !strings.Contains(received, "surface\r\nsecond") {
		t.Errorf("the newline between lines did not survive: %q", received)
	}
}
