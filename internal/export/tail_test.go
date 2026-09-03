package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

func decision(id, title string) record.Record {
	return record.Record{ID: id, Kind: "decision", Title: title, At: "2026-09-03", Body: "why"}
}

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "decisions.md")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The hand-written half is untouched and the generated half is everything the
// marker names onward.
func TestTailKeepsTheProseAndRewritesTheRest(t *testing.T) {
	p := write(t, "# Log\n\nprose nobody generated\n\n<!-- mustur:generated from=MUS-D-0002 -->\n\nstale\n")
	recs := []record.Record{
		decision("MUS-D-0001", "written by hand, above the line"),
		decision("MUS-D-0002", "the first generated one"),
		decision("MUS-D-0003", "and the next"),
		{ID: "MUS-F-0001", Kind: "finding", Title: "not a decision", At: "2026-09-03"},
	}
	if err := Tail(p, recs); err != nil {
		t.Fatal(err)
	}
	got := read(t, p)

	if !strings.Contains(got, "prose nobody generated") {
		t.Error("the hand-written half was lost")
	}
	if strings.Contains(got, "stale") {
		t.Error("what was below the marker survived, so the tail is appended rather than rewritten")
	}
	if strings.Contains(got, "MUS-D-0001") {
		t.Error("a decision before the marker's from= was generated, so the hand-written half is duplicated")
	}
	if strings.Contains(got, "MUS-F-0001") {
		t.Error("a finding reached the decision log")
	}
	for _, want := range []string{"### MUS-D-0002", "### MUS-D-0003"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %s in the tail", want)
		}
	}
	if strings.Index(got, "MUS-D-0002") > strings.Index(got, "MUS-D-0003") {
		t.Error("the tail is not in identifier order")
	}
}

// Running it twice changes nothing, which is what makes it safe to put in a
// target somebody runs without thinking.
func TestTailIsIdempotent(t *testing.T) {
	p := write(t, "# Log\n\n<!-- mustur:generated from=MUS-D-0001 -->\n")
	recs := []record.Record{decision("MUS-D-0001", "one")}
	if err := Tail(p, recs); err != nil {
		t.Fatal(err)
	}
	once := read(t, p)
	if err := Tail(p, recs); err != nil {
		t.Fatal(err)
	}
	if again := read(t, p); again != once {
		t.Errorf("a second run changed the file:\n--- once\n%s\n--- again\n%s", once, again)
	}
}

// A file with no marker is not a file this may rewrite. The alternative is
// guessing where the prose ends, and guessing wrong deletes it.
func TestTailRefusesAFileWithNoMarker(t *testing.T) {
	p := write(t, "# Log\n\nall of this is prose\n")
	before := read(t, p)
	err := Tail(p, []record.Record{decision("MUS-D-0001", "one")})
	if err == nil {
		t.Fatal("a file with no marker was rewritten")
	}
	if !strings.Contains(err.Error(), "marker") {
		t.Errorf("the error does not say what is missing: %v", err)
	}
	if read(t, p) != before {
		t.Error("the file was changed despite the error")
	}
}

// The boundary lives in the document, so a marker without one is an error
// rather than a default.
func TestTailRefusesAMarkerWithNoBoundary(t *testing.T) {
	p := write(t, "# Log\n\n<!-- mustur:generated -->\n")
	if err := Tail(p, []record.Record{decision("MUS-D-0001", "one")}); err == nil {
		t.Fatal("a marker naming no from= was accepted")
	}
}
