package main

// What an amendment does to the record it amends.
//
// This was a wholesale replacement, and the argument for it was written down:
// carrying fields forward silently would make `amend --title` keep data the
// writer never saw, and the log holds the earlier version anyway. Both halves
// are true, and it lost to the evidence — fifteen amendments in the owner's
// store dropped content nobody meant to drop, eight records were still missing
// citations two days later, and one milestone lost its whole body and took
// three more amendments to rebuild (MUS-F-0055). The owner chose merge on
// MUS-Q-0063.

import (
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

func full() record.Record {
	return record.Record{
		ID: "MUS-F-0001", Kind: "finding",
		Title: "As it was", At: "2026-08-01", Body: "the original body",
		Refs: []record.Field{
			{Key: "found in", Value: "MUS-W-0001"},
			{Key: "found in", Value: "MUS-W-0002"},
		},
		Data: []record.Field{
			{Key: "Status", Value: "open"},
			{Key: "Where", Value: "somewhere.go"},
		},
	}
}

func keys(fs []record.Field) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Key+"="+f.Value)
	}
	return out
}

func same(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s is %v, want %v", what, got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s is %v, want %v", what, got, want)
			return
		}
	}
}

// The defect itself: changing the title took everything else with it.
func TestAmendingTheTitleKeepsTheRestOfTheRecord(t *testing.T) {
	in := record.Record{Title: "Retitled"}
	got := merge(full(), in, map[string]bool{"title": true}, nil)

	if got.Title != "Retitled" {
		t.Errorf("the title did not change: %q", got.Title)
	}
	if got.Body != "the original body" {
		t.Errorf("the body was lost: %q", got.Body)
	}
	if got.At != "2026-08-01" {
		t.Errorf("the date moved to %q; a correction is not a refiling", got.At)
	}
	same(t, "citations", keys(got.Refs), []string{"found in=MUS-W-0001", "found in=MUS-W-0002"})
	same(t, "fields", keys(got.Data), []string{"Status=open", "Where=somewhere.go"})
}

// A field passed again replaces its own value and keeps its own place. Fields
// render in order, so a correction that shuffles them is a diff nobody asked
// for.
func TestAFieldIsReplacedWhereItAlreadyStood(t *testing.T) {
	in := record.Record{Data: []record.Field{
		{Key: "Status", Value: "fixed"},
		{Key: "Evidence", Value: "measured"},
	}}
	got := merge(full(), in, map[string]bool{"data": true}, nil)
	same(t, "fields", keys(got.Data),
		[]string{"Status=fixed", "Where=somewhere.go", "Evidence=measured"})
}

// A citation key repeats — a record can be found in two work units under the
// same word — so citations are identified by both halves, not by the key.
func TestARepeatedCitationKeyIsNotCollapsed(t *testing.T) {
	in := record.Record{Refs: []record.Field{{Key: "found in", Value: "MUS-W-0003"}}}
	got := merge(full(), in, map[string]bool{"ref": true}, nil)
	same(t, "citations", keys(got.Refs),
		[]string{"found in=MUS-W-0001", "found in=MUS-W-0002", "found in=MUS-W-0003"})
}

// Passing one that is already there changes nothing, rather than doubling it.
func TestACitationAlreadyPresentIsNotDuplicated(t *testing.T) {
	in := record.Record{Refs: []record.Field{{Key: "found in", Value: "MUS-W-0002"}}}
	got := merge(full(), in, map[string]bool{"ref": true}, nil)
	same(t, "citations", keys(got.Refs),
		[]string{"found in=MUS-W-0001", "found in=MUS-W-0002"})
}

// Removal is the thing you type on purpose.
func TestDropTakesAFieldByNameAndACitationByIdentifier(t *testing.T) {
	got := merge(full(), record.Record{}, map[string]bool{}, names{"Where", "MUS-W-0001"})
	same(t, "fields", keys(got.Data), []string{"Status=open"})
	same(t, "citations", keys(got.Refs), []string{"found in=MUS-W-0002"})
}

// And a citation can go by its label, for the times the identifier is the part
// you cannot remember.
func TestDropTakesACitationByItsLabel(t *testing.T) {
	got := merge(full(), record.Record{}, map[string]bool{}, names{"found in"})
	if len(got.Refs) != 0 {
		t.Errorf("citations survived a drop of their label: %v", keys(got.Refs))
	}
}

// An empty body passed explicitly is a body being cleared, not a body absent.
func TestAnExplicitlyEmptyBodyClearsIt(t *testing.T) {
	got := merge(full(), record.Record{Body: ""}, map[string]bool{"body": true}, nil)
	if got.Body != "" {
		t.Errorf("--body '' did not clear the body: %q", got.Body)
	}
}
