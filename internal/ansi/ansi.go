// Package ansi turns a captured terminal screen into HTML.
//
// It exists because of MUS-F-0049. Mustur used to stream the pane's raw byte
// protocol and append it to a log, and a third of those bytes are cursor
// addressing — ESC[21;3H, ESC[H, ESC[K — which is a protocol for painting a
// grid rather than a transcript with decoration on it. tmux is a terminal
// emulator and had already turned that into a screen; Mustur threw it away and
// re-derived it badly.
//
// So the screen now comes from `tmux capture-pane -p -e`, which is the pane as
// tmux has already assembled it. What is left in it is SGR — colour, bold,
// underline — and nothing that moves a cursor. That is what this reads.
//
// **It renders what it understands and drops what it does not.** An escape
// sequence this does not know is removed rather than printed, because printing
// it is exactly the defect this package exists to fix. Text is escaped for
// HTML on the way through; nothing from the pane reaches the page as markup.
package ansi

import (
	"fmt"
	"strconv"
	"strings"
)

// palette maps the eight ANSI colours, and their bright forms, to something
// legible on a page that follows the reader's system theme.
//
// The terminal palette assumes a terminal: white on black, or black on white,
// with the extremes carrying meaning. A page that is light for one reader and
// dark for the next cannot use the extremes at all — white text on white is
// nothing, and so is black on black. So white and black both become
// currentColor, which is whatever the page's own text is, and the six hues in
// between are chosen mid-tone so they hold contrast either way.
var palette = map[int]string{
	1: "#c0392b", 2: "#1e8449", 3: "#b9770e", 4: "#2471a3", 5: "#8e44ad", 6: "#117a65",
	9: "#e74c3c", 10: "#27ae60", 11: "#d4ac0d", 12: "#3498db", 13: "#af7ac5", 14: "#17a589",
}

// colour renders one SGR colour index, or "" for the two that must follow the
// page rather than the terminal.
func colour(n int) string {
	if c, ok := palette[n%8+bright(n)]; ok {
		return c
	}
	return ""
}

func bright(n int) int {
	if n >= 8 {
		return 8
	}
	return 0
}

// state is the run of attributes currently in force.
type state struct {
	fg, bg            string
	bold, dim         bool
	italic, underline bool
	reverse           bool
}

func (s state) empty() bool {
	return s == state{}
}

// style renders the state as one inline style, or "" when nothing is set.
func (s state) style() string {
	var b strings.Builder
	fg, bg := s.fg, s.bg
	if s.reverse {
		// Reverse video with no colours set means the page's own two, swapped.
		if fg == "" && bg == "" {
			fg, bg = "Canvas", "CanvasText"
		} else {
			fg, bg = bg, fg
		}
	}
	if fg != "" {
		fmt.Fprintf(&b, "color:%s;", fg)
	}
	if bg != "" {
		fmt.Fprintf(&b, "background:%s;", bg)
	}
	if s.bold {
		b.WriteString("font-weight:600;")
	}
	if s.dim {
		b.WriteString("opacity:.65;")
	}
	if s.italic {
		b.WriteString("font-style:italic;")
	}
	if s.underline {
		b.WriteString("text-decoration:underline;")
	}
	return b.String()
}

// HTML converts a captured screen into HTML.
//
// The result is a sequence of text and span elements. Every character from the
// pane is HTML-escaped; the only markup in the output is what this writes.
func HTML(screen string) string {
	var out strings.Builder
	var cur state
	open := false

	flush := func() {
		if open {
			out.WriteString("</span>")
			open = false
		}
	}
	begin := func() {
		if open || cur.empty() {
			return
		}
		if s := cur.style(); s != "" {
			out.WriteString(`<span style="` + s + `">`)
			open = true
		}
	}

	for i := 0; i < len(screen); {
		c := screen[i]
		if c != 0x1b {
			// Plain text. Escaped, always: the pane's contents are somebody
			// else's output and must never be markup.
			switch c {
			case '&':
				begin()
				out.WriteString("&amp;")
			case '<':
				begin()
				out.WriteString("&lt;")
			case '>':
				begin()
				out.WriteString("&gt;")
			case '\n':
				// A newline ends the run: a style that spans lines makes a
				// background bleed across the whole width of the pane.
				flush()
				out.WriteByte('\n')
			default:
				begin()
				out.WriteByte(c)
			}
			i++
			continue
		}

		// An escape. Only SGR is understood; everything else is dropped,
		// because leaving it in is the defect.
		params, final, n := parseCSI(screen[i:])
		if n == 0 {
			// Not a CSI at all — an OSC hyperlink, or a stray ESC. Skip to the
			// terminator rather than printing the payload.
			i += skipEscape(screen[i:])
			continue
		}
		i += n
		if final != 'm' {
			continue
		}
		flush()
		cur = apply(cur, params)
	}
	flush()
	return out.String()
}

