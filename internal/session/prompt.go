package session

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/DevOfPie/Mustur/internal/ansi"
)

// A Prompt is a selection the CLI is waiting on, read off the pane.
//
// Read rather than known. MUS-D-0142: the surface does not understand what a
// dialog means, it reads what the screen says is on offer — the numbered rows,
// and the legend of keys the CLI prints in its own words on the last line. A
// dialog that changes its keys changes its legend in the same edit.
type Prompt struct {
	Title string `json:"title,omitempty"` // The dialog's own heading, when it has one.
	// Body is the sentence under the heading, unwrapped. The CLI wraps it to
	// the pane's width, which is not the reader's, so the lines are joined back
	// into one and left for the surface to wrap again.
	Body    string   `json:"body,omitempty"`
	Options []Choice `json:"options,omitempty"` // Numbered rows, in order.
	Keys    []Choice `json:"keys,omitempty"`    // The legend, in the CLI's words.
}

// A Choice is one thing the pane says can be pressed, and what pressing it does.
//
// Key is what to send: a digit for a numbered row, and whatever the legend
// named for a legend entry — "Enter", "Esc", or a single character like "s".
type Choice struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Selected bool   `json:"selected,omitempty"` // The row the cursor is on.
	// Sendable is whether pressing this is something Mustur can do. A legend
	// names keys in the CLI's own words and some of them are not one key at
	// all -- "←/→ to change usage" is a pair, and there is nothing to send for
	// it. A surface that drew a button there would draw one that fails
	// (MUS-F-0101), so the server says which are real and the rest are text.
	Sendable bool `json:"sendable,omitempty"`
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

// The characters the CLI rules and boxes its dialogs with.
//
// Two shapes have been seen and they are drawn differently: the model picker
// sits under a plain ▔ rule, and the feedback-draft prompt is inside a rounded
// box whose sides are on every line. Both are furniture, and a parser that does
// not take it off reads "│ 1 to review" and finds no key called "│".
const boxChars = "╭╮╰╯┌┐└┘─━│┃▔▁═╌╍┄┅ "

// unbox strips the box a line may be drawn inside, and reports whether there
// was one.
//
// Whether there was one matters: a legend inside a box is a dialog even when it
// has no numbered rows beneath it, and a legend on a bare line is only a dialog
// when rows follow. That is the whole of what keeps an ordinary sentence
// containing "· x to y · z to w" from being offered as buttons.
func unbox(line string) (string, bool) {
	t := strings.TrimSpace(line)
	stripped := strings.Trim(t, boxChars)
	return strings.TrimSpace(stripped), stripped != t
}

