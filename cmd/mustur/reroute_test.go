package main

// What reroute will and will not take.
//
// It corrects a jot that "Route it for me" put in the wrong place. It used to
// take anything with a body: a project record went through, its description
// filed as a fresh finding in the idea inbox and the record defining the
// store's own prefix marked superseded by that finding (MUS-F-0058).

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/intake"
	"github.com/DevOfPie/Mustur/internal/question"
	"github.com/DevOfPie/Mustur/internal/seed"
	"github.com/DevOfPie/Mustur/internal/store"
)

// jotted gives a store with routing in it and one jot in the idea inbox.
func jotted(t *testing.T) (string, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "t.db")
	s, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := seed.Apply(ctx, s); err != nil {
		t.Fatal(err)
	}
	r, _, err := intake.File(ctx, s, intake.Request{
		Project: "MUS", Text: "a jot that went to the wrong place",
		Actor: "test", Now: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return path, r.ID
}

func TestRerouteMovesAJot(t *testing.T) {
	path, id := jotted(t)
	if err := cmdReroute([]string{id, "--to", "MUS-P-0001", "--db", path}); err != nil {
		t.Fatalf("a jot would not reroute: %v", err)
	}
}

// A record intake never filed has no routing to correct. "Routed to" is written
// by intake.File and by nothing else, so its absence is the answer.
func TestRerouteRefusesARecordIntakeNeverFiled(t *testing.T) {
	path, _ := jotted(t)
	for _, id := range []string{"MUS-P-0001", "MUS-R-0001", "MUS-H-0001"} {
		err := cmdReroute([]string{id, "--to", "MUS-P-0002", "--db", path})
		if err == nil {
			t.Errorf("%s was rerouted; it was never routed in the first place", id)
			continue
		}
		if !strings.Contains(err.Error(), "intake box") {
			t.Errorf("%s was refused for the wrong reason: %v", id, err)
		}
	}
}

// An option is a label, what it costs, and the paragraph behind it. A label on
// its own hands the owner a word and leaves them to work out the rest — the
// bare question the contract asks nobody to send, one level down. --data
// refuses a value that is not key=value at this same boundary.
func TestABareLabelIsNotAnOption(t *testing.T) {
	var v values
	if err := v.Set("Just a label"); err == nil {
		t.Error("a label with nothing behind it was accepted as an option")
	}
	if err := v.Set("Label" + question.OptionSep + "what it costs"); err != nil {
		t.Errorf("a two-part option was refused: %v", err)
	}
	if err := v.Set("Label" + question.OptionSep + "line" + question.OptionSep + "detail"); err != nil {
		t.Errorf("a three-part option was refused: %v", err)
	}
	if len(v) != 2 {
		t.Errorf("%d option(s) kept, want 2", len(v))
	}
}
