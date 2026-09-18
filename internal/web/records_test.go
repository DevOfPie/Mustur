package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

func serveRecords(t *testing.T, home string, recs ...record.Record) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for _, r := range recs {
		if err := st.Append(ctx, r, "create", "test"); err != nil {
			t.Fatal(err)
		}
	}
	rr := &Records{Store: st, Project: "MUS", Home: home}
	mux := http.NewServeMux()
	rr.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func fetch(t *testing.T, srv *httptest.Server, path string) (string, int) {
	t.Helper()
	res, err := srv.Client().Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	// ReadAll, not one Read: a single Read returns whatever the first chunk
	// held, and the page outgrew it once the markdown CSS arrived.
	b, _ := io.ReadAll(res.Body)
	return string(b), res.StatusCode
}

func decision(id, title, body string, refs ...record.Field) record.Record {
	return record.Record{ID: id, Kind: "decision", Title: title, At: "2026-08-20", Body: body, Refs: refs}
}

// rows counts the index's lines, each of which is one record.
func rows(body string) int { return strings.Count(body, `class="row"`) }

// The index is a list, and a record is still a page (MUS-D-0186, amending
// MUS-D-0040). Before it, /records rendered every record in full with every
// citation resolved — 11,413,213 bytes for 2,144 records on a copy of the live
// store on 2026-09-15 (MUS-F-0164).
func TestTheIndexIsOneLinePerRecordAndRendersNoBodies(t *testing.T) {
	srv := serveRecords(t, "",
		decision("MUS-D-0001", "The first decision", "Something was decided, as MUS-D-0002 says."),
		decision("MUS-D-0002", "The second decision", "So was this.",
			record.Field{Key: "corrects", Value: "MUS-D-0001"}),
		record.Record{ID: "MUS-F-0001", Kind: "finding", Title: "A finding", At: "2026-08-21"},
	)
	body, code := fetch(t, srv, "/records")
	if code != http.StatusOK {
		t.Fatalf("records returned %d", code)
	}
	if got := rows(body); got != 3 {
		t.Errorf("%d rows, want one per record", got)
	}
	for _, want := range []string{
		`href="/records/MUS-D-0001"`, "The first decision", "A finding",
		"3 records · newest first",
		`<form class="narrow" method="get" action="/records"`,
		`name="project"`, `name="kind"`, `name="q"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the index does not show %q", want)
		}
	}
	for _, never := range []string{"Something was decided", "So was this", "<details>", "corrects:", "<article"} {
		if strings.Contains(body, never) {
			t.Errorf("the index rendered %q, which belongs on the record's own page", never)
		}
	}
	// Nothing chosen, so nothing to clear.
	if strings.Contains(body, ">Clear<") {
		t.Error("Clear is offered with nothing chosen")
	}
	// The form is a plain GET and adds no script: the bar's stays the only
	// one (MUS-Q-0053, MUS-Q-0078).
	if got := scriptsIn(body); len(got) != 1 || got[0] != "/assets/bar.js" {
		t.Errorf("the records page loads %v, want only the bar's script", got)
	}
}

// Newest first by date, then by identifier descending.
func TestTheIndexIsNewestFirst(t *testing.T) {
	srv := serveRecords(t, "",
		record.Record{ID: "MUS-D-0009", Kind: "decision", Title: "Old", At: "2026-08-01"},
		record.Record{ID: "LNK-F-0001", Kind: "finding", Title: "New, low", At: "2026-09-15"},
		record.Record{ID: "MUS-F-0002", Kind: "finding", Title: "New, high", At: "2026-09-15"},
	)
	body, _ := fetch(t, srv, "/records")
	a, b, c := strings.Index(body, "MUS-F-0002"), strings.Index(body, "LNK-F-0001"), strings.Index(body, "MUS-D-0009")
	if a < 0 || b < 0 || c < 0 || !(a < b && b < c) {
		t.Errorf("order is not newest first then identifier descending: %d %d %d", a, b, c)
	}
}

// Project is the identifier's prefix and kind is the record's kind. Each picker
// says how many records its choice holds, the kinds are those within the chosen
// project, and the line above the list names what is chosen.
func TestProjectAndKindNarrowTheIndex(t *testing.T) {
	srv := serveRecords(t, "",
		record.Record{ID: "MUS-P-0004", Kind: "project", Title: "LinkCtrl", At: "2026-09-13",
			Data: []record.Field{{Key: "Prefix", Value: "LNK"}}},
		record.Record{ID: "LNK-S-0001", Kind: "phase", Title: "Phase 1", At: "2026-09-13"},
		record.Record{ID: "LNK-S-0002", Kind: "phase", Title: "Phase 2", At: "2026-09-13"},
		record.Record{ID: "LNK-D-0001", Kind: "decision", Title: "A LinkCtrl decision", At: "2026-09-13"},
		decision("MUS-D-0001", "A Mustur decision", ""),
		record.Record{ID: "MUS-F-0001", Kind: "finding", Title: "A Mustur finding", At: "2026-08-21"},
	)
	body, _ := fetch(t, srv, "/records?project=LNK&kind=phase")
	if got := rows(body); got != 2 {
		t.Errorf("LinkCtrl phases: %d rows, want 2", got)
	}
	if strings.Contains(body, "LNK-D-0001") || strings.Contains(body, "MUS-D-0001") {
		t.Error("a record outside the chosen project and kind is listed")
	}
	for _, want := range []string{
		`<option value="LNK" selected>LinkCtrl (LNK) · 3</option>`,
		`<option value="MUS">MUS · 3</option>`,
		`<option value="phase" selected>phase · 2</option>`,
		`<option value="decision">decision · 1</option>`,
		"2 records · LinkCtrl · phase",
		`<a href="/records">Clear</a>`,
		// The row names the project without the tag the identifier carries.
		`<span class="proj">LinkCtrl</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the narrowed index does not show %q", want)
		}
	}
	// The kinds are those within the project: LinkCtrl holds no finding.
	if strings.Contains(body, `value="finding"`) {
		t.Error("the kind picker offers a kind the chosen project does not hold")
	}

	// Unknown values are ignored rather than refused, so a stale bookmark still
	// shows something.
	body, code := fetch(t, srv, "/records?project=ZZZ&kind=nonsense")
	if code != http.StatusOK || rows(body) != 6 {
		t.Errorf("unknown project and kind: %d, %d rows, want 200 and every record", code, rows(body))
	}
}

