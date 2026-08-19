package export

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

func sample() []record.Record {
	return []record.Record{
		{ID: "MUS-D-0002", Kind: "decision", Title: "Second", At: "2026-08-19", Body: "Because."},
		{ID: "MUS-M-0001", Kind: "milestone", Title: "First", At: "2026-08-19"},
		{ID: "MUS-W-0001", Kind: "work-unit", Title: "A unit", At: "2026-08-19",
			Refs: []record.Field{{Key: "Discharges", Value: "MUS-M-0001"}},
			Data: []record.Field{{Key: "Done means", Value: "It is done."}, {Key: "Risks", Value: "Some."}}},
		{ID: "MUS-R-0001", Kind: "repository", Title: "A repo | with a pipe", At: "2026-08-19"},
	}
}

// The export is committed, so a diff has to show what changed in the records
// and nothing else. That is only true if rendering is a function of the
// records alone.
func TestRenderIsDeterministic(t *testing.T) {
	first, err := Render(sample())
	if err != nil {
		t.Fatal(err)
	}
	shuffled := sample()
	shuffled[0], shuffled[3] = shuffled[3], shuffled[0]
	second, err := Render(shuffled)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(second) {
		t.Fatalf("%d files then %d", len(first), len(second))
	}
	for name, want := range first {
		got, ok := second[name]
		if !ok {
			t.Errorf("%s missing from the second render", name)
			continue
		}
		if !bytes.Equal(want, got) {
			t.Errorf("%s differs between two renders of the same records", name)
		}
	}
}

func TestCitationsBecomeLinks(t *testing.T) {
	files, err := Render(sample())
	if err != nil {
		t.Fatal(err)
	}
	unit := string(files["work-units/MUS-W-0001.md"])
	if !strings.Contains(unit, "[MUS-M-0001](../milestones.md#mus-m-0001)") {
		t.Errorf("a work unit did not link out to the milestone it discharges:\n%s", unit)
	}
	index := string(files["README.md"])
	if !strings.Contains(index, "[MUS-W-0001](work-units/MUS-W-0001.md#mus-w-0001)") {
		t.Errorf("the index did not link into the work-unit tree:\n%s", index)
	}
}

// A pipe in a title would otherwise open a cell the header never declared, and
// the repository's own table check would fail on generated output.
func TestPipesInCellsAreEscaped(t *testing.T) {
	files, err := Render(sample())
	if err != nil {
		t.Fatal(err)
	}
	index := string(files["README.md"])
	if !strings.Contains(index, `A repo \| with a pipe`) {
		t.Errorf("a pipe in a title reached a table cell unescaped:\n%s", index)
	}
}

func TestWritePrunesWhatItDidNotWrite(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "stale.md")
	if err := os.WriteFile(stale, []byte("a record that moved"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, sample()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale file survived the export: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "decisions.md")); err != nil {
		t.Errorf("decisions.md was not written: %v", err)
	}
}

func TestUnknownKindIsAnError(t *testing.T) {
	_, err := Render([]record.Record{{ID: "MUS-Z-0001", Kind: "nonsense", Title: "x", At: "2026-08-19"}})
	if err == nil {
		t.Fatal("a kind with no export file rendered without complaint")
	}
}
