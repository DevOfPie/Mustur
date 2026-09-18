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
	"html"
	"html/template"
	"regexp"
	"strings"

	"github.com/DevOfPie/Mustur/internal/ident"
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
	goldmark.WithParserOptions(
		parser.WithASTTransformers(util.Prioritized(recordLinks{}, 100)),
		// Ahead of emphasis, which goldmark registers at 500.
		parser.WithInlineParsers(util.Prioritized(reservedID{}, 450)),
	),
)

// reservedID reads a reserved identifier as text before emphasis can take its
// underscore as a delimiter: left to goldmark, "a _IB-F-0001_ b" renders as
// "a <em>IB-F-0001</em> b" (review of #107, nit 8). It fires only where
// ident.Spans would read an identifier, so an underscore anywhere else is
// still emphasis, and "__IB-F-0001_" is still _IB-F-0001 in italics.
type reservedID struct{}

func (reservedID) Trigger() []byte { return []byte{'_'} }

func (reservedID) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, seg := block.PeekLine()
	const width = 10
	// A run of underscores before one: goldmark would take the whole run as
	// one delimiter, and "__IB-F-0001_" came out as "_<em>IB-F-0001</em>". The
	// underscores before the identifier's own are given up as text instead, so
	// the identifier is never split; the italics are lost with them.
	run := 0
	for run < len(line) && line[run] == '_' {
		run++
	}
	if run > 1 && len(line) >= run-1+width && ident.Valid(string(line[run-1:run-1+width])) {
		block.Advance(run - 1)
		return ast.NewTextSegment(text.NewSegment(seg.Start, seg.Start+run-1))
	}
	if len(line) < width || !ident.Valid(string(line[:width])) {
		return nil
	}
	if len(line) > width && inIdentifier(line[width]) {
		return nil
	}
	if prev := block.PrecendingCharacter(); prev < 128 && prev != '\n' && inIdentifier(byte(prev)) {
		return nil
	}
	block.Advance(width)
	return ast.NewTextSegment(text.NewSegment(seg.Start, seg.Start+width))
}

func inIdentifier(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-'
}

// A body's citations are written as links into the exported tree --
// "questions.md#mus-q-0034", "work-units/HRD-W-0001.md" -- which is where they
// resolve. Served from /records or /questions they resolve to nothing, so a
// link that names a record is pointed at that record's own page instead.
// Every other link is left exactly as written.
//
// Both are built on ident.ProjectPattern, so a reserved prefix ("_ib-f-0001")
// is pointed at its page like any other. The anchor is the export's, which is
// the identifier lower-cased, so the pattern is lower-cased with it.
var (
	anchorID = regexp.MustCompile(`#(` + strings.ToLower(ident.ProjectPattern) + `-[a-z]-[0-9]{4})$`)
	fileID   = regexp.MustCompile(`(?:^|/)(` + ident.ProjectPattern + `-[A-Z]-[0-9]{4})\.md$`)
)

type recordLinks struct{}

// plainKey carries, into one conversion, the retired identifiers the record
// being rendered shows as plain text (MUS-D-0197).
var plainKey = parser.NewContextKey()

func (recordLinks) Transform(doc *ast.Document, _ text.Reader, pc parser.Context) {
	plain, _ := pc.Get(plainKey).(map[string]bool)
	var unlink []*ast.Link
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
		id := ""
		if m := anchorID.FindStringSubmatch(dest); m != nil {
			id = strings.ToUpper(m[1])
		} else if m := fileID.FindStringSubmatch(dest); m != nil {
			id = m[1]
		}
		switch {
		case id == "":
		case plain[id]:
			// Retired here: its text stays and the link goes, so it can never
			// point at a record issued later under the same spelling.
			unlink = append(unlink, l)
		default:
			l.Destination = []byte("/records/" + id)
		}
		return ast.WalkContinue, nil
	})
	// Replaced after the walk, not during it: moving a node's children while
	// walking them loses the walk's place.
	for _, l := range unlink {
		parent := l.Parent()
		for c := l.FirstChild(); c != nil; {
			next := c.NextSibling()
			parent.InsertBefore(parent, l, c)
			c = next
		}
		parent.RemoveChild(parent, l)
	}
}

// markdown renders src for a page.
//
// A table is wrapped in a container of its own that scrolls sideways, because a
// table has no break opportunity to fall back on: left alone it sets the width
// of the document, and on a phone the fixed tab bar goes off the screen with it
// (MUS-F-0033, MUS-F-0131). The string replace is sound because raw HTML is
// dropped and text is escaped, so a literal <table> in the output can only be
// the renderer's.
func markdown(src string) template.HTML { return markdownPlain(src, nil) }

