package session

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type sender struct {
	live    bool
	liveErr error
	sendErr error
	sent    string
	project string
}

func (s *sender) Alive(context.Context, string) (bool, error) { return s.live, s.liveErr }

func (s *sender) Send(_ context.Context, project, text string) error {
	s.project, s.sent = project, text
	return s.sendErr
}

func TestDeliveryTypesTheAnswerIntoTheSession(t *testing.T) {
	s := &sender{live: true}
	got := Deliver(context.Background(), s, "Mustur", "MUS-Q-0001", "Split it.")

	if s.project != "Mustur" {
		t.Errorf("sent to %q", s.project)
	}
	for _, want := range []string{"MUS-Q-0001", "Split it.", "owner answered"} {
		if !strings.Contains(s.sent, want) {
			t.Errorf("typed text is missing %q: %q", want, s.sent)
		}
	}
	if !strings.Contains(got, "typed into mustur/Mustur") {
		t.Errorf("recorded %q", got)
	}
}

// The receiving agent has to be able to tell an answer from a fresh
// instruction, and must not read Mustur as the author of the decision.
func TestTheTypedTextNamesTheQuestionAndTheOwner(t *testing.T) {
	got := Text("MUS-Q-0007", "Refuse self-answer.")
	if !strings.HasPrefix(got, "The owner answered MUS-Q-0007:") {
		t.Errorf("text = %q", got)
	}
}

// An answer is never lost because a session went away. It is recorded either
// way, and what happened to the delivery is recorded with it.
func TestAnUndeliverableAnswerIsStillRecordedWithTheReason(t *testing.T) {
	cases := []struct {
		name    string
		sender  *sender
		project string
		want    string
	}{
		{"no session named", &sender{live: true}, "", "names no session"},
		{"session is gone", &sender{live: false}, "Mustur", "has no session Mustur started"},
		{"tmux failed while looking", &sender{liveErr: fmt.Errorf("socket gone")}, "Mustur", "socket gone"},
		{"typing failed", &sender{live: true, sendErr: fmt.Errorf("pane died")}, "Mustur", "pane died"},
		{"name would address a pane", &sender{live: true}, "Mustur:0", "target separators"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Deliver(context.Background(), c.sender, c.project, "MUS-Q-0001", "an answer")
			if !strings.HasPrefix(got, "not delivered") {
				t.Fatalf("recorded %q, want a not-delivered reason", got)
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("recorded %q, missing %q", got, c.want)
			}
		})
	}
}