// A whole identifier the store holds opens it. One it does not hold is a search
// like any other rather than a trip to a missing page.
func TestAWholeIdentifierOpensItsRecord(t *testing.T) {
	srv := serveRecords(t, "", decision("LNK-D-0131", "The identity rides beside src", ""))
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, q := range []string{"LNK-D-0131", "lnk-d-0131", "  LNK-D-0131 "} {
		res, err := client.Get(srv.URL + "/records?q=" + url.QueryEscape(q))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/records/LNK-D-0131" {
			t.Errorf("q=%q answered %d to %q, want 303 to /records/LNK-D-0131", q, res.StatusCode, res.Header.Get("Location"))
		}
	}
	res, err := client.Get(srv.URL + "/records?q=LNK-D-0999")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("an identifier the store does not hold answered %d, want the index", res.StatusCode)
	}
}

// A bare number matches identifier endings in every project and kind. Anything
// else is words in the title, whatever their case — and never the body.
func TestTheSearchBoxMatchesIdentifierEndingsAndTitles(t *testing.T) {
	srv := serveRecords(t, "",
		decision("MUS-D-0131", "The four tabs are drawings", "Nothing about ellipses."),
		record.Record{ID: "LNK-F-0131", Kind: "finding", Title: "MarkDomainVerificationFailed", At: "2026-09-13"},
		decision("MUS-D-1131", "Ends differently", ""),
		decision("MUS-D-0132", "Mentions 0131 in its title", "Tabs, tabs."),
	)
	body, _ := fetch(t, srv, "/records?q=0131")
	if got := rows(body); got != 2 {
		t.Errorf("q=0131: %d rows, want the two identifiers ending in it", got)
	}
	if !strings.Contains(body, "2 records match 0131") {
		t.Error("the line above the list does not say what was searched")
	}
	body, _ = fetch(t, srv, "/records?q=TABS")
	if got := rows(body); got != 1 || !strings.Contains(body, "MUS-D-0131") {
		t.Errorf("q=TABS: %d rows, want the one title holding it and not the body", got)
	}
	if !strings.Contains(body, "1 record matches TABS") {
		t.Error("one result is not said in the singular: want \"1 record matches TABS\"")
	}
	body, _ = fetch(t, srv, "/records?q=ellipses")
	if got := rows(body); got != 0 || !strings.Contains(body, "No records match.") {
		t.Errorf("q=ellipses matched %d rows from a body", got)
	}
}