// ReadPrompt finds a selection prompt on a captured screen.
//
// It returns nil when there is nothing it can read, which is the important
// case: a pane with no legend gets no controls and keeps its terminal, and
// nothing is guessed (MUS-D-0142). Every failure here is that one.
func ReadPrompt(screen string) *Prompt {
	lines := strings.Split(ansi.Plain(screen), "\n")

	// The legend anchors everything. Searched from the bottom, because the
	// pane's own footer is below it and a dialog is the last thing drawn.
	legendAt, keys, boxed := -1, []Choice(nil), false
	for i := len(lines) - 1; i >= 0; i-- {
		if k, b := readLegend(lines[i]); len(k) > 0 {
			legendAt, keys, boxed = i, k, b
			break
		}
	}
	if legendAt < 0 {
		return nil
	}

	// Where the dialog starts.
	//
	// A dialog is drawn inside something: a rule above it, or a box around it.
	// Finding that boundary first is what stops the rows being collected from
	// the whole screen, which is what they were -- an agent's own numbered
	// prose, two hundred and sixty lines above the dialog, came back as its
	// options, and the heading came from whatever sentence sat above that
	// (MUS-F-0100).
	//
	// No boundary means this is not a dialog. Both shapes that are have one:
	// the model picker sits under a rule, the feedback prompt inside a box.
	blockStart := -1
	for i := legendAt - 1; i >= 0 && i > legendAt-maxBlock; i-- {
		if isRule(strings.TrimSpace(lines[i])) {
			blockStart = i + 1
			break
		}
	}
	if blockStart < 0 {
		return nil
	}

	// The rows inside it, and where they start.
	//
	// A dialog's rows are not always numbered. One shape has toggles instead --
	// "Also scan shell history [✔]", "How you use Claude here ◀ Mixed ▶" --
	// which are moved between with the arrows rather than pressed by name. They
	// are still rows, and joining them into the description turned a set of
	// switches into a run-on sentence with a cursor in the middle of it
	// (MUS-F-0101).
	//
	// So the first row-shaped line ends the description, and everything from
	// there to the legend is a row. A wrapped sentence after that point would
	// be read as one, which is visible on the surface rather than silent.
	var opts []Choice
	first := legendAt
	for i := blockStart; i < legendAt; i++ {
		row, _ := unbox(lines[i])
		if strings.TrimSpace(row) == "" {
			continue
		}
		if first == legendAt && !rowish(row) {
			continue
		}
		if first == legendAt {
			first = i
		}
		if m := numbered.FindStringSubmatch(row); m != nil {
			opts = append(opts, Choice{
				Key: m[2], Label: strings.TrimSpace(m[3]),
				Selected: m[1] != "", Sendable: true,
			})
			continue
		}
		// Unnumbered: there is nothing to press, so it carries its state and
		// says so by being unsendable. The cursor still marks which one the
		// arrows are on.
		label, sel := strings.TrimSpace(row), false
		if c := []rune(label); len(c) > 0 && (c[0] == '❯' || c[0] == '>') {
			label, sel = strings.TrimSpace(string(c[1:])), true
		}
		opts = append(opts, Choice{Label: label, Selected: sel})
	}
	// Rows are not what makes it a dialog; the boundary above it is.
	//
	// This asked for numbered rows unless the legend was boxed, which refused a
	// third shape the CLI draws: a heading, a description, a set of toggles
	// with no numbers on them, and "←/→ to change usage · Enter to continue ·
	// Esc to cancel" underneath (MUS-F-0101). It is bounded by a rule like the
	// model picker and has no rows like the feedback prompt, so it fell between
	// the two tests.
	//
	// The boundary is the guard now and it is a stronger one: it is what
	// stopped an agent's numbered prose being read as options, and a legend
	// with nothing drawn above it is refused before this is reached.
	_ = boxed

	// And it has to still be the thing the pane is asking.
	//
	// Mustur runs a session in a pane 300 rows tall, because an agent CLI keeps
	// no scrollback and a tall pane is the only place a transcript can live
	// (MUS-F-0052). Nothing scrolls: the CLI paints down the screen and what it
	// painted an hour ago is still there. So a dialog that was answered, or
	// simply moved past, stays on the screen exactly as it looked -- and the
	// surface offered its buttons for an hour after the CLI stopped listening
	// (MUS-F-0092).
	//
	// What separates the two is what is under it. A live dialog is the last
	// thing on the transcript; a dead one has the conversation that came after
	// it printed below. Measured on the pane that produced this: the live one
	// sat one line from the end, the dead one a hundred and thirty-nine.
	if paintedBelow(screen, lines, legendAt) {
		return nil
	}

	title, body := heading(lines, blockStart, first)
	return &Prompt{Title: title, Body: body, Options: opts, Keys: keys}
}

// rowish reports whether a line is the first of a dialog's rows rather than
// more of its description.
//
// Three shapes say so: a number, the cursor, and a state marker on the end of
// the line. Anything else is prose until one of them appears.
func rowish(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	if numbered.MatchString(t) {
		return true
	}
	if r := []rune(t); r[0] == '❯' || r[0] == '>' {
		return true
	}
	return stateMarker.MatchString(t)
}

// stateMarker is a switch or a cycler drawn at the end of a row: "[✔]", "[ ]",
// "◀ Mixed ▶". Vendor-specific, and named rather than guessed at.
var stateMarker = regexp.MustCompile(`(\[[^\]]?\]|◀[^▶]*▶)\s*$`)

// readLegend parses a line like "Enter to set as default · s to use this
// session only · Esc to cancel".
//
// Two entries at least. One "X to Y" on a line is a sentence, and a dialog's
// legend has never been a single key — requiring two is what stops an ordinary
// line of output being read as a footer.
func readLegend(line string) ([]Choice, bool) {
	line, boxed := unbox(line)
	if !strings.Contains(line, "·") {
		return nil, false
	}
	var out []Choice
	for _, part := range strings.Split(line, "·") {
		m := legendOne.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			return nil, false // One unreadable entry, and the line is not a legend.
		}
		out = append(out, Choice{Key: m[1], Label: strings.TrimSpace(m[2]), Sendable: Sendable(m[1])})
	}
	if len(out) < 2 {
		return nil, false
	}
	return out, boxed
}

