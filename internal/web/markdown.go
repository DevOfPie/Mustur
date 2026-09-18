package web

// Record text rendered as markdown rather than as escaped plain text.
//
// record.Record.Body has always been markdown -- the export writes it verbatim
// into a .md file -- and the surfaces showed it as one escaped paragraph, so a
// question's newlines collapsed and its ** and | showed raw. The owner asked for
// it readable (MUS-F-0151, prompted by LNK-Q-0002) and chose goldmark on
// MUS-Q-0119: no dependencies outside its own packages.

import (
	"bytes"
	"html/template"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// md is built once and shared; goldmark.Markdown is safe for concurrent use.
//
// html.WithUnsafe is never set, and must not be. With the default renderer raw
// HTML is dropped and javascript:, vbscript:, file: and non-image data: URLs are
// blanked, entity-encoded forms included. That is the only thing standing
// between a body and the page: these pages carry no Content-Security-Policy,
// and bodies are imported from other projects' files, so a body is not text
// this project wrote.
var md = goldmark.New(
	goldmark.WithExtensions(extension.Table),
	goldmark.WithParserOptions(parser.WithASTTransformers(
		util.Prioritized(recordLinks{}, 100),
	)),
)

// A body's citations are written as links into the exported tree --
// "questions.md#mus-q-0034", "work-units/HRD-W-0001.md" -- which is where they
// resolve. Served from /records or /questions they resolve to nothing, so a
// link that names a record is pointed at that record's own page instead.
// Every other link is left exactly as written.
var (
	anchorID = regexp.MustCompile(`#([a-z]{3}-[a-z]-[0-9]{4})$`)
	fileID   = regexp.MustCompile(`(?:^|/)([A-Z]{3}-[A-Z]-[0-9]{4})\.md$`)
)

type recordLinks struct{}

func (recordLinks) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		l, ok := n.(*ast.Link)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		dest := string(l.Destination)
		// A link off this site stays where its author sent it, even when its
		// fragment happens to spell a record.
		if strings.Contains(dest, "://") || strings.HasPrefix(dest, "//") {
			return ast.WalkContinue, nil
		}
		if m := anchorID.FindStringSubmatch(dest); m != nil {
			l.Destination = []byte("/records/" + strings.ToUpper(m[1]))
		} else if m := fileID.FindStringSubmatch(dest); m != nil {
			l.Destination = []byte("/records/" + m[1])
		}
		return ast.WalkContinue, nil
	})
}

// The decision queue renders the same markdown, and an identifier in a
// question's text is a link to that record (MUS-F-0168). The owner met
// "MUS-F-0164" and "MUS-F-0027" in a question and its options and had no way to
// them but typing the address.
//
// Only here, not on Records: that surface expands an identifier in place, below
// the body, which is MUS-D-0040's answer for it, and a link in the prose beside
// the expansion would be a second way to do the same thing.
//
// Only an identifier the store holds is linked. One it does not hold stays
// text, which is how Records treats a dangling citation: a link to a page that
// says "no such record" reads as if the citation resolved.
var mdQueue = goldmark.New(
	goldmark.WithExtensions(extension.Table),
	goldmark.WithParserOptions(parser.WithASTTransformers(
		util.Prioritized(recordLinks{}, 100),
		util.Prioritized(queueLinks{}, 200),
	)),
)

// queueLinkAttrs is what every link inside a question's text carries -- the
// body, an option's detail, the title, what it blocks, the card's identifier,
// and the banner naming the question just answered. Those, and only those, open
// a new tab; the page's own furniture, the header's Account link and the tab
// bar, opens in the same one. It is here once, for the markdown and for the
// plain text alike, because whether it should be a new tab at all is with the
// owner on MUS-Q-0160, and the answer should be one edit.
//
// The case for it: the answer box keeps no draft, so following a link in the
// same tab loses a half-written note.
var queueLinkAttrs = [][2]string{{"target", "_blank"}, {"rel", "noopener"}}

// knownIDs is the parser context key for the set of identifiers the store
// holds. The set is built from the listing the queue already reads, once per
// render, rather than looked up per identifier.
var knownIDs = parser.NewContextKey()

// queueLinks links bare identifiers the store holds, then gives every link the
// queue's attributes. Both steps are in one transformer so their order does not
// rest on how priorities sort.
type queueLinks struct{}

func (queueLinks) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	known, _ := pc.Get(knownIDs).(map[string]bool)
	src := reader.Source()
	// Collected first and split after, because inserting siblings while
	// walking them is how a walk visits a node twice or skips one.
	var texts []*ast.Text
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		// Code is quoted text, and a link's text is already a link.
		case *ast.Link, *ast.AutoLink, *ast.Image, *ast.CodeSpan, *ast.RawHTML:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			texts = append(texts, n)
		}
		return ast.WalkContinue, nil
	})
	for _, t := range texts {
		linkBare(t, src, known)
	}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if l, ok := n.(*ast.Link); ok && entering {
			for _, a := range queueLinkAttrs {
				l.SetAttributeString(a[0], a[1])
			}
		}
		return ast.WalkContinue, nil
	})
}

// linkBare splits one text node around the known identifiers in it. The node
// itself keeps the tail, so a line break that ends it still ends it.
func linkBare(t *ast.Text, src []byte, known map[string]bool) {
	seg := t.Segment
	if t.IsRaw() || seg.Padding != 0 {
		return
	}
	value := seg.Value(src)
	parent := t.Parent()
	pos := 0
	for _, m := range idInProse.FindAllIndex(value, -1) {
		if !known[string(value[m[0]:m[1]])] {
			continue
		}
		if m[0] > pos {
			parent.InsertBefore(parent, t, ast.NewTextSegment(text.NewSegment(seg.Start+pos, seg.Start+m[0])))
		}
		l := ast.NewLink()
		l.Destination = []byte("/records/" + string(value[m[0]:m[1]]))
		l.AppendChild(l, ast.NewTextSegment(text.NewSegment(seg.Start+m[0], seg.Start+m[1])))
		parent.InsertBefore(parent, t, l)
		pos = m[1]
	}
	if pos > 0 {
		t.Segment = text.NewSegment(seg.Start+pos, seg.Stop)
	}
}