// Fifty rows a page, with Newer and Older links that keep what is chosen. A
// page past the end is shown empty with a way back rather than refused.
func TestTheIndexPagesAtFifty(t *testing.T) {
	var recs []record.Record
	for i := 1; i <= 120; i++ {
		recs = append(recs, record.Record{
			ID: fmt.Sprintf("MUS-F-%04d", i), Kind: "finding", Title: "Finding", At: "2026-09-01",
		})
	}
	srv := serveRecords(t, "", recs...)

	body, _ := fetch(t, srv, "/records")
	if rows(body) != 50 || !strings.Contains(body, "Page 1 of 3") ||
		!strings.Contains(body, `href="/records?page=2">Older</a>`) || !strings.Contains(body, "<span>Newer</span>") {
		t.Errorf("page 1: %d rows, or the pager is wrong", rows(body))
	}
	if !strings.Contains(body, "120 records · newest first · 1–50") {
		t.Error("page 1 does not say which rows it holds")
	}
	body, _ = fetch(t, srv, "/records?kind=finding&page=3")
	if rows(body) != 20 || !strings.Contains(body, `href="/records?kind=finding&amp;page=2">Newer</a>`) ||
		!strings.Contains(body, "<span>Older</span>") {
		t.Errorf("page 3: %d rows, or the pager dropped the kind", rows(body))
	}
	body, code := fetch(t, srv, "/records?kind=finding&page=9")
	if code != http.StatusOK || rows(body) != 0 || !strings.Contains(body, "Nothing on page 9") ||
		!strings.Contains(body, `href="/records?kind=finding&amp;page=3">Newer</a>`) {
		t.Errorf("past the end: %d, %d rows, want an empty page whose Newer is the last page", code, rows(body))
	}
	for _, bad := range []string{"0", "-2", "x"} {
		if body, _ := fetch(t, srv, "/records?page="+bad); rows(body) != 50 || !strings.Contains(body, "Page 1 of 3") {
			t.Errorf("page=%s is not read as the first page", bad)
		}
	}
}

// The original complaint: a bare identifier on screen expands in one action,
// with no round trip. On the record's own page, which is where the document
// reading lives now.
func TestACitationExpandsInPlace(t *testing.T) {
	srv := serveRecords(t, "",
		decision("MUS-D-0001", "The cited one", "The thing that was decided first."),
		decision("MUS-D-0002", "The citing one", "This corrects MUS-D-0001 in one respect.",
			record.Field{Key: "corrects", Value: "MUS-D-0001"}),
	)
	body, _ := fetch(t, srv, "/records/MUS-D-0002")

	// The cited record's title is already on the page, inside the expandable,
	// so opening it costs no request.
	if !strings.Contains(body, "corrects: MUS-D-0001") {
		t.Error("the named citation is not rendered as one")
	}
	if strings.Count(body, "The cited one") < 1 {
		t.Error("the cited record's title is not carried inside the citation, so expanding it would need a round trip")
	}
	if !strings.Contains(body, "<details>") || !strings.Contains(body, "<summary") {
		t.Error("citations are not expandable without script")
	}
}

// Identifiers written in prose are citations too — that is where most of this
// tree's cross-references actually live.
func TestAnIdentifierInProseIsACitation(t *testing.T) {
	srv := serveRecords(t, "",
		decision("MUS-D-0001", "The cited one", "First."),
		decision("MUS-D-0009", "Mentions one in passing", "As MUS-D-0001 already said, and unlike MUS-D-9999."),
	)
	body, _ := fetch(t, srv, "/records/MUS-D-0009")

	if !strings.Contains(body, "The cited one") {
		t.Error("an identifier in prose was not resolved")
	}
	// And one that resolves to nothing says so rather than rendering an empty
	// box, because a dangling citation is a defect worth seeing.
	if !strings.Contains(body, "MUS-D-9999") {
		t.Error("an unknown identifier in prose was dropped")
	}
	if !strings.Contains(body, "Nothing in the store has this identifier") {
		t.Error("a dangling citation renders as if it resolved")
	}
}

// Every record addressable by identifier, which is what makes one pasteable.
func TestARecordHasItsOwnURL(t *testing.T) {
	srv := serveRecords(t, "", decision("MUS-D-0007", "On its own", "The body."))

	body, code := fetch(t, srv, "/records/MUS-D-0007")
	if code != http.StatusOK {
		t.Fatalf("the canonical URL returned %d", code)
	}
	if !strings.Contains(body, "On its own") || !strings.Contains(body, "The body.") {
		t.Error("the record is not on its own page")
	}
	// Lower case works, because an identifier gets typed.
	if _, code := fetch(t, srv, "/records/mus-d-0007"); code != http.StatusOK {
		t.Errorf("a lower-cased identifier returned %d", code)
	}
	body, code = fetch(t, srv, "/records/MUS-D-9999")
	if code != http.StatusNotFound {
		t.Errorf("an identifier that is not here returned %d, want 404", code)
	}
	if !strings.Contains(body, "No record called MUS-D-9999") {
		t.Error("a missing record does not say which one")
	}
}

