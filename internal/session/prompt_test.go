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
	if len(p.Options) != 3 {
		t.Fatalf("read %d options, want 3: %+v", len(p.Options), p.Options)
	}
	for i, want := range []string{"1", "2", "3"} {
		if p.Options[i].Key != want {
			t.Errorf("option %d sends %q, want %q", i, p.Options[i].Key, want)
		}
	}
	if !strings.HasPrefix(p.Options[0].Label, "Default (recommended)") {
		t.Errorf("first label = %q", p.Options[0].Label)
	}
	// The cursor marks one row and only one.
	var selected []string
	for _, o := range p.Options {
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
