package session

import (
	"context"
	"os"
	"strings"
	"testing"
)

// The fixture is a real capture, not a remembered shape.
//
// It was taken from a running Claude Code pane on 2026-09-04 by opening the
// model picker and dismissing it. MUS-F-0072 is why: the last thing written
// from a remembered format left a stray middot in every line, and the format
// document and the real output disagreed.
func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestReadPromptReadsARealModelPicker(t *testing.T) {
	p := ReadPrompt(fixture(t, "prompt-model-picker.txt"))
	if p == nil {
		t.Fatal("nothing was read off a screen that is a prompt")
	}
	if p.Title != "Select model" {
		t.Errorf("title = %q, want the dialog's own heading", p.Title)
	}
	// The description is wrapped to the pane's width, which is not the
	// reader's, so it comes back as one line for the surface to rewrap.
	if !strings.HasPrefix(p.Body, "Switch between Claude models.") ||
		!strings.HasSuffix(p.Body, "specify with --model.") {
		t.Errorf("body = %q, want the whole sentence unwrapped", p.Body)
	}
	if strings.Contains(p.Body, "  ") {
		t.Errorf("body kept the pane's padding: %q", p.Body)
	}
	// Three pressable rows and one that is not. The fourth is the picker's
	// effort cycler — "● High effort (default) ←/→ to adjust" — which is a real
	// control on this dialog and was being dropped when only numbered lines
	// counted as rows. It carries state and nothing to press (MUS-F-0101).
	var pressable, shown []Choice
	for _, o := range p.Options {
		if o.Key != "" {
			pressable = append(pressable, o)
		} else {
			shown = append(shown, o)
		}
	}
	if len(pressable) != 3 {
		t.Fatalf("read %d pressable options, want 3: %+v", len(pressable), p.Options)
	}
	for i, want := range []string{"1", "2", "3"} {
		if pressable[i].Key != want {
			t.Errorf("option %d sends %q, want %q", i, pressable[i].Key, want)
		}
		if !pressable[i].Sendable {
			t.Errorf("option %d is not marked sendable", i)
		}
	}
	if len(shown) != 1 || !strings.Contains(shown[0].Label, "High effort") {
		t.Errorf("the effort row read as %+v", shown)
	}
	if shown[0].Sendable {
		t.Error("a row with nothing to press is marked sendable")
	}
	if !strings.HasPrefix(p.Options[0].Label, "Default (recommended)") {
		t.Errorf("first label = %q", p.Options[0].Label)
	}
	// The cursor marks one row and only one.
	var selected []string
	for _, o := range pressable {
		if o.Selected {
			selected = append(selected, o.Key)
		}
	}
	if len(selected) != 1 || selected[0] != "3" {
		t.Errorf("cursor on %v, want just option 3", selected)
	}

	// The legend is the point: three keys, in the CLI's own words, including
	// one no allowlist written from the shape of a menu would have guessed.
	if len(p.Keys) != 3 {
		t.Fatalf("read %d legend keys, want 3: %+v", len(p.Keys), p.Keys)
	}
	got := map[string]string{}
	for _, k := range p.Keys {
		got[k.Key] = k.Label
	}
	for key, want := range map[string]string{
		"Enter": "set as default",
		"s":     "use this session only",
		"Esc":   "cancel",
	} {
		if got[key] != want {
			t.Errorf("legend[%q] = %q, want %q", key, got[key], want)
		}
	}
}

// Nothing readable means nothing offered. That is the fallback the whole
// design rests on: a wrong parse must cost the terminal nothing.
func TestReadPromptOffersNothingWhenItCannotRead(t *testing.T) {
	for name, screen := range map[string]string{
		"an ordinary session": "❯ do the thing\n  ⎿ done\n  auto mode on · PR #38\n",
		"no legend at all":    "Select model\n  1. One\n  2. Two\n",
		"prose with a middot": "  Some line · another clause\n  1. One\n",
		"a legend but no options": "Select model\n" +
			"  Enter to set as default · Esc to cancel\n",
		"one legend entry only": "  1. One\n  Esc to cancel\n",
		"empty":                 "",
	} {
		if p := ReadPrompt(screen); p != nil {
			t.Errorf("%s: read a prompt that is not there: %+v", name, p)
		}
	}
}

