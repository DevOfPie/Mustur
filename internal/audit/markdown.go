package audit

import (
	"regexp"
	"strings"
)

// Reading markdown for the two things checks ask about: its headings, and its
// links. Both have one rule each that the obvious implementation gets wrong,
// and StrucGu's specification names both because it was bitten by them.

var (
	headingLine = regexp.MustCompile(`(?m)^ {0,3}#{1,6} +(.*?)\s*$`)
	linkTarget  = regexp.MustCompile(`\]\(([^)]*)\)`)
	fenceLine   = regexp.MustCompile("^ {0,3}(```|~~~)")

	// Reference-style links: `[text][label]` pointing at a `[label]: target`
	// definition elsewhere in the file. A checker that reads only the inline
	// form passes a rotted link written this way, which is a boundary StrucGu
	// pins with a fixture of its own.
	referenceUse        = regexp.MustCompile(`\[[^\]]*\]\[([^\]]+)\]`)
	referenceDefinition = regexp.MustCompile(`(?m)^ {0,3}\[([^\]]+)\]:\s*(\S+)`)
)

// stripCode removes fenced blocks and inline spans.
//
// Not an optimisation: a module's own check patterns are written in fences, and
// a pattern of the shape `in` followed by a bracketed alternation contains the
// exact byte sequence a link extractor matches on. A checker without this rule
// reports findings against the regular expressions that told it what to do.
// Lines are kept, blanked rather than deleted, so a reported line number still
// means what it says.
func stripCode(text string) string {
	lines := strings.Split(stripFences(text), "\n")
	for i, line := range lines {
		lines[i] = stripInlineCode(line)
	}
	return strings.Join(lines, "\n")
}

// stripFences blanks fenced blocks and leaves everything else, including the
// contents of inline code spans.
//
// Headings want this rather than stripCode. A `#` line inside a fence is not a
// heading, so fences still have to go — but the words inside backticks are part
// of a heading's anchor. GitHub drops the backtick characters and keeps what
// they wrapped, so `### There is no ` + "`session send`" + ` anchors at
// there-is-no-session-send. Blanking the span first made the audit derive
// there-is-no and report a correct link as pointing at no such heading, which
// is a false finding on this repository's own decisions.md
// (MUS-F-0060).
func stripFences(text string) string {
	lines := strings.Split(text, "\n")
	inFence := false
	for i, line := range lines {
		if fenceLine.MatchString(line) {
			inFence = !inFence
			lines[i] = ""
			continue
		}
		if inFence {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

// stripInlineCode blanks backtick spans, leaving the backticks so that an
// unterminated span cannot swallow the rest of the line.
func stripInlineCode(line string) string {
	var b strings.Builder
	rest := line
	for {
		open := strings.Index(rest, "`")
		if open < 0 {
			b.WriteString(rest)
			return b.String()
		}
		close := strings.Index(rest[open+1:], "`")
		if close < 0 {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(rest[:open+1])
		b.WriteString(strings.Repeat(" ", close))
		b.WriteString("`")
		rest = rest[open+close+2:]
	}
}

// headings returns the text of every ATX heading, hashes and surrounding
// whitespace stripped.
func headings(text string) []string {
	var out []string
	for _, m := range headingLine.FindAllStringSubmatch(stripFences(text), -1) {
		out = append(out, strings.TrimSpace(m[1]))
	}
	return out
}

// slug is GitHub's anchor algorithm: lowercase, drop every character that is
// not [a-z0-9 _-], then replace each space with one hyphen.
//
// Each space individually. Runs are NOT collapsed — collapsing them is the
// obvious implementation and it rejects correct anchors: a heading with an
// em dash between two spaces loses the dash to the strip and keeps both
// spaces, so its real anchor carries two hyphens.
func slug(heading string) string {
	lower := strings.ToLower(heading)
	var kept strings.Builder
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == ' ', r == '_', r == '-':
			kept.WriteRune(r)
		}
	}
	return strings.ReplaceAll(kept.String(), " ", "-")
}

// Anchors returns every anchor a markdown document offers, in GitHub's scheme.
//
// Exported because it is the one implementation of this rule. `check-links.sh`
// derived anchors itself and the two disagreed: this one refused a correct
// anchor by stripping a heading's backticks (MUS-F-0060), and the script
// accepted one that does not exist by reading a `#` inside a fence as a heading
// (MUS-F-0062). The owner chose one implementation on MUS-Q-0064, and this is
// it; the script asks for these rather than working them out again.
//
// Order is the document's, and a repeated heading appears as many times as it
// is written. Neither matters to a membership test, and both matter to anybody
// reading the output.
func Anchors(text string) []string {
	var out []string
	for _, h := range headings(text) {
		out = append(out, slug(h))
	}
	return out
}

// slugs returns every anchor a file offers.
func slugs(text string) map[string]bool {
	out := map[string]bool{}
	for _, h := range headings(text) {
		out[slug(h)] = true
	}
	return out
}

// link is one relative reference found in a file.
type link struct {
	raw    string
	path   string // Empty for a same-file anchor.
	anchor string
}

// links returns every relative link and image target, external schemes and
// code excluded. Both markdown forms count: the inline one, and a reference
// label resolved against its definition.
func links(text string) []link {
	body := stripCode(text)
	var raws []string
	for _, m := range linkTarget.FindAllStringSubmatch(body, -1) {
		raws = append(raws, m[1])
	}
	definitions := map[string]string{}
	for _, m := range referenceDefinition.FindAllStringSubmatch(body, -1) {
		definitions[strings.ToLower(strings.TrimSpace(m[1]))] = m[2]
	}
	for _, m := range referenceUse.FindAllStringSubmatch(body, -1) {
		if target, ok := definitions[strings.ToLower(strings.TrimSpace(m[1]))]; ok {
			raws = append(raws, target)
		}
	}

	var out []link
	for _, candidate := range raws {
		raw := strings.TrimSpace(candidate)
		if raw == "" {
			continue
		}
		// A link title — `](path "title")` — is not part of the target.
		if i := strings.IndexAny(raw, " \t"); i >= 0 {
			raw = raw[:i]
		}
		switch {
		case strings.HasPrefix(raw, "http://"),
			strings.HasPrefix(raw, "https://"),
			strings.HasPrefix(raw, "mailto:"):
			continue
		}
		l := link{raw: raw, path: raw}
		if strings.HasPrefix(raw, "#") {
			l.path, l.anchor = "", strings.TrimPrefix(raw, "#")
		} else if i := strings.Index(raw, "#"); i >= 0 {
			l.path, l.anchor = raw[:i], raw[i+1:]
		}
		out = append(out, l)
	}
	return out
}
