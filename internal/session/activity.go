package session

import (
	"regexp"
	"strings"

	"github.com/DevOfPie/Mustur/internal/ansi"
)

// An Activity is the line the CLI animates while a turn is in flight.
//
// "✢ Zigzagging… (3m 40s · ↓ 11.5k tokens)" — a glyph that cycles, a verb the
// CLI picks for flavour, how long it has been going, and what it is spending.
// The glyph is the animation: the CLI redraws the line to move it, which is a
// whole-screen repaint arriving as a frame, rendered as a character that jumps
// between shapes.
//
// So it comes off the output and goes into the dock as its own row, where a
// spinner can turn smoothly and the numbers can update without the line being
// redrawn under them (MUS-F-0098).
type Activity struct {
	Verb   string `json:"verb"`             // "Zigzagging"
	For    string `json:"for,omitempty"`    // "3m 40s"
	Detail string `json:"detail,omitempty"` // "↓ 11.5k tokens", "thinking with high effort"
}

// live matches the line only while it is live.
//
// The ellipsis is what makes it live. A finished one reads "✻ Baked for 2m 47s
// · done 12:22 AM" and is transcript: it says what happened and stays on the
// screen as a record of it. Stripping those would take the history out of the
// history.
var live = regexp.MustCompile(`^\s*[^\s\w]\s+(\S[^…]*)…\s*\((.+)\)\s*$`)

// SplitActivity takes the live line off the end of a body and returns both.
//
// The last one only, and only near the end: the CLI draws it under whatever it
// has printed so far, and a line matching this shape further up is something
// the agent wrote rather than something the CLI is doing.
func SplitActivity(body string) (string, *Activity) {
	lines := strings.Split(body, "\n")
	for i := len(lines) - 1; i >= 0 && i > len(lines)-1-activityWithin; i-- {
		plain := strings.TrimRight(ansi.Plain(lines[i]), " ")
		if strings.TrimSpace(plain) == "" {
			continue
		}
		m := live.FindStringSubmatch(plain)
		if m == nil {
			// Something else was printed after it, so it is not the tail any
			// more and nothing here is live.
			return body, nil
		}
		a := &Activity{Verb: strings.TrimSpace(m[1])}
		// "3m 40s · ↓ 11.5k tokens" — the first part is the clock, the rest is
		// whatever the CLI wanted to say. Split once: a detail may carry its
		// own middot and joining it back would be guesswork.
		if bits := strings.SplitN(m[2], "·", 2); len(bits) == 2 {
			a.For = strings.TrimSpace(bits[0])
			a.Detail = strings.TrimSpace(bits[1])
		} else {
			a.For = strings.TrimSpace(m[2])
		}
		return strings.Join(append(lines[:i], lines[i+1:]...), "\n"), a
	}
	return body, nil
}

// How far from the end the live line may sit. It is drawn last, under a blank
// or two.
const activityWithin = 4
