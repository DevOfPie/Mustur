package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DevOfPie/Mustur/internal/question"
	"github.com/DevOfPie/Mustur/internal/session"
)

// Three states, because "the default set" and "no gate at all" are different
// answers and an empty flag has to mean one of them. MUS-D-0153 promises the
// second and the tree had no way to say it for one commit.
func TestTheGateFlagCanSayNoGateAtAll(t *testing.T) {
	if got := gateFlag(""); got != nil {
		t.Errorf("unset gave %v, want nil so the adapter uses its default", got)
	}
	none := gateFlag("none")
	if none == nil || len(none) != 0 {
		t.Errorf("none gave %v, want an empty set that is not nil", none)
	}
	if session.Gated(none, "default", "Bash") {
		t.Error("a session that gates nothing held a call anyway")
	}
	if got := gateFlag("Bash, Edit"); len(got) != 2 || got[0] != "Bash" || got[1] != "Edit" {
		t.Errorf("a list gave %v", got)
	}
}

// Mustur is the prompt in its own sessions (MUS-D-0156), so a question raised
// with --in naming a live one is surfaced by the raising. A question that names
// no session still owes a prompt.
//
// The behaviour needs tmux to be meaningful: what decides it is whether the
// adapter finds a session Mustur started.
func TestAQuestionRaisedIntoAMusturSessionIsSurfacedByBeingRaised(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux on PATH; this test only means something against the real thing")
	}
	db := filepath.Join(t.TempDir(), "q.db")
	if err := run([]string{"seed", "--db", db}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := &session.Adapter{}
	project := "zzAskSurfaced"
	if _, err := a.Start(context.Background(), project, t.TempDir(), "sh -c 'sleep 20'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Stop(context.Background(), project) })

	if err := run([]string{"ask", "--db", db, "--title", "Into a live session", "--in", project,
		"--option", "yes :: a :: b"}); err != nil {
		t.Fatalf("ask: %v", err)
	}
	if err := run([]string{"ask", "--db", db, "--title", "Into nothing at all",
		"--option", "yes :: a :: b"}); err != nil {
		t.Fatalf("ask: %v", err)
	}

	s, ctx, err := openStore(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	into, err := s.Get(ctx, "MUS-Q-0001")
	if err != nil {
		t.Fatal(err)
	}
	if !question.Surfaced(into) {
		t.Error("a question raised into a live Mustur session still owes a prompt, which is the collision MUS-Q-0098 settled")
	}
	alone, err := s.Get(ctx, "MUS-Q-0002")
	if err != nil {
		t.Fatal(err)
	}
	if question.Surfaced(alone) {
		t.Error("a question naming no session was surfaced by nothing at all")
	}
}
