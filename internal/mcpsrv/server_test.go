package mcpsrv

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

func serverWith(t *testing.T, records ...record.Record) (*Server, context.Context) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, r := range records {
		if err := s.Append(ctx, r, "create", "test"); err != nil {
			t.Fatal(err)
		}
	}
	return &Server{store: s}, ctx
}

func fixtures() []record.Record {
	return []record.Record{
		{ID: "MUS-R-0001", Kind: "repository", Title: "DevOfPie/Mustur", At: "2026-08-19",
			Data: []record.Field{{Key: "Contract", Value: "workflow.md"}}},
		{ID: "MUS-D-0001", Kind: "decision", Title: "Inject, never offer", At: "2026-08-19"},
	}
}

func TestIndexCarriesRoutingAndRecords(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	got, err := s.answer(ctx, Args{Repository: "Mustur", Task: "milestone 2"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Mustur: Mustur", "Session: milestone 2", "## Routing", "DevOfPie/Mustur", "workflow.md", "MUS-D-0001 — Inject, never offer"} {
		if !strings.Contains(got, want) {
			t.Errorf("the index is missing %q:\n%s", want, got)
		}
	}
}

func TestIdentifierReturnsOneRecord(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	got, err := s.answer(ctx, Args{Repository: "Mustur", ID: "MUS-D-0001"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "## MUS-D-0001") || !strings.Contains(got, "Inject, never offer") {
		t.Errorf("record not returned:\n%s", got)
	}
	if strings.Contains(got, "## Routing") {
		t.Errorf("an identifier call returned the whole index:\n%s", got)
	}
}

// An empty result reads as "nothing to say about it", which is a different
// claim from "no such record".
func TestUnknownIdentifierSaysSo(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	got, err := s.answer(ctx, Args{Repository: "Mustur", ID: "MUS-D-9999"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "holds no record MUS-D-9999") {
		t.Errorf("unknown identifier gave:\n%s", got)
	}
}

func TestMalformedIdentifierSaysWhatOneLooksLike(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	got, err := s.answer(ctx, Args{Repository: "Mustur", ID: "decision 1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "PROJECT-ROLE-SERIAL") {
		t.Errorf("malformed identifier gave:\n%s", got)
	}
}

func TestUnknownKindListsTheKinds(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	got, err := s.answer(ctx, Args{Repository: "Mustur", Kind: "tickets"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "no record kind") || !strings.Contains(got, "investigation") {
		t.Errorf("unknown kind gave:\n%s", got)
	}
}

// The argument the disproof scored is required, not optional.
func TestRepositoryIsRequired(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	if _, err := s.answer(ctx, Args{}); err == nil {
		t.Fatal("a call with no repository was answered")
	}
}
