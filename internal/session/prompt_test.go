package session

import (
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
