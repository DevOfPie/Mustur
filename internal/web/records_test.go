package web

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	b := make([]byte, 65536)
	n, _ := res.Body.Read(b)
	return string(b[:n]), res.StatusCode
}

func decision(id, title, body string, refs ...record.Field) record.Record {
	return record.Record{ID: id, Kind: "decision", Title: title, At: "2026-08-20", Body: body, Refs: refs}
}

// A document to read: the counts are the navigation, and every record is on the
// one page rather than behind a link.
func TestTheRecordsPageIsOneDocumentWithCountsForNavigation(t *testing.T) {
	srv := serveRecords(t, "",
		decision("MUS-D-0001", "The first decision", "Something was decided."),
		decision("MUS-D-0002", "The second decision", "So was this."),
		record.Record{ID: "MUS-F-0001", Kind: "finding", Title: "A finding", At: "2026-08-21"},
	)
	body, code := fetch(t, srv, "/records")
	if code != http.StatusOK {
		t.Fatalf("records returned %d", code)
	}
	for _, want := range []string{
		"2 decisions", "1 finding", // counts, and the singular is not "1 findings"
		"The first decision", "The second decision", "A finding",
		`href="#decision"`, `href="#finding"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the document does not show %q", want)
		}
	}
	// No script: expansion is a details element, which the browser already
	// knows how to open.
	if strings.Contains(body, "<script") {
		t.Error("the records page carries script")
	}
}

// The original complaint: a bare identifier on screen expands in one action,
// with no round trip.
func TestACitationExpandsInPlace(t *testing.T) {
	srv := serveRecords(t, "",
		decision("MUS-D-0001", "The cited one", "The thing that was decided first."),
		decision("MUS-D-0002", "The citing one", "This corrects MUS-D-0001 in one respect.",
			record.Field{Key: "corrects", Value: "MUS-D-0001"}),
	)
	body, _ := fetch(t, srv, "/records")

	// The cited record's title is already on the page, inside the expandable,
	// so opening it costs no request.
	if !strings.Contains(body, "corrects: MUS-D-0001") {
		t.Error("the named citation is not rendered as one")
	}
	if strings.Count(body, "The cited one") < 2 {
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
	body, _ := fetch(t, srv, "/records")

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
	body, _ := fetch(t, srv, "/records")

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
	body, _ := fetch(t, srv, "/records")
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
