package session

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type sender struct {
	live    bool
	liveErr error
	sendErr error
	sent    string
	project string
	// dialog is what the pane is showing. Empty is the ordinary case; a
	// non-empty one is the case MUS-F-0125 measured, where a paste and its
	// Enter operate the dialog instead of reaching the agent.
	dialog    string
	dialogErr error
	pressed   []string
}

func (s *sender) Dialog(context.Context, string) (*Prompt, error) {
	if s.dialog == "" {
		return nil, s.dialogErr
	}
	return &Prompt{Title: s.dialog}, s.dialogErr
}

func (s *sender) SendChoice(_ context.Context, _, key string) error {
	s.pressed = append(s.pressed, key)
	return nil
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
		{"name would address a pane", &sender{live: true}, "Mustur:0", `cannot contain ":"`},
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

// A relayed answer says so when it lands in the session.
//
// MUS-F-0085: the delivered line read "The owner answered X: ..." whatever its
// provenance, so an agent that wrote down an answer with --from-owner, into a
// session the question named with --in, had its own words typed back into its
// own stdin under the owner's name. MUS-D-0126 exists so nobody reads a relay
// as the owner having been here, and the one place the relay actually travelled
// to was the place that lost it.
func TestARelayedAnswerDoesNotArriveWearingTheOwnersName(t *testing.T) {
	plain := Text("MUS-Q-0001", "do the thing")
	if plain != "The owner answered MUS-Q-0001: do the thing" {
		t.Errorf("an unrelayed answer changed shape: %q", plain)
	}

	got := TextRelayed("MUS-Q-0076", "keep the pane",
		"written down by whippy, from the owner's own message in this session's chat")
	if strings.Contains(got, "The owner answered") {
		t.Errorf("a relayed answer still claims the owner typed it: %q", got)
	}
	for _, want := range []string{"MUS-Q-0076", "keep the pane", "written down by whippy", "Not typed by the owner here"} {
		if !strings.Contains(got, want) {
			t.Errorf("relayed text is missing %q: %q", want, got)
		}
	}
}

// A dialog on the screen eats the paste and is pressed by the Enter behind it,
// so nothing is sent at all. Measured against the real CLI on 2026-09-10: the
// answer was nowhere on the pane afterwards and the model picker had been
// pressed.
func TestNothingIsDeliveredIntoADialog(t *testing.T) {
	s := &sender{live: true, dialog: "Select model"}
	got := Deliver(context.Background(), s, "Mustur", "MUS-Q-0001", "Split it.")

	if s.sent != "" || len(s.pressed) != 0 {
		t.Fatalf("typed %q and pressed %q into a pane showing a dialog", s.sent, s.pressed)
	}
	for _, want := range []string{"not delivered", "Select model", "in the queue"} {
		if !strings.Contains(got, want) {
			t.Errorf("record says %q, want it to mention %q", got, want)
		}
	}
}

// A screen that cannot be read is not a dialog. Refusing on a failed read would
// make an unreadable pane and a dialog the same thing, and the common case is
// that there is no dialog at all.
func TestAnUnreadablePaneIsDeliveredInto(t *testing.T) {
	s := &sender{live: true, dialogErr: fmt.Errorf("no server running")}
	got := Deliver(context.Background(), s, "Mustur", "MUS-Q-0001", "Split it.")

	if s.sent == "" {
		t.Fatalf("nothing was delivered because the pane could not be read: %q", got)
	}
}

// The session name tmux gives you is a name delivery accepts.
//
// A session raising a question knows itself as "mustur/Hoard_Work", because
// that is what tmux reports. Passing it to --in produced a question whose
// answer could never arrive: delivery prepends the prefix again and a project
// name may not contain a slash. The owner met it as "not delivered" on an
// answer they had already given (MUS-F-0141).
func TestTheTmuxSessionNameIsAcceptedAsWellAsTheProject(t *testing.T) {
	for _, name := range []string{"Hoard_Work", "mustur/Hoard_Work", "  mustur/Hoard_Work  "} {
		if got := ProjectFrom(name); got != "Hoard_Work" {
			t.Errorf("ProjectFrom(%q) = %q, want Hoard_Work", name, got)
		}
	}

	for _, name := range []string{"Hoard_Work", "mustur/Hoard_Work"} {
		s := &sender{live: true}
		said := Deliver(context.Background(), s, name, "HRD-Q-0006", "Fork only")
		if strings.Contains(said, "not delivered") {
			t.Errorf("%q: %s", name, said)
		}
		// And it reaches the project, not the session name with the prefix on.
		if s.project != "Hoard_Work" {
			t.Errorf("%q was delivered to %q", name, s.project)
		}
	}
}

// A name that is wrong for some other reason still says so.
func TestANameThatIsActuallyWrongIsStillRefused(t *testing.T) {
	s := &sender{live: true}
	said := Deliver(context.Background(), s, "two words", "MUS-Q-0001", "yes")
	if !strings.Contains(said, "not delivered") {
		t.Errorf("a name with a space was delivered to: %s", said)
	}
	if s.sent != "" {
		t.Error("something was typed into a session that cannot be named")
	}
}

// paneTmux is a tmux with one session whose screen changes when a key is
// pressed on it, which is the whole of what MUS-D-0202 turns on: the survey's
// Dismiss is pressed, and the screen after it decides whether the answer goes
// in.
type paneTmux struct {
	mu      sync.Mutex
	screen  string
	pressed map[string]string // a key sent with "-l --", and the screen after it
	calls   [][]string
}

func (r *paneTmux) Run(_ context.Context, _ string, args ...string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, args)
	switch args[0] {
	case "list-sessions":
		return owned("mustur/Mustur", 1, false), nil
	case "capture-pane":
		return r.screen, nil
	case "send-keys":
		if n := len(args); n >= 2 && args[n-2] == "--" {
			if after, ok := r.pressed[args[n-1]]; ok {
				r.screen = after
			}
		}
	}
	return "", nil
}

