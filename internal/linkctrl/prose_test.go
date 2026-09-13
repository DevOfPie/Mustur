package linkctrl

import (
	"strings"
	"testing"
)

func TestInvestigationTakesItsNumberStatusAndDate(t *testing.T) {
	rec, err := Investigation("0001-partitioning-and-sqlc.md",
		"# ADR 0001: Partitioning and sqlc\n\nStatus: accepted, 2026-07-29\n\n## Context\n\nSee [the log](../build-notes/decisions.md).\n", "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != "LNK-I-0001" || rec.At != "2026-07-29" || rec.Title != "Partitioning and sqlc" {
		t.Fatalf("%s at %s titled %q", rec.ID, rec.At, rec.Title)
	}
	if v, _ := rec.Get("Status"); v != "accepted" {
		t.Fatalf("status %q", v)
	}
	if strings.Contains(rec.Body, "Status:") || strings.Contains(rec.Body, "](") {
		t.Fatalf("body kept the status line or a link into the tree: %q", rec.Body)
	}
}

const questionsFixture = "# Upcoming decisions\n\n" +
	"## Open — a milestone needs this\n\n" +
	"### M55 — Does the update checker default on or off?\n\n**Answered 2026-08-08** by the owner.\n\n" +
	"## Open — nothing forces this\n\n" +
	"### An 'All Workspaces' dashboard scope — which phase?\n\n**Needed by:** nothing.\n\n" +
	"### <milestone> — <the question in one sentence>\n\n**Needed by:**\n\n```\n"

func TestQuestionsSkipTheTemplateAndReadAnswered(t *testing.T) {
	got, err := Questions(questionsFixture, "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%d questions, want 2 and no template", len(got))
	}
	if s, _ := got[0].Get("Status"); s != "answered" || got[0].At != "2026-08-08" {
		t.Fatalf("first: %s at %s", s, got[0].At)
	}
	if s, _ := got[1].Get("Status"); s != "open" || got[1].ID != "LNK-Q-0002" {
		t.Fatalf("second: %s %s", got[1].ID, s)
	}
}