// The routing surface verifies rather than repeats: a checkout that moved reads
// as stale on the row itself.
func TestARoutingRowIsVerifiedAgainstTheMachine(t *testing.T) {
	home := t.TempDir()
	// One repository that is where it says, with its contract file.
	real := filepath.Join(home, "repos", "Real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "workflow.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// One whose contract file is gone, and one whose checkout is not there.
	noContract := filepath.Join(home, "repos", "NoContract")
	if err := os.MkdirAll(noContract, 0o755); err != nil {
		t.Fatal(err)
	}

	repo := func(id, title, path string) record.Record {
		return record.Record{
			ID: id, Kind: "repository", Title: title, At: "2026-08-19",
			Data: []record.Field{
				{Key: "Checkout on MUS-H-0001", Value: path},
				{Key: "Contract", Value: "workflow.md"},
			},
		}
	}
	srv := serveRecords(t, home,
		repo("MUS-R-0001", "DevOfPie/Real", "~/repos/Real"),
		repo("MUS-R-0002", "DevOfPie/NoContract", "~/repos/NoContract"),
		repo("MUS-R-0003", "DevOfPie/Gone", "~/repos/Gone"),
	)
	// On each record's own page: the index lists one line a record and
	// verifies nothing.
	var body string
	for _, id := range []string{"MUS-R-0001", "MUS-R-0002", "MUS-R-0003"} {
		page, _ := fetch(t, srv, "/records/"+id)
		body += page
	}

	if !strings.Contains(body, "there") {
		t.Error("a checkout that is where it says does not read as there")
	}
	if !strings.Contains(body, "stale — no workflow.md") {
		t.Error("a missing contract file does not read as stale")
	}
	if !strings.Contains(body, "stale — nothing at ~/repos/Gone") {
		t.Error("a checkout that is not there does not read as stale")
	}
	// The stale ones are marked as such, not merely described.
	if strings.Count(body, "badge stale") != 2 {
		t.Errorf("%d rows marked stale, want 2", strings.Count(body, "badge stale"))
	}
}

// A decision cannot be stale in the routing sense, and must not be given a
// badge that implies it was checked.
func TestOnlyRoutingRowsAreVerified(t *testing.T) {
	srv := serveRecords(t, t.TempDir(), decision("MUS-D-0001", "A decision", "Body."))
	body, _ := fetch(t, srv, "/records/MUS-D-0001")
	if strings.Contains(body, "badge stale") || strings.Contains(body, ">there<") {
		t.Error("a decision was given a verification badge")
	}
}

// A ref field may name several records, and each is its own citation.
//
// Looking the whole value up as one identifier rendered eleven perfectly good
// citations as dangling on the first run against the real store — which reads
// as a finding about the tree until somebody looks.
func TestARefFieldMayNameSeveralRecords(t *testing.T) {
	srv := serveRecords(t, "",
		decision("MUS-D-0002", "The first cited", "One."),
		decision("MUS-D-0008", "The second cited", "Two."),
		decision("MUS-D-0027", "The third cited", "Three."),
		record.Record{
			ID: "MUS-W-0001", Kind: "work-unit", Title: "A unit", At: "2026-08-20",
			Refs: []record.Field{
				{Key: "Decided by", Value: "MUS-D-0002, MUS-D-0008, MUS-D-0027"},
				{Key: "Method", Value: "docs/ingress.md"},
			},
		},
	)
	body, _ := fetch(t, srv, "/records/MUS-W-0001")

	for _, want := range []string{"The first cited", "The second cited", "The third cited"} {
		if !strings.Contains(body, want) {
			t.Errorf("%q was not resolved from a multi-value ref", want)
		}
	}
	if strings.Contains(body, "Nothing in the store has this identifier") {
		t.Error("a multi-value ref rendered as dangling")
	}
	// A ref that is not an identifier is shown as written rather than looked up
	// and reported missing.
	if !strings.Contains(body, "docs/ingress.md") {
		t.Error("a ref naming a file was dropped")
	}
}

// Every piece of record text a long token can appear in has somewhere to break.
//
// MUS-F-0033 gave field values that and left titles, bodies and summaries
// alone. It held until a record body carried a pasted terminal box — an
// unbroken run of box-drawing characters — which set the width of the document
// and took the fixed tab bar off the bottom of the screen with it. Measured in
// a headless browser at 390px: 625px of document before, 390 after, and the bar
// back on the first screen (MUS-F-0131).
func TestRecordTextCanBreakWhereverItHasTo(t *testing.T) {
	srv := serveRecords(t, "", decision("MUS-D-0001", "A decision", "Decided."))
	css, code := fetch(t, srv, "/records")
	if code != http.StatusOK {
		t.Fatalf("records returned %d", code)
	}
	for _, rule := range []string{"article h3", "article p", "summary"} {
		i := strings.Index(css, rule+" {")
		if i < 0 {
			t.Errorf("no %s rule on the records page", rule)
			continue
		}
		block := css[i : i+strings.Index(css[i:], "}")]
		if !strings.Contains(block, "overflow-wrap") {
			t.Errorf("%s has no break opportunity: %s", rule, block)
		}
	}
}
