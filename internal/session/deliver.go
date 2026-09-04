package session

// Carrying an answer back to the session that raised it — the clause milestone
// 3 could not honour, because nothing then could reach a session.
//
// It is typed in, with tmux send-keys. At the far end it is indistinguishable
// from the owner having typed it, which is exactly why the text says where it
// came from: an agent that cannot tell an answer from the owner's next
// instruction would treat every answer as a new task.

import (
	"context"
	"fmt"
	"strings"
)

// Sender is the part of the adapter delivery needs. Narrow so the answer path
// can be tested without tmux and cannot reach for anything else.
type Sender interface {
	Alive(ctx context.Context, project string) (bool, error)
	Send(ctx context.Context, project, text string) error
}

// Deliver types an answer into the session that raised the question and
// returns what to record about it — reaching the session, or why it did not.
//
// It never returns an error. A question that cannot be delivered is still
// answered: the answer is in the store and on the queue, and refusing to record
// it because a session went away would lose the one thing that was not
// recoverable. What the caller gets back is a sentence for the record.
func Deliver(ctx context.Context, s Sender, project, id, answer string) string {
	return DeliverRelayed(ctx, s, project, id, answer, "")
}

// DeliverRelayed is Deliver for an answer somebody wrote down on the owner's
// behalf. The provenance travels into the session with it (MUS-F-0085).
func DeliverRelayed(ctx context.Context, s Sender, project, id, answer, relayed string) string {
	if strings.TrimSpace(project) == "" {
		return "not delivered: the question names no session"
	}
	if _, err := NameFor(project); err != nil {
		return fmt.Sprintf("not delivered: %v", err)
	}
	live, err := s.Alive(ctx, project)
	if err != nil {
		return fmt.Sprintf("not delivered: %v", err)
	}
	if !live {
		return fmt.Sprintf("not delivered: %s has no session Mustur started, and Mustur never attaches to one it did not", project)
	}
	if err := s.Send(ctx, project, TextRelayed(id, answer, relayed)); err != nil {
		return fmt.Sprintf("not delivered: %v", err)
	}
	return fmt.Sprintf("typed into %s%s", Prefix, project)
}

// Text is what gets typed. It names the question so the receiving agent can
// tell an answer from a fresh instruction, and says the owner answered it so
// nothing treats Mustur as the author of a decision it only carried.
func Text(id, answer string) string { return TextRelayed(id, answer, "") }

// TextRelayed says who wrote the answer down when it was not the owner typing.
//
// Without this the delivered line reads "The owner answered X: ..." whatever
// its provenance -- so an agent that writes down an answer with
// `mustur answer --from-owner`, in a session the question names with --in, has
// its own words typed back into its own stdin wearing the owner's name. It
// happened on MUS-Q-0076 and was caught only by reading the event log, which is
// the exact thing MUS-D-0126 exists to make unnecessary (MUS-F-0085).
func TextRelayed(id, answer, relayed string) string {
	answer = strings.TrimSpace(answer)
	if r := strings.TrimSpace(relayed); r != "" {
		return fmt.Sprintf("%s was answered %q -- %s. Not typed by the owner here.", id, answer, r)
	}
	return fmt.Sprintf("The owner answered %s: %s", id, answer)
}
