package web

// Records needing attention (MUS-D-0193): the count, and the badge on every
// surface that draws the bar.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/intake"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

// attending is a store holding one jot that needs attention: it names Archive,
// which takes jots only on a confirmed move, and fell to the idea inbox.
func attending(t *testing.T) (*store.Store, string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for _, r := range []record.Record{
		{ID: "MUS-P-0002", Kind: "project", Title: "Idea inbox", At: "2026-08-20",
			Data: []record.Field{{Key: intake.DefaultField, Value: intake.DefaultValue}, {Key: intake.PrefixField, Value: "IDW"}}},
		{ID: "MUS-P-0003", Kind: "project", Title: "Archive", At: "2026-09-18",
			Data: []record.Field{{Key: intake.OptOutField, Value: intake.OptOutValue}, {Key: intake.PrefixField, Value: "ARC"}}},
	} {
		if err := st.Append(ctx, r, "create", "test"); err != nil {
			t.Fatal(err)
		}
	}
	jot, _, err := intake.File(ctx, st, intake.Request{
		Project: "MUS", Text: "archive the old intake notes", Actor: "pie", Now: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !intake.NeedsAttention(jot) {
		t.Fatalf("the fixture jot %s does not need attention", jot.ID)
	}
	return st, jot.ID
}

// A number and nothing else, shaped like the decisions count, and cached the
// same way: a change inside the two seconds is not seen, one after it is.
func TestTheAttentionCountIsANumber(t *testing.T) {
	st, id := attending(t)
	clock := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	rr := &Records{Store: st, Project: "MUS", Now: func() time.Time { return clock }}
	mux := http.NewServeMux()
	rr.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ask := func() int {
		t.Helper()
		res, err := srv.Client().Get(srv.URL + "/records/attention/count")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("Content-Type %q", ct)
		}
		if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
			t.Errorf("Cache-Control %q", cc)
		}
		var got map[string]int
		if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Errorf("the count says more than a number: %v", got)
		}
		return got["waiting"]
	}
	if n := ask(); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
	if _, err := intake.Keep(context.Background(), st, id, "owner", clock); err != nil {
		t.Fatal(err)
	}
	if n := ask(); n != 1 {
		t.Errorf("count = %d inside the cache window, want the cached 1", n)
	}
	clock = clock.Add(3 * time.Second)
	if n := ask(); n != 0 {
		t.Errorf("count = %d after keeping, want 0", n)
	}
}

const attentionBadge = `<em class="cnt att">`

// Every surface that draws the bar renders the Records badge server-side, so a
// page with script blocked still shows it.
func TestTheRecordsBadgeIsOnEveryBarSurface(t *testing.T) {
	st, _ := attending(t)
	mux := http.NewServeMux()
	mux.Handle("/intake", (&Intake{Store: st, Project: "MUS", Actor: "test"}).Handler())
	(&Questions{Store: st, Project: "MUS", Actor: "test"}).Routes(mux)
	(&Records{Store: st, Project: "MUS"}).Routes(mux)
	(&Compose{Store: st, Project: "MUS", Actor: "test"}).Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	for _, path := range []string{"/intake", "/questions", "/records", "/compose"} {
		body := bodyOf(t, srv.Client(), srv.URL+path)
		if !strings.Contains(body, attentionBadge+"1</em>") {
			t.Errorf("%s does not render the Records badge", path)
		}
	}

	// The two that need a hub or an account to serve, rendered from their
	// templates with the count set.
	for name, run := range map[string]func(*bytes.Buffer) error{
		"sessions": func(b *bytes.Buffer) error { return sessionTmpl.Execute(b, sessionPage{Attention: 2}) },
		"account":  func(b *bytes.Buffer) error { return accountTmpl.Execute(b, accountPage{Attention: 2}) },
	} {
		var b bytes.Buffer
		if err := run(&b); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(b.String(), attentionBadge+"2</em>") {
			t.Errorf("the %s page does not render the Records badge", name)
		}
	}

	// And absent, not empty, when nothing needs attention.
	empty, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { empty.Close() })
	mux = http.NewServeMux()
	(&Records{Store: empty, Project: "MUS"}).Routes(mux)
	quiet := httptest.NewServer(mux)
	t.Cleanup(quiet.Close)
	if body := bodyOf(t, quiet.Client(), quiet.URL+"/records"); strings.Contains(body, attentionBadge) {
		t.Error("the Records badge is drawn with nothing needing attention")
	}
}

// A bar added later cannot forget the badge: every Records tab in the source
// carries it, on the same line.
func TestEveryRecordsTabCarriesTheBadge(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(src), "\n") {
			if !strings.Contains(line, `aria-label="Records"`) {
				continue
			}
			seen++
			if !strings.Contains(line, `{{if .Attention}}`+attentionBadge+`{{.Attention}}</em>{{end}}`) {
				t.Errorf("%s draws a Records tab without the attention badge", f)
			}
		}
	}
	if seen < 6 {
		t.Errorf("found %d Records tabs, want the six surfaces that draw the bar", seen)
	}
}
