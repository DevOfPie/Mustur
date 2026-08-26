package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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

// A retry of the POST is what a phone on a flaky connection actually sends.
// Post-redirect-get protects a reload after the 303 and nothing before it, so
// three identical posts became three records.
func TestARetriedPostDoesNotFileTwice(t *testing.T) {
	srv, s := serve(t)
	for i := 0; i < 3; i++ {
		res, err := http.PostForm(srv.URL+"/intake", url.Values{"jot": {"the same thought, sent three times"}})
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	findings, err := s.List(context.Background(), "finding")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("%d records for one jot sent three times", len(findings))
	}
}

// Two different jots inside the window are two jots. The dedup is on the text,
// not on the minute.
func TestDifferentJotsInTheWindowAreBothFiled(t *testing.T) {
	srv, s := serve(t)
	for _, jot := range []string{"first thought", "second thought"} {
		res, err := http.PostForm(srv.URL+"/intake", url.Values{"jot": {jot}})
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	findings, err := s.List(context.Background(), "finding")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("%d records for two different jots", len(findings))
	}
}

// What was typed comes back on the error path. Losing it is the one failure
// this surface cannot have, and the comment saying so used to sit above code
// that dropped it — the page had no field for the text at all.
func TestTheBoxComesBackHoldingWhatWasTyped(t *testing.T) {
	var b strings.Builder
	const typed = "a long thumb-typed paragraph that must not vanish"
	if err := tmpl.Execute(&b, page{Error: "the store said no", Jot: typed, Project: "MUS"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), typed) {
		t.Fatalf("the textarea came back empty:\n%s", b.String())
	}
	if !strings.Contains(b.String(), "the store said no") {
		t.Error("the page does not say why it was refused")
	}
}

func TestAnEmptyJotSaysSo(t *testing.T) {
	srv, _ := serve(t)
	res, err := http.PostForm(srv.URL+"/intake", url.Values{"jot": {"  \n  "}})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Not filed") {
		t.Errorf("the page does not say it was refused:\n%s", body)
	}
}

func TestAnOversizedJotIsRefusedRatherThanStored(t *testing.T) {
	srv, s := serve(t)
	res, err := http.PostForm(srv.URL+"/intake", url.Values{"jot": {strings.Repeat("x", MaxJot+1024)}})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	findings, err := s.List(context.Background(), "finding")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("an oversized jot was stored: %d record(s)", len(findings))
	}
}

// A jot filed from a phone reached the store and nothing else. The findings
// role is mapped at the exported file, so until the surface exported, "lands in
// Mustur's findings-queue" was true of the database and not of the thing the
// audit reads.
func TestFilingExportsWhenAskedTo(t *testing.T) {
	srv, s := serve(t)
	dir := t.TempDir()
	in := &Intake{Store: s, Project: "MUS", Actor: "pie", ExportTo: dir}
	exporting := httptest.NewServer(in.Handler())
	defer exporting.Close()

	if _, err := http.PostForm(exporting.URL+"/intake", url.Values{"jot": {"a jot that must reach the file"}}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "findings.md"))
	if err != nil {
		t.Fatalf("the findings role's file was not written: %v", err)
	}
	if !strings.Contains(string(body), "a jot that must reach the file") {
		t.Errorf("the export does not carry the jot:\n%s", body)
	}
	_ = srv
}

// Without a directory the surface writes the store and nothing else, which is
// the right default for a server that is not sitting on a checkout.
func TestFilingExportsNothingByDefault(t *testing.T) {
	srv, _ := serve(t)
	res, err := http.PostForm(srv.URL+"/intake", url.Values{"jot": {"no export configured"}})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusSeeOther {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestTheBoxOffersTheDestinations(t *testing.T) {
	srv, _ := serve(t)
	body := get(t, srv.URL+"/intake")
	for _, want := range []string{"Route it for me", "DevOfPie/Mustur", "Idea inbox", `name="to"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the box does not offer %q:\n%s", want, body)
		}
	}
}

func TestPickingADestinationOverridesTheGuess(t *testing.T) {
	srv, s := serve(t)
	// The text names the repository; the form says the inbox.
	res, err := http.PostForm(srv.URL+"/intake",
		url.Values{"jot": {"mustur should log slow queries"}, "to": {"MUS-P-0002"}})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	findings, err := s.List(context.Background(), "finding")
	if err != nil || len(findings) != 1 {
		t.Fatalf("findings %d: %v", len(findings), err)
	}
	if to, _ := findings[0].Get("Routed to"); !strings.Contains(to, "MUS-P-0002") {
		t.Errorf("routed to %q despite an explicit choice", to)
	}
}

// An explicit destination that is not in the registry is refused, and the box
// comes back holding what was typed.
func TestAnUnknownDestinationComesBackWithTheText(t *testing.T) {
	srv, s := serve(t)
	res, err := http.PostForm(srv.URL+"/intake",
		url.Values{"jot": {"a thought worth keeping"}, "to": {"MUS-R-9999"}})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "a thought worth keeping") {
		t.Errorf("what was typed did not come back:\n%s", body)
	}
	findings, err := s.List(context.Background(), "finding")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("%d record(s) written for a refused destination", len(findings))
	}
}

// The file button says a tap registered.
//
// MUS-F-0024, filed from a phone while the owner was proving milestone 2c: the
// button looked identical before and after a tap. A phone has no hover, so
// :active is the half that matters and is asserted first.
func TestTheFileButtonHasAPressState(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	in := &Intake{Store: st, Project: "MUS", Actor: "pie"}
	mux := http.NewServeMux()
	mux.Handle("/", in.Handler())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := getFrom(t, srv, "/intake")
	for _, want := range []string{"button:active", "button:hover", "button:focus-visible"} {
		if !strings.Contains(body, want) {
			t.Errorf("the file button has no %s rule, so a tap says nothing", want)
		}
	}
}

// An identifier on the intake surface goes to its record.
//
// The owner asked for it and the reason is plain: a jot's id is the thing you
// want next, and it was sitting there as text to be retyped somewhere else.
func TestFiledIdentifiersLinkToTheirRecord(t *testing.T) {
	srv, _ := serve(t)
	defer srv.Close()

	res, err := srv.Client().PostForm(srv.URL+"/intake", url.Values{"jot": {"a thing worth noting"}})
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	page := string(body)

	// The confirmation carries the identifier, and it is a link to the record.
	if !strings.Contains(page, `href="/records/MUS-F-`) {
		t.Error("the filed identifier is not a link to its record")
	}
	// And so does the recent list, which is where you look a minute later.
	again, err := srv.Client().Get(srv.URL + "/intake")
	if err != nil {
		t.Fatal(err)
	}
	listBody, err := io.ReadAll(again.Body)
	again.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(listBody), `class="rec" href="/records/`); n < 1 {
		t.Errorf("the recent list has %d linked identifiers, want at least 1", n)
	}
}
