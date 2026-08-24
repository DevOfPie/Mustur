package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	res, err := srv.Client().PostForm(srv.URL+"/compose", url.Values{
		"text": {"a thought with no obvious home\nand a second line"},
		"to":   {"MUS-P-0002"},
	})
	if err != nil {
		t.Fatal(err)
	}
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

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.PostForm(srv.URL+"/compose", url.Values{
		"text": {"words that must not be lost"},
		"to":   {"NoSuchSession"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "draft=") {
		t.Fatalf("a failed send did not carry the draft back: %q", loc)
	}
	if !strings.Contains(loc, "error=") {
		t.Errorf("a failed send did not say why: %q", loc)
	}
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatal(err)
	}
	if got := u.Query().Get("draft"); got != "words that must not be lost" {
		t.Errorf("the draft came back as %q", got)
	}
	_ = now
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
	if strings.Contains(body, "<script") {
		t.Error("the client layer loads on a page with nothing for it to do")
	}
}
