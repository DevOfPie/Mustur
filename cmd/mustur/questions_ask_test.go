package main

// `mustur ask` is where a marker on the label is moved or refused. The rules
// live in internal/question; these hold that ask applies them, so deleting the
// NormaliseOption call, or the CheckOption call, fails here.

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/question"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/seed"
	"github.com/DevOfPie/Mustur/internal/store"
)

// questionsIn lists the store's questions.
func questionsIn(t *testing.T, path string) []record.Record {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rs, err := s.List(ctx, question.Kind)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func seeded(t *testing.T) string {
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
	return path
}

func TestAskMovesAnUnambiguousMarkerOffTheLabel(t *testing.T) {
	path := seeded(t)
	before := len(questionsIn(t, path))
	if err := cmdAsk([]string{"--db", path, "--title", "which", "--actor", "test",
		"--option", "Recommended: Refuse :: closes it :: the detail",
		"--option", "Allow :: leaves it open :: the detail"}); err != nil {
		t.Fatal(err)
	}
	after := questionsIn(t, path)
	if len(after) != before+1 {
		t.Fatalf("ask left %d question(s), want %d", len(after), before+1)
	}
	var got []question.Option
	for _, r := range after {
		if r.Title == "which" {
			got = question.Options(r)
		}
	}
	if len(got) != 2 {
		t.Fatalf("stored options = %+v, want two", got)
	}
	if o := got[0]; o.Label != "Refuse" || !strings.HasPrefix(o.Line, question.Recommended) || !o.IsRecommended() {
		t.Errorf("stored option = %+v, want label %q and the marker on the line", o, "Refuse")
	}
	if got[1].IsRecommended() {
		t.Errorf("second option %+v gained a star", got[1])
	}
}

func TestAskRefusesAMarkerTheLabelMayNotMean(t *testing.T) {
	path := seeded(t)
	before := len(questionsIn(t, path))
	err := cmdAsk([]string{"--db", path, "--title", "keep them?", "--actor", "test",
		"--option", "Recommended settings :: keep what ships :: the detail"})
	if err == nil {
		t.Fatal("ask accepted \"Recommended settings\" as a label")
	}
	if !strings.Contains(err.Error(), "settings :: Recommended. keep what ships :: the detail") {
		t.Errorf("refusal %q does not give the corrected --option", err)
	}
	if n := len(questionsIn(t, path)); n != before {
		t.Errorf("a refused ask left %d question(s), want %d", n, before)
	}
}