// The legend survives being read off a coloured pane, because that is what
// Capture returns.
func TestReadPromptSeesThroughColour(t *testing.T) {
	raw := fixture(t, "prompt-model-picker.txt")
	if !strings.Contains(raw, "\x1b[") {
		t.Fatal("the fixture has no escapes in it, so this test proves nothing")
	}
	if ReadPrompt(raw) == nil {
		t.Error("colour hid the prompt")
	}
}

// A choice off the pane is a name or a character, and nothing else.
func TestSendChoiceTakesALegendKeyAndRefusesTheRest(t *testing.T) {
	// Refusals happen before tmux is reached, so a runner that fails anything
	// makes a leak loud rather than silent.
	a := &Adapter{Run: refuseAll{}}
	for _, bad := range []string{
		"", "  ",
		"C-c",           // a name send-keys would interpret, and not one of ours
		"ab",            // two characters
		"\x1b",          // an escape byte dressed as a character
		" ",             // a space is a keypress nobody asked for
		"kill-server; ", // a tmux command, in case an argument is ever a command
	} {
		if err := a.SendChoice(context.Background(), "Mustur", bad); err == nil {
			t.Errorf("SendChoice(%q) was allowed", bad)
		}
	}

	// The legend's spellings reach the allowlist: Esc is what the CLI prints,
	// escape is what the map holds.
	for _, name := range []string{"Esc", "esc", "Enter", "enter", "Up"} {
		err := a.SendChoice(context.Background(), "Mustur", name)
		if err == nil {
			t.Errorf("SendChoice(%q) reached no runner at all", name)
		}
		if err != nil && strings.Contains(err.Error(), "neither a key this may send") {
			t.Errorf("SendChoice(%q) was refused as unknown; it is in the allowlist", name)
		}
	}
}

// And a legend character reaches the pane as that character.
//
// The named keys have their own real-tmux test; this is the other half, and it
// is the half that only exists because the model picker's legend offered "s".
func TestALegendCharacterArrivesLiterally(t *testing.T) {
	realTmux(t)
	a := &Adapter{}
	project := "zzChoice"
	start(t, a, project, "cat -v")

	ctx := context.Background()
	if err := a.SendChoice(ctx, project, "s"); err != nil {
		t.Fatalf("sending s: %v", err)
	}
	if got := waitFor(t, a, project, "s"); !strings.Contains(got, "s") {
		t.Fatalf("the character never reached the pane; screen was:\n%s", got)
	}
	// A digit is the same path, which is why numbered options need no allowlist
	// entry of their own.
	if err := a.SendChoice(ctx, project, "3"); err != nil {
		t.Fatalf("sending 3: %v", err)
	}
	got := waitFor(t, a, project, "s3")
	if !strings.Contains(got, "s3") {
		t.Fatalf("the digit did not follow the character; screen was:\n%s", got)
	}
	// Nothing submitted between them: cat -v echoes a line at a time, so two
	// characters on one line means no Enter was sent after either.
	if strings.Contains(got, "s\n3") {
		t.Error("something was appended to a key")
	}
}

// The prompt is read from the raw capture, not from what is left after the
// CLI's furniture comes off.
//
// SplitChrome exists to take the input box, the dividers and the status line
// off the screen before it is rendered (MUS-F-0053). A dialog's legend is a
// line of key hints at the bottom of the screen, which is what that function
// is looking for — so reading a prompt out of the body would be reading it out
// of the half the furniture was removed from.
func TestThePromptSurvivesTheChromeSplit(t *testing.T) {
	raw := fixture(t, "prompt-model-picker.txt")
	if ReadPrompt(raw) == nil {
		t.Fatal("the fixture no longer parses at all")
	}
	body, _ := SplitChrome(raw)
	fromBody := ReadPrompt(body)
	if fromBody == nil {
		t.Log("SplitChrome removes the legend, which is why the frame reads the raw capture")
		return
	}
	if len(fromBody.Keys) != len(ReadPrompt(raw).Keys) {
		t.Errorf("the split changed the legend: %d keys against %d", len(fromBody.Keys), len(ReadPrompt(raw).Keys))
	}
}

