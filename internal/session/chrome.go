package session

// Telling the CLI's own furniture apart from what the session actually said.
//
// The bottom of an agent pane is not transcript. It is an input box, a divider
// carrying the task's name, a status line, and a right-aligned hint — furniture
// the CLI redraws every frame, which the owner asked to have out of the output
// and its useful parts shown properly instead.
//
// Two things fall out of removing it. The output becomes only what the session
// said. And the blank rows between the transcript and the box — a hundred of
// them on a tall pane, because the box is anchored to the bottom — become
// trailing blanks, which are already trimmed. Without that, making the pane
// tall enough to scroll back through would have put a hundred-line hole in the
// middle of every view.
//
// **This reads one CLI's furniture and says so.** A pane it does not recognise
// is returned whole, with an empty status: showing everything is a much better
// failure than guessing which lines to delete. The anchor is the input caret,
// because that is the one thing every shape seen so far has — a session with a
// modal open loses its status line and its lower divider, and still has that.

import (
	"strings"
)

// Status is what the CLI's furniture says, once it has been read out of it.
type Status struct {
	// Mode is the input mode: "auto mode on", "plan mode on".
	Mode string
	// Items are the middle of the status line: "PR #31", "1 agent".
	Items []string
	// Note is the right-aligned tail of the status line, which is where a
	// failing check announces itself: "/rc failed".
	Note string
	// Hint is the right-aligned line above the input box, when it is a hint:
	// "new task? /clear to save 118.3k tokens".
	Hint string
	// Update is that same line when it is an update notice instead.
	Update string
}

// Empty reports whether anything was read at all.
func (s Status) Empty() bool {
	return s.Mode == "" && len(s.Items) == 0 && s.Note == "" && s.Hint == "" && s.Update == ""
}

// caret is the input prompt, and the anchor everything else is found from.
const caret = "❯"

// SplitChrome separates what the session said from the CLI's own furniture.
//
// The body comes back with the furniture removed and nothing else changed. A
// pane with no recognisable input box comes back whole.
func SplitChrome(screen string) (string, Status) {
	lines := strings.Split(screen, "\n")

	// The last caret, not the first: a transcript can quote one.
	prompt := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(strings.TrimSpace(stripSGR(lines[i])), caret) {
			prompt = i
			break
		}
	}
	if prompt < 0 {
		return screen, Status{}
	}

	start := prompt
	// The divider immediately above the box, and above that a single
	// right-aligned line — a hint or an update notice. Both are optional: a
	// session with a modal open draws neither.
	if start > 0 && isDivider(lines[start-1]) {
		start--
		if start > 0 && isRightAligned(lines[start-1]) {
			start--
		}
	}

	return strings.Join(lines[:start], "\n"), readStatus(lines[start:])
}

// readStatus pulls the useful parts out of the furniture.
func readStatus(chrome []string) Status {
	var st Status
	for _, raw := range chrome {
		line := stripSGR(raw)
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "" || isDivider(raw):
			// A divider carries the task's name, which the surface already
			// shows in its own header.
		case strings.HasPrefix(trimmed, caret):
			// What is half-typed into the box is the owner's, not the
			// session's, and it is already in front of them.
		case isStatusLine(trimmed):
			readStatusLine(trimmed, &st)
		case isRightAligned(raw):
			if strings.Contains(trimmed, "Update") && strings.Contains(trimmed, "estart") {
				st.Update = strings.TrimSpace(strings.TrimPrefix(trimmed, "✔"))
			} else {
				st.Hint = trimmed
			}
		}
	}
	return st
}

// readStatusLine splits "⏵⏵ auto mode on (shift+tab to cycle) · PR #31 · ← 1
// agent         /rc failed" into its parts.
func readStatusLine(line string, st *Status) {
	parts := strings.Split(line, "·")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// A right-aligned tail is glued to the last part by a run of spaces.
		if at := strings.Index(p, "   "); at >= 0 {
			if tail := strings.TrimSpace(p[at:]); tail != "" {
				st.Note = tail
			}
			p = strings.TrimSpace(p[:at])
			if p == "" {
				continue
			}
		}
		if i == 0 {
			// The mode, minus the glyph that marks it and the reminder of how
			// to change it.
			p = strings.TrimSpace(strings.TrimLeft(p, "⏵⏸⏵ "))
			if at := strings.Index(p, "("); at >= 0 {
				p = strings.TrimSpace(p[:at])
			}
			st.Mode = p
			continue
		}
		st.Items = append(st.Items, p)
	}
}

// isDivider reports whether a line is one of the rules the CLI draws. Twenty is
// well past anything a transcript would contain by accident and well under the
// width of the narrowest pane.
func isDivider(raw string) bool {
	return strings.Count(stripSGR(raw), "─") >= 20
}

// isRightAligned reports whether a line's text sits far enough right to be
// furniture rather than something the session said. The CLI pads these out to
// the pane's width; nothing in a transcript starts thirty columns in.
func isRightAligned(raw string) bool {
	line := strings.TrimRight(stripSGR(raw), " \t")
	if strings.TrimSpace(line) == "" {
		return false
	}
	return len(line)-len(strings.TrimLeft(line, " ")) >= 30
}

// isStatusLine reports whether a line is the CLI's own status row.
func isStatusLine(trimmed string) bool {
	return strings.HasPrefix(trimmed, "⏵") ||
		strings.Contains(trimmed, "mode on ") ||
		strings.HasSuffix(trimmed, "mode on")
}
