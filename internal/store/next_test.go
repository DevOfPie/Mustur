package store

import (
	"testing"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
)

func TestNextIDStartsAtOneAndCounts(t *testing.T) {
	s, ctx, _ := open(t)
	first, err := s.NextID(ctx, "MUS", ident.Finding)
	if err != nil {
		t.Fatal(err)
	}
	if first != "MUS-F-0001" {
		t.Fatalf("first finding is %s", first)
	}
	if err := s.Append(ctx, record.Record{ID: first, Kind: "finding", Title: "one", At: "2026-08-20"}, "create", "test"); err != nil {
		t.Fatal(err)
	}
	next, err := s.NextID(ctx, "MUS", ident.Finding)
	if err != nil {
		t.Fatal(err)
	}
	if next != "MUS-F-0002" {
		t.Fatalf("second finding is %s", next)
	}
}

// Roles and projects number independently: a decision does not push a finding's
// serial along, and a second project starts its own count.
func TestSerialsAreScopedToProjectAndRole(t *testing.T) {
	s, ctx, _ := open(t)
	if err := s.Append(ctx, record.Record{ID: "MUS-D-0007", Kind: "decision", Title: "d", At: "2026-08-20"}, "create", "test"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		project string
		role    ident.Role
		want    string
	}{
		{"MUS", ident.Finding, "MUS-F-0001"},
		{"MUS", ident.Decision, "MUS-D-0008"},
		{"LNK", ident.Decision, "LNK-D-0001"},
	} {
		got, err := s.NextID(ctx, c.project, c.role)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("next %s %s = %s, want %s", c.project, c.role, got, c.want)
		}
	}
}

// A serial is never reused. Amending a record leaves it occupying its number,
// and nothing fills a gap — an identifier quoted in a report has to keep
// meaning the same record.
func TestSerialsAreNeverReused(t *testing.T) {
	s, ctx, _ := open(t)
	for _, id := range []string{"MUS-F-0001", "MUS-F-0002"} {
		if err := s.Append(ctx, record.Record{ID: id, Kind: "finding", Title: "f", At: "2026-08-20"}, "create", "test"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Append(ctx, record.Record{ID: "MUS-F-0002", Kind: "finding", Title: "corrected", At: "2026-08-20"}, "amend", "test"); err != nil {
		t.Fatal(err)
	}
	got, err := s.NextID(ctx, "MUS", ident.Finding)
	if err != nil {
		t.Fatal(err)
	}
	if got != "MUS-F-0003" {
		t.Fatalf("next finding is %s; an amendment moved the count", got)
	}
}