// Plain strips every escape sequence, leaving the text a reader would see.
//
// Used where something has to be recognised rather than rendered — a divider,
// a caret, a status line. It shares parseCSI and skipEscape with HTML rather
// than approximating them, because approximating them is what went wrong: a
// hand-rolled version that scanned to the next "m" ate an OSC 8 hyperlink whole
// and turned "PR #31" into a fragment of its own URL.
func Plain(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			out.WriteByte(s[i])
			i++
			continue
		}
		if _, _, n := parseCSI(s[i:]); n > 0 {
			i += n
			continue
		}
		i += skipEscape(s[i:])
	}
	return out.String()
}

// apply folds one SGR sequence into the running state.
func apply(s state, params []int) state {
	if len(params) == 0 {
		return state{}
	}
	for i := 0; i < len(params); i++ {
		switch p := params[i]; {
		case p == 0:
			s = state{}
		case p == 1:
			s.bold = true
		case p == 2:
			s.dim = true
		case p == 3:
			s.italic = true
		case p == 4:
			s.underline = true
		case p == 7:
			s.reverse = true
		case p == 22:
			s.bold, s.dim = false, false
		case p == 23:
			s.italic = false
		case p == 24:
			s.underline = false
		case p == 27:
			s.reverse = false
		case p >= 30 && p <= 37:
			s.fg = colour(p - 30)
		case p == 39:
			s.fg = ""
		case p >= 40 && p <= 47:
			s.bg = colour(p - 40)
		case p == 49:
			s.bg = ""
		case p >= 90 && p <= 97:
			s.fg = colour(p - 90 + 8)
		case p >= 100 && p <= 107:
			s.bg = colour(p - 100 + 8)
		case p == 38 || p == 48:
			c, used := extended(params[i:])
			if p == 38 {
				s.fg = c
			} else {
				s.bg = c
			}
			i += used
		}
	}
	return s
}

// extended reads 38;5;n, 38;2;r;g;b and their background forms, returning the
// colour and how many parameters it consumed beyond the first.
func extended(params []int) (string, int) {
	if len(params) < 2 {
		return "", 0
	}
	switch params[1] {
	case 5:
		if len(params) < 3 {
			return "", 1
		}
		return xterm(params[2]), 2
	case 2:
		if len(params) < 5 {
			return "", len(params) - 1
		}
		return fmt.Sprintf("#%02x%02x%02x", clamp(params[2]), clamp(params[3]), clamp(params[4])), 4
	}
	return "", 1
}

func clamp(n int) int {
	if n < 0 {
		return 0
	}
	if n > 255 {
		return 255
	}
	return n
}

// xterm renders one of the 256 palette entries.
//
// The first sixteen go through the same mapping as the basic colours, so a
// pane that says 38;5;7 and one that says 37 look the same. The greys at the
// top are folded onto the page's own text colour at varying opacity rather
// than fixed greys, for the same reason white and black are.
func xterm(n int) string {
	switch {
	case n < 16:
		return colour(n)
	case n < 232:
		n -= 16
		steps := []int{0, 95, 135, 175, 215, 255}
		return fmt.Sprintf("#%02x%02x%02x", steps[n/36], steps[(n/6)%6], steps[n%6])
	default:
		return ""
	}
}

// parseCSI reads one CSI sequence from the front of s, returning its numeric
// parameters, its final byte, and how many bytes it occupied. n is 0 when s
// does not begin with a CSI.
func parseCSI(s string) (params []int, final byte, n int) {
	if len(s) < 2 || s[0] != 0x1b || s[1] != '[' {
		return nil, 0, 0
	}
	i := 2
	start := i
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == ';' || s[i] == ':' || s[i] == '?') {
		i++
	}
	if i >= len(s) {
		return nil, 0, 0
	}
	for _, f := range strings.FieldsFunc(s[start:i], func(r rune) bool { return r == ';' || r == ':' }) {
		v, err := strconv.Atoi(f)
		if err != nil {
			continue
		}
		params = append(params, v)
	}
	// An empty parameter list means the default, which for SGR is a reset.
	if strings.TrimSpace(s[start:i]) == "" {
		params = nil
	}
	return params, s[i], i + 1
}

// skipEscape steps over an escape sequence this does not understand, so its
// payload does not end up on the page. OSC runs to a BEL or an ESC backslash;
// anything else is assumed to be two bytes.
func skipEscape(s string) int {
	if len(s) < 2 {
		return len(s)
	}
	if s[1] == ']' {
		for i := 2; i < len(s); i++ {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
		return len(s)
	}
	return 2
}