// sent is what every send-keys call sent, in order.
func (r *paneTmux) sent() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, c := range r.calls {
		if c[0] == "send-keys" {
			out = append(out, c[len(c)-1])
		}
	}
	return out
}

func quickSurveyWait(t *testing.T) {
	t.Helper()
	w, p := surveyGoneWait, surveyGonePoll
	surveyGoneWait, surveyGonePoll = 200*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { surveyGoneWait, surveyGonePoll = w, p })
}

// MUS-D-0202: with the survey up, delivery presses the survey's own Dismiss --
// alone, no Enter -- sees it gone, and then types the answer.
func TestAnAnswerDismissesTheSurveyAndIsThenTyped(t *testing.T) {
	quickSurveyWait(t)
	r := &paneTmux{
		screen:  fixture(t, "screen-survey.txt"),
		pressed: map[string]string{"0": fixture(t, "screen-working.txt")},
	}
	got := Deliver(context.Background(), &Adapter{Run: r}, "Mustur", "MUS-Q-0001", "Split it.")

	if !strings.Contains(got, "typed into mustur/Mustur") {
		t.Fatalf("recorded %q", got)
	}
	want := []string{"0", Text("MUS-Q-0001", "Split it."), "Enter"}
	if s := r.sent(); !slices.Equal(s, want) {
		t.Errorf("sent %q, want %q", s, want)
	}
}

// A survey still on the screen after its Dismiss is a dialog, and a dialog is
// not typed into.
func TestASurveyThatWillNotDismissIsNotTypedInto(t *testing.T) {
	quickSurveyWait(t)
	r := &paneTmux{screen: fixture(t, "screen-survey.txt")}
	got := Deliver(context.Background(), &Adapter{Run: r}, "Mustur", "MUS-Q-0001", "Split it.")

	for _, want := range []string{"not delivered", "would not dismiss", "in the queue"} {
		if !strings.Contains(got, want) {
			t.Errorf("record says %q, want it to mention %q", got, want)
		}
	}
	if s := r.sent(); !slices.Equal(s, []string{"0"}) {
		t.Errorf("sent %q, want only the Dismiss", s)
	}
}

// Any other dialog is refused as before, and nothing at all is pressed on it.
func TestAnotherDialogIsStillRefusedWithNothingPressed(t *testing.T) {
	quickSurveyWait(t)
	r := &paneTmux{
		screen:  fixture(t, "prompt-model-picker.txt"),
		pressed: map[string]string{"0": fixture(t, "screen-working.txt")},
	}
	got := Deliver(context.Background(), &Adapter{Run: r}, "Mustur", "MUS-Q-0001", "Split it.")

	if !strings.HasPrefix(got, "not delivered") || !strings.Contains(got, "in the queue") {
		t.Errorf("recorded %q", got)
	}
	if s := r.sent(); len(s) != 0 {
		t.Errorf("sent %q into a pane showing a dialog", s)
	}
}

// With nothing up, delivery is what it always was: the answer and its Enter.
func TestWithNothingUpTheAnswerIsTypedAsBefore(t *testing.T) {
	quickSurveyWait(t)
	r := &paneTmux{screen: fixture(t, "screen-working.txt")}
	got := Deliver(context.Background(), &Adapter{Run: r}, "Mustur", "MUS-Q-0001", "Split it.")

	if !strings.Contains(got, "typed into mustur/Mustur") {
		t.Fatalf("recorded %q", got)
	}
	want := []string{Text("MUS-Q-0001", "Split it."), "Enter"}
	if s := r.sent(); !slices.Equal(s, want) {
		t.Errorf("sent %q, want %q", s, want)
	}
}
