package store

import (
	"context"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

var intakeBox = []Renaming{{"IDW-F-0001", "_IB-F-0001"}, {"IDW-F-0004", "_IB-F-0004"}}

// renameFixture is MUS-D-0192's shape in miniature: two jots under IDW, one of
// them amended, a record citing them in a ref, a data field and a body — its
// own history citing them too — and the decision recording the rename, which
// is kept.
func renameFixture(t *testing.T) (*Store, context.Context) {
	t.Helper()
	s, ctx, _ := open(t)
	jot := func(id, body string) record.Record {
		return record.Record{ID: id, Kind: "finding", Title: "a jot", At: "2026-09-01", Body: body}
	}
	mustAppend := func(r record.Record, op string) {
		t.Helper()
		if err := s.Append(ctx, r, op, "test"); err != nil {
			t.Fatal(err)
		}
	}
	mustAppend(jot("IDW-F-0001", "first"), "create")
	mustAppend(jot("IDW-F-0004", "fourth"), "create")
	mustAppend(jot("IDW-F-0004", "fourth, corrected; see IDW-F-0001"), "amend")
	mustAppend(jot("IDW-F-0005", "not renamed"), "create")
	// A longer lookalike beside a real citation: only the whole one is renamed.
	mustAppend(jot("XIB-F-0001", "IDW-F-00010 is not IDW-F-0001's neighbour"), "create")

	cites := record.Record{ID: "MUS-D-0010", Kind: "decision", Title: "Cites IDW-F-0004", At: "2026-09-02",
		Body: "Was in IDW-F-0001.",
	}
	mustAppend(cites, "create")
	cites.Body = "IDW-F-0004 and [the first](findings.md#idw-f-0001), not IDW-F-0005."
	cites.Refs = []record.Field{{Key: "Evidence", Value: "IDW-F-0004"}}
	cites.Data = []record.Field{{Key: "Routed to", Value: "Idea inbox (IDW-F-0001)"}, {Key: "Status", Value: "open"}}
	mustAppend(cites, "amend")

	mustAppend(record.Record{ID: "MUS-D-0192", Kind: "decision", Title: "IDW-F-0001 becomes _IB-F-0001", At: "2026-09-18",
		Body: "IDW-F-0001 and IDW-F-0004 are renamed."}, "create")

	if _, err := s.Attach(ctx, "IDW-F-0004", pngBytes(t), "test"); err != nil {
		t.Fatal(err)
	}
	return s, ctx
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	// The smallest PNG the sniffer accepts: signature, IHDR, IEND.
	return []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0, 0, 0, 13, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89,
		0, 0, 0, 0, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
	}
}

