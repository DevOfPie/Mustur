package web

// The records surface acting on attention (MUS-D-0193): the pinned section on
// the index, the banner on a record, the Move and Keep presses, and the line
// in the intake box.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/intake"
	"github.com/DevOfPie/Mustur/internal/store"
)

// attendingServer serves the records surface over st, behind the guard when
// guarded is set.
func attendingServer(t *testing.T, st *store.Store, guarded bool) (*httptest.Server, *Auth) {
	t.Helper()
	mux := http.NewServeMux()
	rr := &Records{Store: st, Project: "MUS", Actor: "configured"}
	rr.Routes(mux)
	if !guarded {
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		return srv, nil
	}
	auth := &Auth{Accounts: account.New(st.DB()), Origin: "http://127.0.0.1"}
	auth.Routes(mux)
	rr.Auth = auth
	srv := httptest.NewServer((&Guard{Auth: auth, Project: "MUS"}).Wrap(mux))
	t.Cleanup(srv.Close)
	return srv, auth
}

// press posts a form as a browser would, with the Origin given, and does not
// follow the redirect.
func press(t *testing.T, c *http.Client, srv *httptest.Server, path, origin string, form url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res
}

// The pinned section is there whatever the filters say, names where each
// record would go, sits above the filters, and is absent when nothing needs
// attention. The row in the ordinary list carries the dot.
func TestThePinnedSectionIgnoresTheFilters(t *testing.T) {
	st, id := attending(t)
	srv, _ := attendingServer(t, st, false)
	for _, path := range []string{"/records", "/records?kind=decision", "/records?q=nothing-matches", "/records?page=9"} {
		body := bodyOf(t, srv.Client(), srv.URL+path)
		if !strings.Contains(body, "Needs attention · 1") {
			t.Errorf("%s has no pinned section", path)
			continue
		}
		if !strings.Contains(body, `href="/records/`+id+`"`) || !strings.Contains(body, "names Archive — move?") {
			t.Errorf("%s does not link %s with what it names", path, id)
		}
		if strings.Index(body, `class="attn"`) > strings.Index(body, `class="narrow"`) {
			t.Errorf("%s puts the section below the filters", path)
		}
	}
	body := bodyOf(t, srv.Client(), srv.URL+"/records")
	if !strings.Contains(body, `<span class="dot" title="Needs attention" aria-label="Needs attention"></span><span class="id">`+id) {
		t.Error("the row in the ordinary list carries no dot")
	}

	if _, err := intake.Keep(context.Background(), st, id, "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	body = bodyOf(t, srv.Client(), srv.URL+"/records")
	if strings.Contains(body, `class="attn"`) || strings.Contains(body, `class="dot"`) {
		t.Error("the section or the dot is drawn with nothing needing attention")
	}
}

// An owner is offered the two buttons; a reader sees the banner and no button.
// With no accounts at all, every viewer is the owner.
func TestTheBannerOffersButtonsOnlyToAnOwner(t *testing.T) {
	st, id := attending(t)
	srv, auth := attendingServer(t, st, true)
	owner := signedInAs(t, srv, auth.Accounts, "owner@example.com", "MUS", account.Owner)
	reader := signedInAs(t, srv, auth.Accounts, "reader@example.com", "MUS", account.Reader)

	const banner = "This jot names Archive, which takes a jot only when a move is confirmed."
	move := `action="/records/` + id + `/move"`
	keep := `action="/records/` + id + `/keep"`

	body := bodyOf(t, owner, srv.URL+"/records/"+id)
	for _, want := range []string{banner, move, keep, "Move to Archive", "Keep in intake box", `name="to" value="MUS-P-0003"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the owner's page lacks %q", want)
		}
	}
	if strings.Index(body, `class="banner"`) > strings.Index(body, "<h3>") {
		t.Error("the banner is below the title")
	}
	body = bodyOf(t, reader, srv.URL+"/records/"+id)
	if !strings.Contains(body, banner) {
		t.Error("a reader is not shown the banner")
	}
	if strings.Contains(body, move) || strings.Contains(body, keep) {
		t.Error("a reader is offered a button that answers 403")
	}

	open, _ := attendingServer(t, st, false)
	if body := bodyOf(t, open.Client(), open.URL+"/records/"+id); !strings.Contains(body, move) {
		t.Error("with no accounts the owner lost the buttons")
	}
}

// Move is reroute, through the same function, by the signed-in owner. The new
// record says so, the old one points at it, and nothing needs attention.
func TestMoveFilesItWhereItNamed(t *testing.T) {
	st, id := attending(t)
	srv, auth := attendingServer(t, st, true)
	owner := signedInAs(t, srv, auth.Accounts, "owner@example.com", "MUS", account.Owner)

	res := press(t, owner, srv, "/records/"+id+"/move", srv.URL, url.Values{"to": {"MUS-P-0003"}})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("move answered %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.HasPrefix(loc, "/records/ARC-F-") || !strings.HasSuffix(loc, "?moved="+id) {
		t.Fatalf("move sent the browser to %q", loc)
	}
	body := bodyOf(t, owner, srv.URL+loc)
	if !strings.Contains(body, "Moved to Archive. "+id+" is kept and points here.") {
		t.Error("the moved record does not say so")
	}
	if strings.Contains(body, `class="banner"`) {
		t.Error("the moved record still asks to be moved")
	}

	ctx := context.Background()
	old, err := st.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	fresh := strings.TrimSuffix(strings.TrimPrefix(loc, "/records/"), "?moved="+id)
	if v, _ := old.Get(intake.SupersededBy); !strings.HasPrefix(v, fresh) || !strings.Contains(v, "owner@example.com confirmed it") {
		t.Errorf("%s = %q", intake.SupersededBy, v)
	}
	if n := intake.AttentionCount(ctx, st); n != 0 {
		t.Errorf("AttentionCount = %d after the move", n)
	}

	// The success line is the moved record's to say. Another record given the
	// same address says nothing.
	if body := bodyOf(t, owner, srv.URL+"/records/MUS-P-0002?moved="+id); strings.Contains(body, "Moved to") {
		t.Error("a record the move never touched claims it")
	}
}

// Keep leaves it where it is and says who kept it.
func TestKeepLeavesItAndSaysWho(t *testing.T) {
	st, id := attending(t)
	srv, auth := attendingServer(t, st, true)
	owner := signedInAs(t, srv, auth.Accounts, "owner@example.com", "MUS", account.Owner)

	res := press(t, owner, srv, "/records/"+id+"/keep", srv.URL, nil)
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/records/"+id+"?kept=1" {
		t.Fatalf("keep answered %d to %q", res.StatusCode, res.Header.Get("Location"))
	}
	rec, err := st.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := rec.Get(intake.KeptField); !strings.HasPrefix(v, "owner@example.com ") {
		t.Errorf("%s = %q, want the signed-in owner", intake.KeptField, v)
	}
	body := bodyOf(t, owner, srv.URL+"/records/"+id+"?kept=1")
	if !strings.Contains(body, "Kept in the intake box.") || strings.Contains(body, `class="banner"`) {
		t.Error("the kept record does not say so, or still asks")
	}
}

// What is refused, and that a refusal changes nothing.
func TestMoveAndKeepRefuse(t *testing.T) {
	st, id := attending(t)
	srv, auth := attendingServer(t, st, true)
	owner := signedInAs(t, srv, auth.Accounts, "owner@example.com", "MUS", account.Owner)
	reader := signedInAs(t, srv, auth.Accounts, "reader@example.com", "MUS", account.Reader)
	to := url.Values{"to": {"MUS-P-0003"}}

	for _, action := range []string{"move", "keep"} {
		path := "/records/" + id + "/" + action
		if res := press(t, reader, srv, path, srv.URL, to); res.StatusCode != http.StatusForbidden {
			t.Errorf("a reader's %s answered %d, want 403", action, res.StatusCode)
		}
		if res := press(t, owner, srv, path, "", to); res.StatusCode != http.StatusForbidden {
			t.Errorf("an owner's %s with no Origin answered %d, want 403", action, res.StatusCode)
		}
		if res := press(t, owner, srv, path, "https://elsewhere.example", to); res.StatusCode != http.StatusForbidden {
			t.Errorf("a cross-site %s answered %d, want 403", action, res.StatusCode)
		}
		// A record that names nothing has nothing to move or keep.
		if res := press(t, owner, srv, "/records/MUS-P-0002/"+action, srv.URL, to); res.StatusCode != http.StatusConflict {
			t.Errorf("%s of a record naming nothing answered %d, want 409", action, res.StatusCode)
		}
	}
	// Only somewhere it names.
	if res := press(t, owner, srv, "/records/"+id+"/move", srv.URL, url.Values{"to": {"MUS-P-0002"}}); res.StatusCode != http.StatusBadRequest {
		t.Errorf("a move somewhere it does not name answered %d, want 400", res.StatusCode)
	}
	if res := press(t, owner, srv, "/records/NOPE-F-0001/move", srv.URL, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("a move of nothing answered %d, want 404", res.StatusCode)
	}

	// Without the guard in front, the handler's own check still holds a
	// reader.
	mux := http.NewServeMux()
	(&Records{Store: st, Project: "MUS"}).Routes(mux)
	asReader := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(withRole(r.Context(), account.Reader)))
	}))
	t.Cleanup(asReader.Close)
	if res := press(t, asReader.Client(), asReader, "/records/"+id+"/keep", asReader.URL, nil); res.StatusCode != http.StatusForbidden {
		t.Errorf("a reader past no guard kept a record: %d", res.StatusCode)
	}

	rec, err := st.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !intake.NeedsAttention(rec, "MUS-P-0002") {
		t.Error("a refused press changed the record")
	}
}

// The intake box says how many records need attention, beside the decisions
// line, and says nothing when none do.
func TestTheIntakeBoxSaysWhatNeedsAttention(t *testing.T) {
	st, id := attending(t)
	srv := httptest.NewServer((&Intake{Store: st, Project: "MUS", Actor: "pie"}).Handler())
	t.Cleanup(srv.Close)
	const line = `<a href="/records">1 record needs attention</a>`
	if body := bodyOf(t, srv.Client(), srv.URL+"/intake"); !strings.Contains(body, line) {
		t.Error("the intake box does not say a record needs attention")
	}
	if _, err := intake.Keep(context.Background(), st, id, "owner", time.Now()); err != nil {
		t.Fatal(err)
	}
	if body := bodyOf(t, srv.Client(), srv.URL+"/intake"); strings.Contains(body, "need attention") || strings.Contains(body, "needs attention") {
		t.Error("the intake box speaks with nothing needing attention")
	}
}

// A press forgets the held count, so the page it lands on and bar.js's first
// poll there agree. Found in a browser: after Move the page rendered 1 and the
// poll put 2 back from the cache.
func TestAPressIsSeenByTheNextPoll(t *testing.T) {
	st, id := attending(t)
	srv, _ := attendingServer(t, st, false)
	poll := func() string {
		t.Helper()
		return strings.TrimSpace(bodyOf(t, srv.Client(), srv.URL+"/records/attention/count"))
	}
	if got := poll(); got != `{"waiting":1}` {
		t.Fatalf("before: %s", got)
	}
	nf := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if res := press(t, nf, srv, "/records/"+id+"/keep", srv.URL, nil); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("keep answered %d", res.StatusCode)
	}
	if got := poll(); got != `{"waiting":0}` {
		t.Errorf("the poll straight after a keep said %s", got)
	}
}
