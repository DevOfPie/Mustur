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
	"time"

	"github.com/DevOfPie/Mustur/internal/intake"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

func serve(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, r := range []record.Record{
		{ID: "MUS-R-0001", Kind: "repository", Title: "DevOfPie/Mustur", At: "2026-08-20"},
		{ID: "MUS-P-0002", Kind: "project", Title: "Idea inbox", At: "2026-08-20",
			Data: []record.Field{{Key: intake.DefaultField, Value: intake.DefaultValue}}},
	} {
		if err := s.Append(ctx, r, "create", "test"); err != nil {
			t.Fatal(err)
		}
	}
	in := &Intake{Store: s, Project: "MUS", Actor: "pie"}
	srv := httptest.NewServer(in.Handler())
	t.Cleanup(srv.Close)
	return srv, s
}

// The whole surface in one test: type a line, press the button, and it is
// filed. Nothing was chosen, nothing was named.
func TestFilingTakesOnlyTheText(t *testing.T) {
	srv, s := serve(t)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.PostForm(srv.URL+"/intake", url.Values{"jot": {"mustur should keep a draft across a dropped connection"}})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	// Post, redirect, get. A phone reloading after a dropped connection must
	// not file the same jot twice.
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("status %d, want 303", res.StatusCode)
	}
	location, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	filed := location.Query().Get("filed")
	if filed == "" {
		t.Fatal("the redirect does not say what was filed")
	}
	if got := location.Query().Get("routed"); got != "DevOfPie/Mustur" {
		t.Errorf("routed to %q", got)
	}

	r, err := s.Get(context.Background(), filed)
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "finding" {
		t.Errorf("filed as %s", r.Kind)
	}
	if by, _ := r.Get("Filed by"); by != "pie" {
		t.Errorf("filed by %q", by)
	}
}

func TestTheBoxShowsWhatWasJustFiled(t *testing.T) {
	srv, _ := serve(t)
	if _, err := http.PostForm(srv.URL+"/intake", url.Values{"jot": {"a thought with no home"}}); err != nil {
		t.Fatal(err)
	}
	body := get(t, srv.URL+"/intake")
	if !strings.Contains(body, "a thought with no home") {
		t.Errorf("the recent list does not carry the jot:\n%s", body)
	}
	if !strings.Contains(body, "Idea inbox") {
		t.Errorf("the recent list does not say where it went:\n%s", body)
	}
}

// The recency window is read from the log's own timestamps, so a record dated
// today but written a week ago is not "recent".
func TestOnlyTheRecentIsShown(t *testing.T) {
	srv, s := serve(t)
	if _, err := http.PostForm(srv.URL+"/intake", url.Values{"jot": {"filed just now"}}); err != nil {
		t.Fatal(err)
	}
	in := &Intake{Store: s, Project: "MUS", Actor: "pie",
		Now: func() time.Time { return time.Now().Add(2 * time.Hour) }}
	later := httptest.NewServer(in.Handler())
	defer later.Close()
	body := get(t, later.URL+"/intake")
	if strings.Contains(body, "filed just now") {
		t.Errorf("an hour-old jot is still on the surface:\n%s", body)
	}
	if !strings.Contains(body, "Nothing filed") {
		t.Errorf("an empty surface does not say it is empty:\n%s", body)
	}
}

// What was typed is not lost because the store complained. That is the one
// failure a capture surface cannot have.
func TestAnEmptyJotIsRefusedWithoutRedirecting(t *testing.T) {
	srv, _ := serve(t)
	res, err := http.PostForm(srv.URL+"/intake", url.Values{"jot": {"   "}})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want the page back", res.StatusCode)
	}
}

// Cloudflare Access puts the authenticated identity in a header at the edge.
// Until it is in front of this, the configured actor is who it is — and either
// way the record says which.
func TestTheEdgeIdentityIsUsedWhenItIsThere(t *testing.T) {
	srv, s := serve(t)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/intake",
		strings.NewReader(url.Values{"jot": {"from the phone"}}.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cf-Access-Authenticated-User-Email", "dev@killerofpie.com")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	findings, err := s.List(context.Background(), "finding")
	if err != nil || len(findings) != 1 {
		t.Fatalf("findings %d: %v", len(findings), err)
	}
	if by, _ := findings[0].Get("Filed by"); by != "dev@killerofpie.com" {
		t.Errorf("filed by %q, not the edge identity", by)
	}
}

func TestTheRootRedirectsToTheBox(t *testing.T) {
	srv, _ := serve(t)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/intake" {
		t.Errorf("root gave %d -> %q", res.StatusCode, res.Header.Get("Location"))
	}
}

func get(t *testing.T, url string) string {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
