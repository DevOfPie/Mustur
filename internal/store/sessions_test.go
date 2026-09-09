package store

import (
	"testing"
	"time"
)

func TestRememberedSessionSurvivesAndIsForgottenOnStop(t *testing.T) {
	s, ctx, _ := open(t)

	if err := s.RememberSession(ctx, "Mustur", "/home/w/repos/Mustur", "claude"); err != nil {
		t.Fatal(err)
	}
	got, err := s.RememberedSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Project != "Mustur" || got[0].Dir != "/home/w/repos/Mustur" || got[0].Cmd != "claude" {
		t.Fatalf("remembered %+v, want the one session that was started", got)
	}
	if got[0].CLI != "" {
		t.Errorf("CLI = %q before the hook has said anything, want empty", got[0].CLI)
	}
	if got[0].Started.IsZero() {
		t.Error("Started is zero, so nothing can say how long ago the session was lost")
	}

	if err := s.NoteSessionCLI(ctx, "Mustur", "abc-123", "/transcripts/abc-123.jsonl"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.RememberedSessions(ctx)
	if got[0].CLI != "abc-123" || got[0].Transcript != "/transcripts/abc-123.jsonl" {
		t.Errorf("row = %+v after the hook reported, want the identifier and the file", got[0])
	}

	// The distinction the whole table exists for: a session the owner ended is
	// finished, and must not be offered back.
	if err := s.ForgetSession(ctx, "Mustur"); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.RememberedSessions(ctx); len(got) != 0 {
		t.Errorf("a stopped session is still remembered: %+v", got)
	}
}

func TestStartingAgainClearsTheOldConversation(t *testing.T) {
	s, ctx, _ := open(t)
	if err := s.RememberSession(ctx, "Mustur", "/a", "claude"); err != nil {
		t.Fatal(err)
	}
	if err := s.NoteSessionCLI(ctx, "Mustur", "old-id", "/transcripts/old.jsonl"); err != nil {
		t.Fatal(err)
	}
	// Started again, in a different place, before its hook has fired. Carrying
	// the old identifier forward would offer a restore that resumed a
	// conversation belonging to the previous session.
	if err := s.RememberSession(ctx, "Mustur", "/b", "claude --model x"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.RememberedSessions(ctx)
	if len(got) != 1 {
		t.Fatalf("remembered %d rows, want one per project", len(got))
	}
	if got[0].CLI != "" || got[0].Transcript != "" {
		t.Errorf("row = %+v after a fresh start, want the old conversation cleared", got[0])
	}
	if got[0].Dir != "/b" || got[0].Cmd != "claude --model x" {
		t.Errorf("row = %+v, want the second start's directory and command", got[0])
	}
}

func TestAConversationIsNotNotedForASessionNobodyIsHolding(t *testing.T) {
	s, ctx, _ := open(t)
	// A hook naming a project with no row is a session Mustur did not start.
	// Inventing a row would put a restore button in front of it.
	if err := s.NoteSessionCLI(ctx, "SomebodyElses", "abc", "/t.jsonl"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.RememberedSessions(ctx); len(got) != 0 {
		t.Errorf("a hook created a row out of nothing: %+v", got)
	}
}

func TestRememberedSessionsAreNewestFirst(t *testing.T) {
	s, ctx, _ := open(t)
	at := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return at }
	if err := s.RememberSession(ctx, "older", "/a", "claude"); err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return at.Add(time.Hour) }
	if err := s.RememberSession(ctx, "newer", "/b", "claude"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.RememberedSessions(ctx)
	if len(got) != 2 || got[0].Project != "newer" {
		t.Errorf("order = %+v, want the most recently started first", got)
	}
}