// An ordinary session screen is not a prompt.
//
// The fixture is a real capture of a running Claude Code pane with no dialog on
// it, taken while the owner reported an empty pop-up. It exists because that
// report had two possible causes and only one was the real one: a false
// positive here would have drawn an empty box just as surely as the CSS bug
// that turned out to be responsible (MUS-F-0087), and nothing distinguished
// them without a screen to try.
//
// The status line is the thing that could plausibly fool the legend reader —
// "auto mode on (shift+tab to cycle) · PR #38 · esc to interrupt · 1 agent"
// has middots and contains "esc to interrupt", which reads exactly like a
// legend entry. It is rejected because every entry on a line must parse, and
// the first one does not.
// ansiPlain is the package's own stripper, named here so the test reads.
func ansiPlain(s string) string { return plainForTest(s) }

func TestAnOrdinarySessionScreenIsNotAPrompt(t *testing.T) {
	screen := fixture(t, "screen-no-prompt.txt")
	if !strings.Contains(ansiPlain(screen), "esc to interrupt") {
		t.Fatal("the fixture has no status line in it, so it cannot prove the status line is rejected")
	}
	if p := ReadPrompt(screen); p != nil {
		t.Errorf("read a prompt off an ordinary screen: %+v", p)
	}
}

// A dialog whose choices are its legend, with no rows at all.
//
// MUS-F-0089. The owner reported the feedback-draft prompt on the pane with no
// pop-up beside it. Two independent reasons, and each alone was enough:
// the box is drawn with a side character on every line, so "│ 1 to review"
// offered a key called "│"; and the parser required numbered rows, which this
// dialog does not have — its choices are the legend.
//
// The fixture is a real capture, taken by a watcher that recorded distinct
// screens until this one appeared, because the session holding the draft is
// working whenever it looks for it (MUS-F-0088).
func TestADialogWhoseChoicesAreItsLegend(t *testing.T) {
	p := ReadPrompt(fixture(t, "prompt-feedback-draft.txt"))
	if p == nil {
		t.Fatal("nothing read off a screen that is asking a question")
	}
	if len(p.Options) != 0 {
		t.Errorf("read %d numbered rows off a dialog that has none: %+v", len(p.Options), p.Options)
	}
	if len(p.Keys) != 3 {
		t.Fatalf("read %d keys, want 1, 2 and 0: %+v", len(p.Keys), p.Keys)
	}
	got := map[string]string{}
	for _, k := range p.Keys {
		got[k.Key] = k.Label
	}
	for key, want := range map[string]string{"1": "review", "2": "send", "0": "dismiss"} {
		if got[key] != want {
			t.Errorf("legend[%q] = %q, want %q", key, got[key], want)
		}
	}
	// The heading comes off the top of the box, without the box and without the
	// glyph the CLI decorates it with.
	if !strings.HasPrefix(p.Title, "Bug report drafted:") {
		t.Errorf("title = %q", p.Title)
	}
	if strings.ContainsAny(p.Title, "│╭╰✻") {
		t.Errorf("the title kept its furniture: %q", p.Title)
	}
	if !strings.Contains(p.Body, "What happened:") {
		t.Errorf("body = %q, want the wrapped description under the heading", p.Body)
	}
	if strings.ContainsAny(p.Body, "│╭╰") {
		t.Errorf("the body kept its furniture: %q", p.Body)
	}
}

// An unboxed legend still needs rows under it.
//
// Dropping the requirement outright would let any sentence with two "x to y"
// clauses in it become a row of buttons. Being inside a box is what says a
// legend is a dialog rather than prose.
func TestALegendOnABareLineIsNotADialogOnItsOwn(t *testing.T) {
	// A legend that genuinely parses, so this is refused for being unboxed and
	// not for being unreadable. The first draft of this test used "press h to
	// help", which fails the entry pattern, and so passed while proving
	// nothing.
	bare := "some prose\n  h to help · q to quit\n"
	if p := ReadPrompt(bare); p != nil {
		t.Errorf("a bare line became a dialog: %+v", p)
	}
	// The same line inside a box is one.
	boxed := "╭────────────╮\n│ Something happened │\n│ h to help · q to quit │\n╰────────────╯\n"
	p := ReadPrompt(boxed)
	if p == nil {
		t.Fatal("a boxed legend with no rows was refused")
	}
	if len(p.Keys) != 2 || p.Title != "Something happened" {
		t.Errorf("boxed dialog read as %+v", p)
	}
}

