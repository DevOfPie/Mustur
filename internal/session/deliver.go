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
	"time"
)

// Sender is the part of the adapter delivery needs. Narrow so the answer path
// can be tested without tmux and cannot reach for anything else.
type Sender interface {
	Alive(ctx context.Context, project string) (bool, error)
	Send(ctx context.Context, project, text string) error
	// Dialog is what the pane is waiting on, or nil. Delivery refuses while
	// one is up, except the session survey, which it dismisses: see below.
	Dialog(ctx context.Context, project string) (*Prompt, error)
	// SendChoice presses one key a prompt offered. Delivery uses it for the
	// survey's Dismiss and nothing else.
	SendChoice(ctx context.Context, project, key string) error
}

// How long delivery waits for a dismissed survey to leave the pane, and how
// often it looks. Variables so a test need not wait out the real bound.
var (
	surveyGoneWait = 3 * time.Second
	surveyGonePoll = 100 * time.Millisecond
)

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
	// Records written before MUS-F-0141 was fixed hold the tmux session name
	// rather than the project, and they are still deliverable -- the answer is
	// worth more than the shape of the field it was recorded against.
	project = ProjectFrom(project)
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
	// **Never into a pane that is showing a dialog.**
	//
	// Send is a paste followed by Enter. A CLI showing its own dialog reads
	// both: the text goes nowhere the reader will ever see, and the Enter
	// presses whatever the dialog had selected. Measured on 2026-09-10 against
	// the real CLI — an answer delivered into a session showing the model
	// picker was nowhere on the screen afterwards, and the picker had been
	// pressed (MUS-F-0125). MUS-F-0105 saw the same thing on 2026-09-08 and
	// named the modal as the first place to look.
	//
	// So the answer stays in the queue and the record says why. That is worse
	// than delivering and better than pressing a button nobody chose — the
	// dialog on the screen may be a permission prompt, and the Enter that
	// followed a lost answer would be answering it.
	//
	// A pane this cannot read is delivered into. Refusing on a failed read
	// would make an unreadable screen indistinguishable from a dialog, and the
	// common case by far is that there is no dialog at all.
	//
	// **The session survey is the one exception** (MUS-D-0202, the owner's
	// answer to MUS-Q-0164). It appears at the end of a turn, which is when
	// answers tend to arrive, so refusing on it would leave most answers queued
	// with no retry. Its own row names a key that dismisses it, the owner's
	// press is what started this delivery, and nothing else is sent to it: the
	// Dismiss key, then a fresh read that has to find the survey gone before a
	// character of the answer is typed. A survey that stays is refused like any
	// other dialog.
	p, err := s.Dialog(ctx, project)
	if err == nil && p != nil && p.Kind == PromptSurvey {
		if refused := dismissSurvey(ctx, s, project, p); refused != "" {
			return refused
		}
		p = nil
	}
	if err == nil && p != nil {
		return fmt.Sprintf("not delivered: %s%s is showing %q, and a paste into a dialog presses it; the answer is in the queue", Prefix, project, promptName(p))
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

// dismissSurvey presses the survey's own Dismiss and waits, bounded, for a read
// of the pane that no longer shows it. It returns "" when the pane is clear,
// and otherwise the sentence to record for an answer left in the queue.
//
// The key is the one the row names beside "Dismiss", not a remembered "0": the
// row is the legend (MUS-D-0190), and a survey whose row names no Dismiss gets
// nothing pressed.
func dismissSurvey(ctx context.Context, s Sender, project string, p *Prompt) string {
	key := ""
	for _, c := range p.Options {
		if strings.EqualFold(strings.TrimSpace(c.Label), "Dismiss") {
			key = c.Key
			break
		}
	}
	if key == "" {
		return fmt.Sprintf("not delivered: %s%s is showing %q with no Dismiss on its row, and a paste into it presses it; the answer is in the queue", Prefix, project, promptName(p))
	}
	if err := s.SendChoice(ctx, project, key); err != nil {
		return fmt.Sprintf("not delivered: dismissing the survey on %s%s failed: %v; the answer is in the queue", Prefix, project, err)
	}
	// Only a read that succeeds and finds nothing is clear. An unreadable pane
	// is delivered into when nothing was pressed, but here something was, and
	// "gone" has to be seen rather than assumed.
	deadline := time.Now().Add(surveyGoneWait)
	for {
		now, err := s.Dialog(ctx, project)
		if err == nil && now == nil {
			return ""
		}
		if err == nil && now.Kind != PromptSurvey {
			return fmt.Sprintf("not delivered: %s%s is showing %q after its survey was dismissed, and a paste into a dialog presses it; the answer is in the queue", Prefix, project, promptName(now))
		}
		if !time.Now().Before(deadline) {
			return fmt.Sprintf("not delivered: the survey on %s%s would not dismiss; %q was pressed and it was still there after %s; the answer is in the queue", Prefix, project, key, surveyGoneWait)
		}
		select {
		case <-ctx.Done():
			return fmt.Sprintf("not delivered: waiting for the survey on %s%s to dismiss: %v; the answer is in the queue", Prefix, project, ctx.Err())
		case <-time.After(surveyGonePoll):
		}
	}
}

// promptName is what a refusal calls a prompt: its heading, when it has one.
func promptName(p *Prompt) string {
	if p.Title != "" {
		return p.Title
	}
	return "a dialog"
}

// Dialog is what the pane is waiting on, or nil.
//
// The same read the session view makes to draw its pop-up, used here to decide
// whether typing into the pane is safe rather than what to draw. A pane that
// cannot be read is not a dialog: the error travels so the caller can tell the
// two apart, and delivery treats an unreadable screen as clear.
func (a *Adapter) Dialog(ctx context.Context, project string) (*Prompt, error) {
	screen, err := a.Capture(ctx, project, paneLines)
	if err != nil {
		return nil, err
	}
	return ReadPrompt(screen), nil
}
