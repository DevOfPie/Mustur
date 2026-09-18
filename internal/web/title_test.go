package web

import (
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

// MUS-F-0174: a title is inline markdown, never a block, and never raw HTML.
func TestATitleRendersInlineMarkdownOnly(t *testing.T) {
	for _, c := range []struct{ in, html, text string }{
		// LNK-D-0010's shape.
		{"Install with `go install ./cmd/linkctrl`", `Install with <code class="t-code">go install ./cmd/linkctrl</code>`, "Install with go install ./cmd/linkctrl"},
		{"A *light* and a **heavy** one", "A <em>light</em> and a <strong>heavy</strong> one", "A light and a heavy one"},
		{"Plain", "Plain", "Plain"},
		{"", "", ""},
		// Raw HTML is text: escaped in the markup, the characters in the text.
		{"Never <script>alert(1)</script> here", "Never &lt;script&gt;alert(1)&lt;/script&gt; here", "Never <script>alert(1)</script> here"},
		{"`<b>` is a tag", `<code class="t-code">&lt;b&gt;</code> is a tag`, "<b> is a tag"},
		// No block: a heading, a list or a quote is the characters.
		{"# Not a heading", "# Not a heading", "# Not a heading"},
		{"- not a list", "- not a list", "- not a list"},
		{"> not a quote", "&gt; not a quote", "> not a quote"},
		// A link is its text, so a title inside an <a> nests no anchor.
		{"See [the docs](https://example.com) first", "See the docs first", "See the docs first"},
		{"[`code` link](x) and ![alt *em*](y.png)", `<code class="t-code">code</code> link and alt <em>em</em>`, "code link and alt em"},
		{"<https://example.com>", "&lt;https://example.com&gt;", "<https://example.com>"},
		// An identifier's underscores are not emphasis, as in a body.
		{"Moves _IB-F-0001_ aside", "Moves _IB-F-0001_ aside", "Moves _IB-F-0001_ aside"},
		{"Fish & chips", "Fish &amp; chips", "Fish & chips"},
	} {
		got := string(title(c.in))
		if got != c.html {
			t.Errorf("title(%q) = %q, want %q", c.in, got, c.html)
		}
		if strings.Contains(got, "<p>") || strings.Contains(got, "<a") || strings.Contains(got, "<h") ||
			strings.Contains(got, "<li") || strings.Contains(got, "<blockquote") {
			t.Errorf("title(%q) = %q carries a block or a link", c.in, got)
		}
		if tx := titleText(c.in); tx != c.text {
			t.Errorf("titleText(%q) = %q, want %q", c.in, tx, c.text)
		}
	}
}

// Every records surface that shows a title renders its code span: the record's
// own heading, the index row, and a citation expanded on another record.
func TestATitleWithCodeReadsAsCodeOnTheRecordsPages(t *testing.T) {
	const code = `<code class="t-code">mustur get</code>`
	srv := serveRecords(t, "",
		decision("MUS-D-0001", "Read one with `mustur get`", "The body."),
		decision("MUS-D-0002", "The citing one", "This follows MUS-D-0001.",
			record.Field{Key: "corrects", Value: "MUS-D-0001"}),
		decision("MUS-D-0003", "The one citing in prose", "This builds on MUS-D-0001."),
	)
	one, _ := fetch(t, srv, "/records/MUS-D-0001")
	if !strings.Contains(one, "<h3>Read one with "+code+"</h3>") {
		t.Error("the record's heading does not render its title's code span")
	}
	if !strings.Contains(one, "code.t-code") {
		t.Error("the record page does not style a title's code")
	}
	index, _ := fetch(t, srv, "/records")
	if !strings.Contains(index, `<span class="t">Read one with `+code+`</span>`) {
		t.Error("the index row does not render its title's code span")
	}
	// A named ref and a citation in prose are drawn by different templates.
	citing, _ := fetch(t, srv, "/records/MUS-D-0002")
	prose, _ := fetch(t, srv, "/records/MUS-D-0003")
	for where, body := range map[string]string{"ref": citing, "prose": prose} {
		if !strings.Contains(body, "<strong>Read one with "+code+"</strong>") {
			t.Errorf("the %s citation does not render the cited title's code span", where)
		}
	}
	for where, body := range map[string]string{"record": one, "index": index, "ref": citing, "prose": prose} {
		if strings.Contains(body, "`mustur get`") {
			t.Errorf("the %s page still shows the backticks", where)
		}
	}
}

// The decision queue's card heading is a title too.
func TestAQuestionTitleWithCodeReadsAsCode(t *testing.T) {
	srv, _ := serveQuestions(t, openQuestion("MUS-Q-0001", "Is `make check` enough?"))
	body := getFrom(t, srv, "/questions")
	if !strings.Contains(body, `<h2>Is <code class="t-code">make check</code> enough?</h2>`) {
		t.Error("the queue card's heading does not render its title's code span")
	}
	if !strings.Contains(body, "code.t-code") {
		t.Error("the queue does not style a title's code")
	}
}
