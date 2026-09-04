package session

import (
	"regexp"
	"strings"

	"github.com/DevOfPie/Mustur/internal/ansi"
)

// A Prompt is a selection the CLI is waiting on, read off the pane.
//
// Read rather than known. MUS-D-0142: the surface does not understand what a
// dialog means, it reads what the screen says is on offer — the numbered rows,
// and the legend of keys the CLI prints in its own words on the last line. A
// dialog that changes its keys changes its legend in the same edit.
type Prompt struct {
	Title string // The dialog's own heading, when it has one.
	// Body is the sentence under the heading, unwrapped. The CLI wraps it to
	// the pane's width, which is not the reader's, so the lines are joined back
	// into one and left for the surface to wrap again.
	Body    string
	Options []Choice // Numbered rows, in the order they appear.
	Keys    []Choice // The legend: what the CLI says every other key does.
}

// A Choice is one thing the pane says can be pressed, and what pressing it does.
//
// Key is what to send: a digit for a numbered row, and whatever the legend
// named for a legend entry — "Enter", "Esc", or a single character like "s".
type Choice struct {
	Key      string
	Label    string
	Selected bool // Only ever set on a numbered row: the one the cursor is on.
}

var (
	// A numbered row, with the cursor optional and before the number. The
	// label runs to the end; the CLI pads a second column with spaces and
	// nothing here tries to split it, because two spaces is a layout detail
	// and the whole label is what a button should say anyway.
	numbered = regexp.MustCompile(`^\s*(?:([^\s\d])\s*)?(\d+)\.\s+(\S.*?)\s*$`)
	// One entry of the legend: a key, the word "to", and what it does.
	legendOne = regexp.MustCompile(`^(\S+) to (\S.*)$`)
)

// ReadPrompt finds a selection prompt on a captured screen.
//
// It returns nil when there is nothing it can read, which is the important
// case: a pane with no legend gets no controls and keeps its terminal, and
// nothing is guessed (MUS-D-0142). Every failure here is that one.
func ReadPrompt(screen string) *Prompt {
	lines := strings.Split(ansi.Plain(screen), "\n")

	// The legend anchors everything. Searched from the bottom, because the
	// pane's own footer is below it and a dialog is the last thing drawn.
	legendAt, keys := -1, []Choice(nil)
	for i := len(lines) - 1; i >= 0; i-- {
		if k := readLegend(lines[i]); len(k) > 0 {
			legendAt, keys = i, k
			break
		}
	}
	if legendAt < 0 {
		return nil
	}

	// Numbered rows above it. Anything else between them is ignored rather
	// than treated as an end: the model picker has an unnumbered line of its
	// own between the last option and the legend.
	var opts []Choice
	first := legendAt
	for i := 0; i < legendAt; i++ {
		m := numbered.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		if len(opts) == 0 {
			first = i
		}
		opts = append(opts, Choice{
			Key:      m[2],
			Label:    strings.TrimSpace(m[3]),
			Selected: m[1] != "",
		})
	}
	if len(opts) == 0 {
		return nil
	}

	title, body := heading(lines, first)
	return &Prompt{Title: title, Body: body, Options: opts, Keys: keys}
}

// readLegend parses a line like "Enter to set as default · s to use this
// session only · Esc to cancel".
//
// Two entries at least. One "X to Y" on a line is a sentence, and a dialog's
// legend has never been a single key — requiring two is what stops an ordinary
// line of output being read as a footer.
func readLegend(line string) []Choice {
	line = strings.TrimSpace(line)
	if !strings.Contains(line, "·") {
		return nil
	}
	var out []Choice
	for _, part := range strings.Split(line, "·") {
		m := legendOne.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			return nil // One unreadable entry and the whole line is not a legend.
		}
		out = append(out, Choice{Key: m[1], Label: strings.TrimSpace(m[2])})
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

// heading is the dialog's title and the sentence under it.
//
// Taken from the top of the block down, not from the options up. The first
// version walked up from the first option and took the nearest non-empty line,
// which on a real picker is the last line of a wrapped description — so the
// title came back as "other/previous model names, specify with --model." The
// block's own rule is the boundary; the first line after it is the heading and
// everything between that and the options is the description, unwrapped.
func heading(lines []string, first int) (string, string) {
	// Where the block starts: the nearest rule above the options, or as far
	// back as this is willing to look.
	start := first - maxHead
	if start < 0 {
		start = 0
	}
	for i := first - 1; i >= start; i-- {
		if isRule(strings.TrimSpace(lines[i])) {
			start = i + 1
			break
		}
	}

	var title string
	var body []string
	for i := start; i < first; i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || isRule(t) {
			continue
		}
		if title == "" {
			title = t
			continue
		}
		body = append(body, t)
	}
	return title, strings.Join(body, " ")
}

// How far above the options a heading may be. Enough for a wrapped paragraph
// and not so much that an unrelated line of output becomes a title.
const maxHead = 12

// isRule reports a line made only of the box-drawing characters the CLI rules
// its dialogs with.
//
// An empty line is not a rule. It read as one in the first version -- both trim
// to nothing -- so the blank the CLI leaves between its description and its
// first option became the top of the block, and the title and description were
// never looked at.
func isRule(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	return strings.TrimLeft(s, "▔▁─━═▂▃▄▅▆▇█ ") == ""
}
