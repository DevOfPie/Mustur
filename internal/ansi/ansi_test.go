package ansi

import (
	"strings"
	"testing"
)

func TestHTMLRendersWhatItUnderstands(t *testing.T) {
	for _, c := range []struct {
		name string
		in   string
		want string
	}{
		{"plain text is untouched", "make check\nok\n", "make check\nok\n"},
		{
			// The whole reason this package exists: a code left on the page as
			// literal text is the defect being fixed.
			"a colour becomes a span",
			"\x1b[31mfailed\x1b[39m",
			`<span style="color:#c0392b;">failed</span>`,
		},
		{
			"bold and underline together",
			"\x1b[1;4mheading\x1b[0m",
			`<span style="font-weight:600;text-decoration:underline;">heading</span>`,
		},
		{
			// White and black follow the page, not the terminal, because a page
			// that is light for one reader and dark for the next cannot use
			// either extreme.
			"white and black fall through to the page's own colour",
			"\x1b[37mwhite\x1b[30mblack\x1b[0m",
			"whiteblack",
		},
		{
			"256-colour indexes resolve",
			"\x1b[38;5;196mred\x1b[0m",
			`<span style="color:#ff0000;">red</span>`,
		},
		{
			"truecolour resolves",
			"\x1b[38;2;18;52;86mexact\x1b[0m",
			`<span style="color:#123456;">exact</span>`,
		},
		{
			// A style that runs past a newline paints its background across the
			// full width of every line below it.
			"a run ends at the newline",
			"\x1b[41mred\nstill\x1b[0m",
			"<span style=\"background:#c0392b;\">red</span>\n<span style=\"background:#c0392b;\">still</span>",
		},
		{
			// Cursor moves are what made the old stream unreadable. They are
			// dropped, not printed.
			"a cursor move is dropped rather than shown",
			"before\x1b[21;3Hafter\x1b[Kend",
			"beforeafterend",
		},
		{
			"a hyperlink's payload does not reach the page",
			"see \x1b]8;id=abc;https://example.com/x\x1b\\here\x1b]8;;\x1b\\",
			"see here",
		},
		{
			"a bell-terminated OSC is skipped too",
			"a\x1b]0;some title\x07b",
			"ab",
		},
		{
			// The pane's contents are somebody else's output.
			"markup in the pane is escaped",
			`<script>alert(1)</script> & "quoted"`,
			`&lt;script&gt;alert(1)&lt;/script&gt; &amp; "quoted"`,
		},
		{
			"a bare reset closes the run",
			"\x1b[32mok\x1b[mplain",
			`<span style="color:#1e8449;">ok</span>plain`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := HTML(c.in); got != c.want {
				t.Errorf("\n got %q\nwant %q", got, c.want)
			}
		})
	}
}

// Nothing that arrives escapes as markup, however malformed.
func TestHTMLNeverEmitsUnescapedInput(t *testing.T) {
	for _, in := range []string{
		"\x1b[",              // a truncated CSI
		"\x1b",               // a lone escape
		"\x1b]8;;",           // an unterminated OSC
		"\x1b[999999999999m", // a parameter that will not fit
		"\x1b[38;5;",         // a truncated extended colour
		"<img src=x onerror=alert(1)>",
		"\x1b[31m<b>",
	} {
		got := HTML(in)
		// Every < in the output must be one this package wrote, which is only
		// ever <span or </span.
		for i := 0; i < len(got); i++ {
			if got[i] != '<' {
				continue
			}
			rest := got[i:]
			if !strings.HasPrefix(rest, "<span style=\"") && !strings.HasPrefix(rest, "</span>") {
				t.Errorf("input %q produced markup this package did not write: %q", in, got)
				break
			}
		}
	}
}

// A real capture, with the sequences a live agent pane actually emits.
func TestHTMLOnARealisticCapture(t *testing.T) {
	in := "\x1b[38;5;246m⏵⏵ auto mode on\x1b[39m · \x1b[1mesc to interrupt\x1b[22m\n" +
		"\x1b[38;5;231m❯ \x1b[39mcount to three\n"
	got := HTML(in)
	for _, want := range []string{"auto mode on", "esc to interrupt", "❯ ", "count to three"} {
		if !strings.Contains(got, want) {
			t.Errorf("the capture lost %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\x1b") || strings.Contains(got, "[38;5;") {
		t.Errorf("an escape survived into the page:\n%s", got)
	}
	// Multi-byte characters must not be split by the byte-at-a-time walk.
	if !strings.Contains(got, "⏵⏵") {
		t.Errorf("a multi-byte character was mangled:\n%s", got)
	}
}
