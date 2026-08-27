package session

import (
	"strings"
	"testing"
)

// The two shapes seen on real panes, copied from them rather than imagined.
const (
	// A session at rest, with everything the CLI draws.
	demoPane = "  Which do you want?\n" +
		"\n" +
		"✻ Sautéed for 4m 5s\n" +
		"                                        new task? /clear to save 118.3k tokens\n" +
		"────────────────────────────────────────────────── mustur milestone resolution ─\n" +
		"❯ Leave it alone\n" +
		"────────────────────────────────────────────────────────────────────────────────\n" +
		"  ⏵⏵ auto mode on (shift+tab to cycle) · PR #31 · ← 1 agent         /rc failed\n"

	// The same CLI with a modal open: no lower divider and no status line at
	// all, and an update notice where the hint would be.
	ringPane = "│ 1 to review · 2 to send · 0 to dismiss                                       │\n" +
		"╰──────────────────────────────────────────────────────────────────────────────╯\n" +
		"\n" +
		"                                        ✔ Update installed · Restart to update\n" +
		"────────────────────────────────────────────────── mustur milestone resolution ─\n" +
		"❯ name three animals, one word each\n"
)

func TestSplitChromeOnARealPane(t *testing.T) {
	body, st := SplitChrome(demoPane)

	// What the session said stays.
	for _, want := range []string{"Which do you want?", "Sautéed for 4m 5s"} {
		if !strings.Contains(body, want) {
			t.Errorf("the body lost %q:\n%s", want, body)
		}
	}
	// What the CLI drew goes.
	for _, gone := range []string{"❯", "shift+tab", "mustur milestone resolution", "/clear to save"} {
		if strings.Contains(body, gone) {
			t.Errorf("the body still carries the CLI's furniture: %q\n%s", gone, body)
		}
	}

	if st.Mode != "auto mode on" {
		t.Errorf("mode is %q", st.Mode)
	}
	if strings.Join(st.Items, "|") != "PR #31|← 1 agent" {
		t.Errorf("items are %q", st.Items)
	}
	if st.Note != "/rc failed" {
		t.Errorf("note is %q", st.Note)
	}
	if st.Hint != "new task? /clear to save 118.3k tokens" {
		t.Errorf("hint is %q", st.Hint)
	}
	if st.Update != "" {
		t.Errorf("an update was invented: %q", st.Update)
	}
}

// A modal open means no status line and no lower divider. The caret is the one
// thing both shapes have, which is why it is the anchor.
func TestSplitChromeWithAModalOpen(t *testing.T) {
	body, st := SplitChrome(ringPane)

	if !strings.Contains(body, "1 to review") {
		t.Errorf("the body lost the modal:\n%s", body)
	}
	for _, gone := range []string{"❯ name three animals", "mustur milestone resolution", "Update installed"} {
		if strings.Contains(body, gone) {
			t.Errorf("the body still carries %q:\n%s", gone, body)
		}
	}
	if st.Update != "Update installed · Restart to update" {
		t.Errorf("update is %q", st.Update)
	}
	if st.Hint != "" {
		t.Errorf("the update was also read as a hint: %q", st.Hint)
	}
	if st.Mode != "" || len(st.Items) != 0 {
		t.Errorf("a status line was invented: %q %q", st.Mode, st.Items)
	}
}

// A pane this does not recognise comes back whole. Showing everything is a much
// better failure than guessing which lines to delete.
func TestSplitChromeLeavesAnUnknownPaneAlone(t *testing.T) {
	for _, in := range []string{
		"$ make check\nok  1205 links resolve\nok  go build, vet and test\n",
		"",
		"just one line",
		"───────────────────────────────────────────\nno caret anywhere\n",
	} {
		body, st := SplitChrome(in)
		if body != in {
			t.Errorf("an unrecognised pane was cut:\n in %q\nout %q", in, body)
		}
		if !st.Empty() {
			t.Errorf("a status was read out of %q: %+v", in, st)
		}
	}
}

// The caret can appear in a transcript. The last one is the input box.
func TestSplitChromeAnchorsOnTheLastCaret(t *testing.T) {
	in := "I ran ❯ echo hi and it worked\nmore output\n" +
		"────────────────────────────────────────────────── something ─\n" +
		"❯ \n"
	body, _ := SplitChrome(in)
	if !strings.Contains(body, "I ran ❯ echo hi") {
		t.Errorf("a quoted caret was taken for the input box:\n%s", body)
	}
	if strings.Contains(body, "something ─") {
		t.Errorf("the real box survived:\n%s", body)
	}
}

// The gap a tall pane leaves between the transcript and the box becomes
// trailing blanks once the box is gone, which is what makes a tall pane usable.
func TestSplitChromeLeavesTheGapAsTrailingBlanks(t *testing.T) {
	in := "said something\n" + strings.Repeat("\n", 40) +
		"                                        new task? /clear to save 1k tokens\n" +
		"────────────────────────────────────────────────── a title ─\n" +
		"❯ \n"
	body, _ := SplitChrome(in)
	if got := trimBlank(body); got != "said something" {
		t.Errorf("the gap did not trim away: %q", got)
	}
}

// A hyperlink in the status line survives being recognised.
//
// The CLI links "PR #31" with an OSC 8 sequence. The first version of the
// stripper scanned to the next "m", which an OSC has none of, so it ate the
// link and left a fragment of the URL — and the chip read
// "PR /DevOfPie/Mustur/pull/31". The same mangling broke divider and caret
// detection, which is how a whole pane's furniture stayed in the output.
func TestSplitChromeReadsThroughAHyperlink(t *testing.T) {
	link := "\x1b]8;id=1;https://github.com/DevOfPie/Mustur/pull/31\x1b\\PR #31\x1b]8;;\x1b\\"
	in := "output above\n" +
		"────────────────────────────────────────────────── a title ─\n" +
		"❯ \n" +
		"────────────────────────────────────────────────────────────\n" +
		"  ⏵⏵ auto mode on (shift+tab to cycle) · " + link + " · ← 1 agent\n"

	body, st := SplitChrome(in)
	if strings.Contains(body, "auto mode on") {
		t.Errorf("the status line stayed in the output:\n%s", body)
	}
	if st.Mode != "auto mode on" {
		t.Errorf("mode is %q", st.Mode)
	}
	if strings.Join(st.Items, "|") != "PR #31|← 1 agent" {
		t.Errorf("items are %q; a hyperlink was read as its own URL", st.Items)
	}
}
