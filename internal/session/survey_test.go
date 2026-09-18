package session

import (
	"strings"
	"testing"
)

// The session survey, MUS-F-0167.
//
// **screen-survey.txt is constructed, not captured.** It is screen-working.txt
// -- a real capture, spinner live -- with the survey inserted where Claude Code
// 2.1.272 draws it, which was read out of that binary rather than off a pane:
// the survey band is rendered after the inline panes and before the input box,
// so it sits under the live line; its column has a margin of one blank line on
// top; the "●" is cyan in a two-column box with the question in bold beside it;
// and the choices are two columns in, one ten-column cell each, the digit cyan.
// Nobody has had a survey on a pane to capture while this was written. When
// one is, it replaces this file.
func surveyRowIndex(t *testing.T, lines []string) int {
	t.Helper()
	for i, l := range lines {
		if strings.Contains(plainForTest(l), "1: Bad") {
			return i
		}
	}
	t.Fatal("the fixture has no survey row, so it proves nothing")
	return -1
}

func TestTheSessionSurveyIsOffered(t *testing.T) {
	raw := fixture(t, "screen-survey.txt")
	p := ReadPrompt(raw)
	if p == nil {
		t.Fatal("the survey was not read")
	}
	if p.Title != "How is Claude doing this session? (optional)" {
		t.Errorf("title is %q", p.Title)
	}
	if p.Body != "" || len(p.Keys) != 0 {
		t.Errorf("read a body or a legend that is not on the screen: %+v", p)
	}
	want := []Choice{
		{Key: "1", Label: "Bad", Sendable: true},
		{Key: "2", Label: "Fine", Sendable: true},
		{Key: "3", Label: "Good", Sendable: true},
		{Key: "0", Label: "Dismiss", Sendable: true},
	}
	if len(p.Options) != len(want) {
		t.Fatalf("options are %+v", p.Options)
	}
	for i := range want {
		if p.Options[i] != want[i] {
			t.Errorf("option %d is %+v, want %+v", i, p.Options[i], want[i])
		}
	}

	// The variant the plugin and memory surveys draw, with Unsure before
	// Dismiss.
	unsure := strings.Replace(raw, "\x1b[36m0\x1b[39m: Dismiss",
		"\x1b[36m4\x1b[39m: Unsure    \x1b[36m0\x1b[39m: Dismiss", 1)
	if u := ReadPrompt(unsure); u == nil || len(u.Options) != 5 ||
		u.Options[3] != (Choice{Key: "4", Label: "Unsure", Sendable: true}) {
		t.Errorf("the Unsure variant read as %+v", u)
	}
}

// A survey the conversation has printed past is not offered.
//
// The same rule as MUS-F-0091: what separates a live prompt from a dead one on
// a pane that never scrolls is whether anything is painted below it.
func TestASurveyWithTheConversationBelowItIsNotOffered(t *testing.T) {
	lines := strings.Split(fixture(t, "screen-survey.txt"), "\n")
	at := surveyRowIndex(t, lines)
	moved := append(append(append([]string{}, lines[:at+1]...),
		"",
		"\x1b[38;5;231m●\x1b[39m Something the agent said after the survey was drawn."),
		lines[at+1:]...)
	if p := ReadPrompt(strings.Join(moved, "\n")); p != nil {
		t.Errorf("offered a survey the pane had printed past: %+v", p)
	}
}

// Lines that look like the row and are not.
func TestProseWithDigitsAndColonsIsNotASurvey(t *testing.T) {
	tail := "\n────────────────────────────────────────\n❯ \n"
	for _, body := range []string{
		// No Dismiss: the component always ends with it.
		"● How is Claude doing this session? (optional)\n  1: Bad    2: Fine   3: Good\n",
		// A word before the first cell.
		"● Plan\n  Step 1: Bad    2: Fine   0: Dismiss\n",
		// No "●" in the first column above it.
		"Some output\n  1: Bad    2: Fine   3: Good   0: Dismiss\n",
		// Not indented.
		"● How is Claude doing this session? (optional)\n1: Bad    2: Fine   3: Good   0: Dismiss\n",
	} {
		if p := ReadPrompt(body + tail); p != nil {
			t.Errorf("read a survey out of %q: %+v", body, p)
		}
	}
}

// The spinner is still read with the survey drawn under it.
//
// The second half of MUS-F-0167: the survey's row was the last line of the
// body, so the live line above it was taken for something printed past.
func TestTheLiveLineIsReadAboveTheSurvey(t *testing.T) {
	raw := fixture(t, "screen-survey.txt")
	body, _ := SplitChrome(raw)
	rest, a := SplitActivity(body)
	if a == nil {
		t.Fatal("the live line was not read with a survey under it")
	}
	if a.Verb != "Zigzagging" || a.For != "3m 40s" || a.Detail != "↓ 11.5k tokens" {
		t.Errorf("read %+v", *a)
	}
	plain := plainForTest(rest)
	if strings.Contains(plain, "Zigzagging") {
		t.Error("the live line is still in the output, so it would be drawn twice")
	}
	for _, keep := range []string{"How is Claude doing this session?", "0: Dismiss"} {
		if !strings.Contains(plain, keep) {
			t.Errorf("the survey was taken off the terminal: %q is gone", keep)
		}
	}
	if DoingIn(raw) != AgentWorking {
		t.Errorf("the pill reads %v with a turn in flight", DoingIn(raw))
	}

	// With no live line, a survey does not make one up.
	lines := strings.Split(raw, "\n")
	var idle []string
	for _, l := range lines {
		if !strings.Contains(plainForTest(l), "Zigzagging") {
			idle = append(idle, l)
		}
	}
	idleBody, _ := SplitChrome(strings.Join(idle, "\n"))
	if _, a := SplitActivity(idleBody); a != nil {
		t.Errorf("read activity off a screen with no live line: %+v", *a)
	}
	// And a live line with the conversation printed under it stays dead even
	// when a survey follows: only the band directly under the live line is
	// looked past.
	at := surveyRowIndex(t, lines)
	var buried []string
	for i, l := range lines {
		if i == at-2 { // the blank above the "●"
			buried = append(buried, "something printed after the live line")
		}
		buried = append(buried, l)
	}
	buriedBody, _ := SplitChrome(strings.Join(buried, "\n"))
	if _, a := SplitActivity(buriedBody); a != nil {
		t.Errorf("read a live line the transcript had printed past: %+v", *a)
	}
}
