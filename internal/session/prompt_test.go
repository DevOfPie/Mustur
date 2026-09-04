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