// A dialog the conversation has moved past is not a dialog.
//
// MUS-F-0092. The pane is 300 rows tall so a transcript has somewhere to live
// (MUS-F-0052), and nothing in it scrolls: what the CLI painted an hour ago is
// still on the screen, pixel for pixel. The feedback-draft box was answered and
// left behind, and the surface went on offering "1 · review · 2 · send · 0 ·
// dismiss" for an hour, over a session that had printed forty more exchanges
// underneath it.
//
// The two fixtures are the same dialog on the same pane, and the only thing
// that differs is what came after it.
func TestADialogTheConversationHasMovedPastIsNotOffered(t *testing.T) {
	live := ReadPrompt(fixture(t, "prompt-feedback-draft.txt"))
	if live == nil {
		t.Fatal("the live dialog stopped being read")
	}
	if len(live.Keys) != 3 {
		t.Errorf("the live dialog lost its keys: %+v", live)
	}

	stale := fixture(t, "prompt-scrolled-past.txt")
	// The same box is still on this screen, character for character.
	if !strings.Contains(plainForTest(stale), "1 to review · 2 to send · 0 to dismiss") {
		t.Fatal("the stale fixture no longer holds the dialog, so it proves nothing")
	}
	if p := ReadPrompt(stale); p != nil {
		t.Errorf("offered a dialog the pane had printed past: %+v", p)
	}
}

// A dialog's options come from inside the dialog.
//
// MUS-F-0100. The owner sent a picture of the pop-up showing an agent's own
// prose as a heading, two of its numbered bullets as options, and the real
// dialog's choices nowhere. The bullets were "1. POST /intake and POST
// /questions have no origin check" and "2. Cf-Access-Authenticated-User-Email
// is trusted unconditionally" — a genuine numbered list in a transcript, two
// hundred and sixty lines above the dialog, matching the row pattern exactly
// because rows were collected from the whole screen above the legend.
func TestOptionsComeFromInsideTheDialogAndNotFromTheTranscript(t *testing.T) {
	p := ReadPrompt(fixture(t, "prompt-below-numbered-prose.txt"))
	if p == nil {
		t.Fatal("the real dialog on this screen was not read at all")
	}
	if p.Title != "Teach auto mode about your environment?" {
		t.Errorf("title = %q, want the dialog's own heading", p.Title)
	}
	if len(p.Options) != 3 {
		t.Fatalf("read %d options, want the dialog's three: %+v", len(p.Options), p.Options)
	}
	for i, want := range []string{"Yes", "Not now", "Don't show again"} {
		if p.Options[i].Label != want {
			t.Errorf("option %d = %q, want %q", i, p.Options[i].Label, want)
		}
	}
	// The cursor is on the first, as the pane draws it.
	if !p.Options[0].Selected {
		t.Error("the cursor row was not read")
	}
	// Nothing from the transcript came with it.
	for _, o := range p.Options {
		if strings.Contains(o.Label, "origin check") || strings.Contains(o.Label, "Cf-Access") {
			t.Errorf("a transcript bullet was offered as an option: %q", o.Label)
		}
	}
	if strings.Contains(p.Title, "Tested against") || strings.Contains(p.Body, "Blockers") {
		t.Errorf("the heading came from the transcript: %q / %q", p.Title, p.Body)
	}
	if len(p.Keys) != 2 {
		t.Errorf("read %d legend keys, want Enter and Esc: %+v", len(p.Keys), p.Keys)
	}
}

// A legend with nothing drawn around it is not a dialog.
//
// The boundary is what says where a dialog begins. Without one there is no way
// to tell its rows from whatever the transcript happens to have numbered, which
// is exactly how MUS-F-0100 happened.
func TestALegendWithNoBoundaryAboveItIsNotADialog(t *testing.T) {
	// Rows and a legend, and nothing enclosing them.
	bare := "some output\n  1. One\n  2. Two\n  Enter to confirm · Esc to cancel\n"
	if p := ReadPrompt(bare); p != nil {
		t.Errorf("read a dialog with no boundary: %+v", p)
	}
	// The same thing under a rule is one.
	ruled := "some output\n────────────\n  Pick one\n  1. One\n  2. Two\n  Enter to confirm · Esc to cancel\n"
	p := ReadPrompt(ruled)
	if p == nil {
		t.Fatal("a ruled dialog was refused")
	}
	if p.Title != "Pick one" || len(p.Options) != 2 {
		t.Errorf("read as %+v", p)
	}
}