// linkIDs is the same for text that is not markdown -- a title, what a question
// blocks, the card's identifier. Escaped first, and the pattern is capitals,
// digits and hyphens, so nothing escaping writes can be mistaken for one.
//
// Not an option's label or line: those sit inside the option's <label>, where a
// link would take the tap meant to choose the option (docs/ui-surfaces.md,
// surface 4, "the whole row is the control").
func linkIDs(known map[string]bool, s string) template.HTML {
	var attrs strings.Builder
	for _, a := range queueLinkAttrs {
		attrs.WriteString(" " + a[0] + `="` + template.HTMLEscapeString(a[1]) + `"`)
	}
	return template.HTML(idInProse.ReplaceAllStringFunc(template.HTMLEscapeString(s), func(id string) string {
		if !known[id] {
			return id
		}
		return `<a href="/records/` + id + `"` + attrs.String() + `>` + id + `</a>`
	}))
}

// markdown renders src for a page.
//
// A table is wrapped in a container of its own that scrolls sideways, because a
// table has no break opportunity to fall back on: left alone it sets the width
// of the document, and on a phone the fixed tab bar goes off the screen with it
// (MUS-F-0033, MUS-F-0131). The string replace is sound because raw HTML is
// dropped and text is escaped, so a literal <table> in the output can only be
// the renderer's.
func markdown(src string) template.HTML {
	return renderMarkdown(md, src)
}

// queueMarkdown renders src for the decision queue, the identifiers in known
// linked.
func queueMarkdown(src string, known map[string]bool) template.HTML {
	pc := parser.NewContext()
	pc.Set(knownIDs, known)
	return renderMarkdown(mdQueue, src, parser.WithContext(pc))
}

func renderMarkdown(m goldmark.Markdown, src string, opts ...parser.ParseOption) template.HTML {
	var b bytes.Buffer
	if err := m.Convert([]byte(src), &b, opts...); err != nil {
		// Convert fails only on a writer error, and a bytes.Buffer has none.
		// Escaped text is still better than nothing if that ever changes.
		return template.HTML(template.HTMLEscapeString(src))
	}
	out := strings.ReplaceAll(b.String(), "<table>", `<div class="md-table"><table>`)
	out = strings.ReplaceAll(out, "</table>", "</table></div>")
	return template.HTML(out)
}

// markdownCSS is shared by every surface that renders record text, inside an
// element with class md. Paragraph margins are left to the page's own rules.
//
// Headings are sized here rather than left to the browser. Its defaults put an
// h5 at .83em and an h6 at .67em -- smaller than the paragraphs beneath them --
// and a page's own heading rules style that page's chrome, not text somebody
// wrote, so a body's heading must not inherit them: a body's "## " once came out
// as a faded uppercase section label. LinkCtrl's phase records nest to ######
// and read as a wall of text with footnotes where the headings should be
// (MUS-F-0163).
//
// Every level sits between the text it heads and the title of the record it is
// in. The floor is 1em of the .md block, which is the size a paragraph in it
// inherits on the queue and a little above it on the records page (15.81px
// there, beside a 17px block). The ceiling is the record's own title: the
// records page's article h3 at 1.1rem, 17.6px -- raised from .98rem, which was
// smaller than the body text and left no room at all (MUS-D-0204, on
// MUS-Q-0166) -- and the queue card's h2 at 1.15rem. On the records page that
// ceiling is 1.035em, so size can take only a small first step:
//
//	h1  1.03em  weight 700
//	h2  1.015em weight 700
//	h3  1em     weight 700
//	h4  1em     weight 600
//	h5  1em     weight 600, opacity .75 -- the pages' muted text is opacity
//	h6  1em     weight 500, italic
//
// Below h2 the order is carried by weight, then by muting, then by style.
const markdownCSS = `
  .md h1, .md h2, .md h3, .md h4, .md h5, .md h6 {
    font-size: 1em; font-weight: 600; line-height: 1.3; margin: 1rem 0 .3rem;
    text-transform: none; letter-spacing: normal; opacity: 1;
    font-style: normal; overflow-wrap: anywhere; }
  .md h1 { font-size: 1.03em; font-weight: 700; }
  .md h2 { font-size: 1.015em; font-weight: 700; }
  .md h3 { font-weight: 700; }
  .md h5 { opacity: .75; }
  .md h6 { font-weight: 500; font-style: italic; }
  .md > :first-child { margin-top: 0; }
  .md > :last-child { margin-bottom: 0; }
  .md ul, .md ol { margin: .3rem 0; padding-left: 1.3rem; }
  .md code { font-size: .9em; background: var(--edge); border-radius: .2rem;
             padding: 0 .2rem; overflow-wrap: anywhere; }
  .md pre { overflow-x: auto; padding: .5rem .6rem; border-radius: .3rem;
            background: var(--edge); }
  .md pre code { background: none; padding: 0; overflow-wrap: normal; }
  .md blockquote { margin: .3rem 0; padding-left: .7rem;
                   border-left: 3px solid var(--edge); }
  .md-table { overflow-x: auto; margin: .4rem 0; max-width: 100%; }
  .md table { border-collapse: collapse; font-size: .92em; }
  .md th, .md td { border: 1px solid var(--edge); padding: .25rem .5rem;
                   text-align: left; vertical-align: top; }
`