// heading is the dialog's title and the sentence under it.
//
// Taken from the top of the block down, not from the options up. The first
// version walked up from the first option and took the nearest non-empty line,
// which on a real picker is the last line of a wrapped description — so the
// title came back as "other/previous model names, specify with --model." The
// block's own rule is the boundary; the first line after it is the heading and
// everything between that and the options is the description, unwrapped.
func heading(lines []string, start, first int) (string, string) {
	var title string
	var body []string
	for i := start; i < first; i++ {
		t, _ := unbox(lines[i])
		if t == "" || isRule(t) {
			continue
		}
		// The CLI marks a dialog's first line with a glyph of its own. It is
		// decoration, and a heading that starts with it reads as a typo.
		t = strings.TrimSpace(strings.TrimLeft(t, "✻✽✳✶●◆*"))
		if t == "" {
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

// How far above the legend a dialog may start. Enough for a heading, a wrapped
// paragraph and a dozen options, and short enough that the boundary found is
// the dialog's own rather than some rule further up the transcript.
const maxBlock = 40

// paintedBelow reports whether the CLI wrote anything under the legend.
//
// The transcript is what SplitChrome leaves once the CLI's own furniture -- its
// input box, its dividers, its status line -- has been taken off the bottom.
// That body is a prefix of the screen, so a line's index means the same in
// both, and anything after the legend and inside the body is output the pane
// produced after the dialog was drawn.
//
// Blanks and the dialog's own closing rule do not count. They are what sits
// under a live dialog.
func paintedBelow(screen string, lines []string, legendAt int) bool {
	body, _ := SplitChrome(screen)
	end := len(strings.Split(ansi.Plain(body), "\n"))
	if end > len(lines) {
		end = len(lines)
	}
	for i := legendAt + 1; i < end; i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || isRule(t) {
			continue
		}
		return true
	}
	return false
}

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
	return strings.TrimLeft(s, boxChars+"▂▃▄▅▆▇█") == ""
}

// Sendable reports whether SendChoice would accept this key.
//
// The same two tests SendChoice makes, without a session to make them against,
// so a surface can tell a key it can press from a word the CLI used to describe
// two of them.
func Sendable(key string) bool {
	k := strings.TrimSpace(key)
	name := strings.ToLower(k)
	if name == "esc" {
		name = "escape"
	}
	if _, ok := keys[name]; ok {
		return true
	}
	r := []rune(k)
	return len(r) == 1 && unicode.IsPrint(r[0]) && !unicode.IsSpace(r[0])
}

// SendChoice presses what a Choice says to press.
//
// A prompt's keys come off the pane in the CLI's own words, so they are two
// different things wearing one field: a name the allowlist knows -- Enter, Esc,
// an arrow -- or a single character the legend named, like the "s" in "s to use
// this session only". Nothing about a numbered menu suggests a letter key,
// which is why the legend is read rather than assumed (MUS-F-0083), and why
// this cannot be the named allowlist alone.
//
// A single character goes in with send-keys -l, which sends it literally and
// interprets no names at all -- so "C-c" as a literal would type three
// characters rather than interrupting. That is what makes accepting a character
// safe where accepting a name would not be. It is still bounded: exactly one
// rune, printable, and not a space.
func (a *Adapter) SendChoice(ctx context.Context, project, key string) error {
	k := strings.TrimSpace(key)
	if k == "" {
		return fmt.Errorf("no key to press")
	}
	// A name the allowlist knows, however the CLI capitalised it. "Esc" is the
	// legend's spelling and "escape" is the allowlist's.
	name := strings.ToLower(k)
	if name == "esc" {
		name = "escape"
	}
	if _, ok := keys[name]; ok {
		return a.SendKey(ctx, project, name)
	}

	r := []rune(k)
	if len(r) != 1 || !unicode.IsPrint(r[0]) || unicode.IsSpace(r[0]) {
		return fmt.Errorf("%q is neither a key this may send nor a single character", key)
	}
	target, err := NameFor(project)
	if err != nil {
		return err
	}
	live, err := a.Alive(ctx, project)
	if err != nil {
		return err
	}
	if !live {
		return fmt.Errorf("%s has no session Mustur started", project)
	}
	// -l is literal and -- ends the flags, so a key that is "-" is a hyphen and
	// not the start of an option.
	if out, err := a.runner().Run(ctx, "tmux", "send-keys", "-t", target, "-l", "--", k); err != nil {
		return fmt.Errorf("tmux send-keys -l %q: %w: %s", k, err, strings.TrimSpace(out))
	}
	return nil
}

// plainForTest exposes the same stripping ReadPrompt does, so a test can assert
// what is on a screen without keeping a second copy of the rule.
func plainForTest(s string) string { return ansi.Plain(s) }