// markdownPlain renders src with the retired identifiers in plain shown as
// text rather than links.
func markdownPlain(src string, plain map[string]bool) template.HTML {
	var b bytes.Buffer
	pc := parser.NewContext()
	if len(plain) > 0 {
		pc.Set(plainKey, plain)
	}
	if err := md.Convert([]byte(src), &b, parser.WithContext(pc)); err != nil {
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

// Record titles are markdown too: the export writes one into a heading of a .md
// file, and a title written with inline code -- LNK-D-0010's -- showed its
// backticks on every surface that printed it as plain text (MUS-F-0174).
//
// A title is a line, not a document, so it is read with a parser that knows no
// block but the paragraph: "# x" or "- x" in a title is that text, never a
// heading or a list, and the paragraph is rendered without its <p>, so what
// comes back sits inside whatever element the page already put the title in.
// Inline code, emphasis and strong emphasis are read as markdown.
//
// Links are not kept as links. A title sits inside an <a> on the records index
// and inside a button when a jot is moved, and an anchor inside an anchor is
// not HTML; a link in a title is its text on every surface rather than a link
// on some and not others. Raw HTML is not parsed at all, so "<script>" is text
// and is escaped like text -- stricter than a body, where it is dropped: a
// title that says "<b>" means the characters. Autolinks go with it and read as
// written. Link reference definitions are not read either, so a title that
// looks like one is shown rather than swallowed.
//
// The reserved-identifier parser is the body's, for the body's reason: an
// underscore around an identifier is not emphasis.
var titleMD = goldmark.New(
	goldmark.WithParser(parser.NewParser(
		parser.WithBlockParsers(util.Prioritized(parser.NewParagraphParser(), 1000)),
		parser.WithInlineParsers(
			util.Prioritized(parser.NewCodeSpanParser(), 100),
			util.Prioritized(parser.NewLinkParser(), 200),
			util.Prioritized(reservedID{}, 450),
			util.Prioritized(parser.NewEmphasisParser(), 500),
		),
		parser.WithASTTransformers(util.Prioritized(titleInline{}, 100)),
	)),
)

// titleInline makes a parsed title inline: every paragraph becomes a text
// block, which renders its content with no element of its own, and every link
// or image is replaced by its text. A code span is given the class the pages
// style it by, since a title is not inside a .md block.
type titleInline struct{}

func (titleInline) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	var paras, links []ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case ast.KindParagraph:
			paras = append(paras, n)
		case ast.KindLink, ast.KindImage:
			links = append(links, n)
		case ast.KindCodeSpan:
			n.SetAttributeString("class", []byte("t-code"))
		}
		return ast.WalkContinue, nil
	})
	// After the walk, for the reason recordLinks gives. Innermost first -- the
	// reverse of the walk's order -- so a link inside an image's text is moved
	// out before the image itself is.
	for i := len(links) - 1; i >= 0; i-- {
		l := links[i]
		parent := l.Parent()
		for c := l.FirstChild(); c != nil; {
			next := c.NextSibling()
			parent.InsertBefore(parent, l, c)
			c = next
		}
		parent.RemoveChild(parent, l)
	}
	for _, p := range paras {
		tb := ast.NewTextBlock()
		for c := p.FirstChild(); c != nil; {
			next := c.NextSibling()
			tb.AppendChild(tb, c)
			c = next
		}
		p.Parent().ReplaceChild(p.Parent(), p, tb)
	}
}

// title renders a record title's inline markdown for a page. Every surface
// that shows a record title as text a person reads goes through it; where
// markup cannot go, titleText.
func title(src string) template.HTML {
	var b bytes.Buffer
	if err := titleMD.Convert([]byte(src), &b); err != nil {
		// As markdownPlain: a bytes.Buffer has no writer error.
		return template.HTML(template.HTMLEscapeString(src))
	}
	return template.HTML(strings.TrimSpace(b.String()))
}

// titleTag matches an element title wrote. Sound for title's output alone:
// raw HTML is never parsed there, so a < in a title arrives as &lt; and only
// the renderer's own tags are left to match.
var titleTag = regexp.MustCompile(`<[^>]*>`)

// titleText is a record title as plain text with its markdown markers gone:
// for an <option>, an attribute, a <title>, or a label a script prints, where
// markup cannot go. It is title with the tags taken off and the entities
// resolved, so the two cannot disagree about what a title says.
func titleText(src string) string {
	return html.UnescapeString(titleTag.ReplaceAllString(string(title(src)), ""))
}

// titleFuncs gives a template both renderings of a title.
var titleFuncs = template.FuncMap{"title": title, "titleText": titleText}

// titleCSS styles inline code in a title on every page that shows one. It is
// markdownCSS's code, which does not reach a title because a title is not
// inside a .md block.
const titleCSS = `
  code.t-code { font-size: .9em; background: var(--edge); border-radius: .2rem;
                padding: 0 .2rem; overflow-wrap: anywhere; }
`