// The animated line comes off the output, and the finished ones stay on it.
//
// MUS-F-0098: the CLI moves a glyph by redrawing one row, so its animation
// arrives here as whole frames and renders as a character jumping between
// shapes. The line is taken off and sent as fields instead, so a spinner can
// turn in CSS and the numbers can change without a repaint under them.
func TestTheLiveActivityLineIsFurnitureAndTheFinishedOnesAreNot(t *testing.T) {
	raw := fixture(t, "screen-working.txt")
	body, _ := SplitChrome(raw)
	rest, a := SplitActivity(body)
	if a == nil {
		t.Fatal("the live line was not read")
	}
	if a.Verb != "Zigzagging" || a.For != "3m 40s" || a.Detail != "↓ 11.5k tokens" {
		t.Errorf("read %+v", *a)
	}
	if strings.Contains(plainForTest(rest), "Zigzagging") {
		t.Error("the live line is still in the output, so it would be drawn twice")
	}
	// A finished line is a record of what happened and stays where it is.
	for _, keep := range []string{"Baked for 2m 47s", "Cogitated for 1m 34s"} {
		if !strings.Contains(plainForTest(rest), keep) {
			t.Errorf("a finished line was stripped: %q is gone from the transcript", keep)
		}
	}

	// An idle screen has nothing live on it. Written out rather than taken
	// from screen-no-prompt.txt, which was captured from a session that was
	// working at the time and carries "Transmuting… (30s · ↓ 1.9k tokens)" —
	// it is a fixture for having no *prompt*, and assuming it was idle as well
	// is how this test first failed.
	if _, a := SplitActivity("  ❯ ready\n"); a != nil {
		t.Errorf("read activity off an idle screen: %+v", *a)
	}
	// And a line of that shape further up the transcript is the agent's, not
	// the CLI's: only the tail is furniture.
	buried := "  ✢ Working… (1s · x)\nsomething printed after it\n"
	if _, a := SplitActivity(buried); a != nil {
		t.Errorf("read a line the transcript had printed past: %+v", *a)
	}
}

// A dialog whose rows are toggles, with no numbers on any of them.
//
// MUS-F-0101, the third shape. A heading, a description, a cycler and two
// checkboxes moved between with the arrows, and "←/→ to change usage · Enter to
// continue · Esc to cancel" underneath. It is bounded by a rule like the model
// picker and has no numbered rows like the feedback prompt, so it fell between
// the two tests and the surface showed nothing.
func TestADialogOfTogglesWithNoNumbers(t *testing.T) {
	p := ReadPrompt(fixture(t, "prompt-toggles-no-numbers.txt"))
	if p == nil {
		t.Fatal("nothing read off a dialog that is asking a question")
	}
	if p.Title != "Teach auto mode about your environment?" {
		t.Errorf("title = %q", p.Title)
	}
	// The description is prose and stops where the rows start. It read as one
	// run-on sentence with a cursor in the middle of it before the rows were
	// separated out.
	if !strings.HasSuffix(p.Body, "better decisions.") {
		t.Errorf("body ran into the rows: %q", p.Body)
	}
	if len(p.Options) != 4 {
		t.Fatalf("read %d rows, want four: %+v", len(p.Options), p.Options)
	}
	for _, o := range p.Options {
		if o.Key != "" || o.Sendable {
			t.Errorf("a toggle was offered as a press: %+v", o)
		}
	}
	if !strings.Contains(p.Options[1].Label, "shell history") || !p.Options[1].Selected {
		t.Errorf("the cursor is not on the row the pane has it on: %+v", p.Options)
	}
	// The state is what makes these worth showing at all.
	if !strings.Contains(p.Options[1].Label, "[✔]") || !strings.Contains(p.Options[2].Label, "[ ]") {
		t.Error("the toggles lost the state they carry")
	}

	// The legend names three keys and one of them is a pair, with nothing to
	// send for it.
	if len(p.Keys) != 3 {
		t.Fatalf("read %d legend keys: %+v", len(p.Keys), p.Keys)
	}
	sendable := map[string]bool{}
	for _, k := range p.Keys {
		sendable[k.Key] = k.Sendable
	}
	if sendable["←/→"] {
		t.Error(`"←/→" is offered as a key, and there is nothing to send for it`)
	}
	if !sendable["Enter"] || !sendable["Esc"] {
		t.Errorf("Enter and Esc are not sendable: %+v", p.Keys)
	}
}