func dump(t *testing.T, s *Store) string {
	t.Helper()
	rows, err := s.db.Query(`SELECT seq, record_id, payload FROM record_event ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var seq int
		var id, p string
		if err := rows.Scan(&seq, &id, &p); err != nil {
			t.Fatal(err)
		}
		b.WriteString(id + " " + p + "\n")
	}
	var att string
	if err := s.db.QueryRow(`SELECT group_concat(record_id) FROM attachment`).Scan(&att); err != nil {
		t.Fatal(err)
	}
	return b.String() + att
}

func TestADryRunWritesNothingAndSaysWhatItWould(t *testing.T) {
	s, ctx := renameFixture(t)
	before := dump(t, s)
	report, err := s.Rename(ctx, intakeBox, []string{"MUS-D-0192"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Applied {
		t.Error("a dry run says it applied")
	}
	if after := dump(t, s); after != before {
		t.Errorf("a dry run wrote:\n%s\nwas:\n%s", after, before)
	}
	// Two events of IDW-F-0004, one of IDW-F-0001, two of MUS-D-0010, the one
	// of XIB-F-0001 that cites IDW-F-0001 beside a longer lookalike, and the
	// attachment.
	if got := report.Records(); got != 4 {
		t.Errorf("%d records would change, want 4: %+v", got, report.Changes)
	}
	var events, attachments int
	for _, c := range report.Changes {
		switch c.Table {
		case "record_event":
			events++
		case "attachment":
			attachments++
		}
	}
	if events != 6 || attachments != 1 {
		t.Errorf("%d events and %d attachments, want 6 and 1: %+v", events, attachments, report.Changes)
	}
	if len(report.Kept) != 1 || report.Kept[0].Record != "MUS-D-0192" {
		t.Errorf("kept %+v, want MUS-D-0192's one event", report.Kept)
	}
	if _, err := s.Get(ctx, "IDW-F-0001"); err != nil {
		t.Errorf("a dry run moved IDW-F-0001: %v", err)
	}
}

func TestARenameReachesEveryEventAndEveryCitation(t *testing.T) {
	s, ctx := renameFixture(t)
	report, err := s.Rename(ctx, intakeBox, []string{"MUS-D-0192"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Applied {
		t.Fatal("not applied")
	}
	for _, gone := range []string{"IDW-F-0001", "IDW-F-0004"} {
		if _, err := s.Get(ctx, gone); err == nil {
			t.Errorf("%s still resolves", gone)
		}
		if h, _ := s.History(ctx, gone); len(h) != 0 {
			t.Errorf("%s still has %d event(s) in the log", gone, len(h))
		}
	}
	// The whole history moved, not only the latest.
	h, err := s.History(ctx, "_IB-F-0004")
	if err != nil || len(h) != 2 {
		t.Fatalf("_IB-F-0004 has %d event(s), want 2: %v", len(h), err)
	}
	if h[0].Record.ID != "_IB-F-0004" || h[1].Record.Body != "fourth, corrected; see _IB-F-0001" {
		t.Errorf("history not rewritten: %+v", h)
	}
	cites, err := s.Get(ctx, "MUS-D-0010")
	if err != nil {
		t.Fatal(err)
	}
	if cites.Title != "Cites _IB-F-0004" {
		t.Errorf("title %q", cites.Title)
	}
	if want := "_IB-F-0004 and [the first](findings.md#_ib-f-0001), not IDW-F-0005."; cites.Body != want {
		t.Errorf("body %q, want %q", cites.Body, want)
	}
	if v, _ := cites.Get("Routed to"); v != "Idea inbox (_IB-F-0001)" {
		t.Errorf("data field %q", v)
	}
	if len(cites.Refs) != 1 || cites.Refs[0].Value != "_IB-F-0004" {
		t.Errorf("refs %+v", cites.Refs)
	}
	ch, _ := s.History(ctx, "MUS-D-0010")
	if len(ch) != 2 || ch[0].Record.Body != "Was in _IB-F-0001." {
		t.Errorf("an earlier event of a citing record was not rewritten: %+v", ch)
	}
	// Kept as written.
	kept, _ := s.Get(ctx, "MUS-D-0192")
	if kept.Title != "IDW-F-0001 becomes _IB-F-0001" || kept.Body != "IDW-F-0001 and IDW-F-0004 are renamed." {
		t.Errorf("a kept record was rewritten: %+v", kept)
	}
	// Untouched neighbours.
	if r, _ := s.Get(ctx, "XIB-F-0001"); r.Body != "IDW-F-00010 is not _IB-F-0001's neighbour" {
		t.Errorf("a longer identifier was rewritten, or a whole one missed: %q", r.Body)
	}
	if _, err := s.Get(ctx, "IDW-F-0005"); err != nil {
		t.Errorf("IDW-F-0005 was not in the map and moved: %v", err)
	}
	// The picture followed its record.
	if a, _ := s.Attachments(ctx, "_IB-F-0004"); len(a) != 1 {
		t.Errorf("_IB-F-0004 has %d attachment(s), want 1", len(a))
	}
	// The log is still insert-only afterwards.
	if _, err := s.db.Exec(`UPDATE record_event SET actor = 'x'`); err == nil || !strings.Contains(err.Error(), "insert-only") {
		t.Errorf("the insert-only guard did not come back: %v", err)
	}
	if n, _ := s.Count(ctx); n != 6 {
		t.Errorf("record_latest holds %d, want 6", n)
	}
	// A serial after the rename continues the renamed sequence.
	if next, _ := s.NextID(ctx, "_IB", "F"); next != "_IB-F-0005" {
		t.Errorf("next _IB serial %s", next)
	}
}

func TestASecondRunRefusesAndSaysItWasApplied(t *testing.T) {
	s, ctx := renameFixture(t)
	if _, err := s.Rename(ctx, intakeBox, []string{"MUS-D-0192"}, true); err != nil {
		t.Fatal(err)
	}
	before := dump(t, s)
	_, err := s.Rename(ctx, intakeBox, []string{"MUS-D-0192"}, true)
	if err == nil || !strings.Contains(err.Error(), "already been applied") {
		t.Fatalf("a second run gave %v", err)
	}
	if dump(t, s) != before {
		t.Error("a refused second run wrote")
	}
}

func TestARenameRefuses(t *testing.T) {
	s, ctx := renameFixture(t)
	before := dump(t, s)
	for _, c := range []struct {
		name    string
		renames []Renaming
		keep    []string
		want    string
	}{
		{"missing old", []Renaming{{"IDW-F-0009", "_IB-F-0009"}}, nil, "does not exist"},
		{"existing new", []Renaming{{"IDW-F-0001", "IDW-F-0005"}}, nil, "already exists"},
		{"role change", []Renaming{{"IDW-F-0001", "_IB-D-0001"}}, nil, "never changes what a record is"},
		{"same old twice", []Renaming{{"IDW-F-0001", "_IB-F-0001"}, {"IDW-F-0001", "_IB-F-0002"}}, nil, "renamed twice"},
		{"same new twice", []Renaming{{"IDW-F-0001", "_IB-F-0001"}, {"IDW-F-0004", "_IB-F-0001"}}, nil, "both become"},
		{"chain", []Renaming{{"IDW-F-0001", "IDW-F-0009"}, {"IDW-F-0009", "_IB-F-0009"}}, nil, "chain or a swap"},
		{"swap", []Renaming{{"IDW-F-0001", "IDW-F-0004"}, {"IDW-F-0004", "IDW-F-0001"}}, nil, "chain or a swap"},
		{"not an identifier", []Renaming{{"IDW-F-1", "_IB-F-0001"}}, nil, "not PROJECT-ROLE-SERIAL"},
		{"nothing", nil, nil, "nothing to rename"},
		{"kept and renamed", intakeBox, []string{"IDW-F-0001"}, "both renamed and kept"},
		{"kept unknown", intakeBox, []string{"MUS-D-0999"}, "no such record"},
	} {
		_, err := s.Rename(ctx, c.renames, c.keep, true)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
	if dump(t, s) != before {
		t.Error("a refused rename wrote")
	}
}

// A body line that starts with the identifier sits after an escaped newline in
// the stored JSON, and has to be found anyway.
func TestARenameFindsAnIdentifierAtTheStartOfALine(t *testing.T) {
	s, ctx := renameFixture(t)
	r := record.Record{ID: "MUS-F-0001", Kind: "finding", Title: "lines", At: "2026-09-03",
		Body: "first line\nIDW-F-0001 opens this one"}
	if err := s.Append(ctx, r, "create", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rename(ctx, intakeBox, []string{"MUS-D-0192"}, true); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(ctx, "MUS-F-0001")
	if got.Body != "first line\n_IB-F-0001 opens this one" {
		t.Errorf("body %q", got.Body)
	}
}

// held_jot is a table some binaries create and this schema does not. Where it
// exists, a held jot's destination and text follow the rename; so does a
// scratch filing's text.
func TestARenameReachesHeldJotsAndScratchWhereTheyExist(t *testing.T) {
	s, ctx := renameFixture(t)
	if _, err := s.db.Exec(`CREATE TABLE held_jot (id TEXT PRIMARY KEY, text TEXT NOT NULL,
		destination TEXT NOT NULL DEFAULT '', account_id TEXT NOT NULL, created TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO held_jot VALUES ('h1', 'more on IDW-F-0004', 'IDW-F-0001', 'a', 'now'),
		('h2', 'nothing to see', 'MUS-P-0002', 'a', 'now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO scratch VALUES ('s1', 'test of IDW-F-0001', 'now', 'a')`); err != nil {
		t.Fatal(err)
	}
	report, err := s.Rename(ctx, intakeBox, []string{"MUS-D-0192"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for _, c := range report.Changes {
		if c.Table == "held_jot" || c.Table == "scratch" {
			tables = append(tables, c.Table+" "+c.Row+" "+strings.Join(c.Where, ","))
		}
	}
	if got := strings.Join(tables, "; "); got != "held_jot h1 destination,text; scratch s1 text" {
		t.Errorf("reported %q", got)
	}
	var text, dest, scratch string
	if err := s.db.QueryRow(`SELECT text, destination FROM held_jot WHERE id = 'h1'`).Scan(&text, &dest); err != nil {
		t.Fatal(err)
	}
	if text != "more on _IB-F-0004" || dest != "_IB-F-0001" {
		t.Errorf("held jot %q -> %q", text, dest)
	}
	if err := s.db.QueryRow(`SELECT text FROM scratch WHERE id = 's1'`).Scan(&scratch); err != nil {
		t.Fatal(err)
	}
	if scratch != "test of _IB-F-0001" {
		t.Errorf("scratch %q", scratch)
	}
}
