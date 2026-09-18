package web

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/question"
	"github.com/DevOfPie/Mustur/internal/record"
)

// LNK-Q-0002's shape: emphasis, code, several paragraphs and a pipe table.
const styledBody = "The **first** paragraph, with *emphasis* and `a-flag`.\n\n" +
	"The second paragraph.\n\n" +
	"| Option | Cost | Effect |\n| --- | --- | --- |\n| One | low | none |\n| Two | high | all |\n"

func assertStyled(t *testing.T, where, body string) {
	t.Helper()
	for _, want := range []string{
		"<strong>first</strong>", "<em>emphasis</em>", "<code>a-flag</code>",
		"<p>The second paragraph.</p>",
		`<div class="md-table"><table>`, "<td>high</td>", "</table></div>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("%s does not render %q", where, want)
		}
	}
	if strings.Contains(body, "**first**") || strings.Contains(body, "| --- |") {
		t.Errorf("%s still shows the markdown raw", where)
	}
}

// MUS-F-0151: a question's body reads as the markdown it is written in.
func TestAQuestionBodyIsMarkdown(t *testing.T) {
	q := openQuestion("MUS-Q-0001", "Styled")
	q.Body = styledBody
	srv, _ := serveQuestions(t, q)
	body := getFrom(t, srv, "/questions")
	assertStyled(t, "the queue", body)
	// Rendering is on the server. The queue still works with script blocked.
	if got := scriptsIn(body); len(got) != 1 || got[0] != "/assets/bar.js" {
		t.Errorf("the queue loads %v, want only the bar's script", got)
	}
}

// An option's detail is one table cell in the export, so it can only carry
// inline markdown -- and that much renders.
func TestAnOptionDetailIsMarkdown(t *testing.T) {
	srv, _ := serveQuestions(t, withOptions("MUS-Q-0001", "Styled",
		"Pick it :: cheap :: Runs **once** with `--flag`."))
	body := getFrom(t, srv, "/questions")
	for _, want := range []string{"<strong>once</strong>", "<code>--flag</code>"} {
		if !strings.Contains(body, want) {
			t.Errorf("the option detail does not render %q", want)
		}
	}
}

func TestARecordBodyIsMarkdown(t *testing.T) {
	srv := serveRecords(t, "", decision("MUS-D-0001", "Styled", styledBody))
	body, _ := fetch(t, srv, "/records/MUS-D-0001")
	assertStyled(t, "the records page", body)
}

// A body is imported from files other projects wrote, and these pages carry no
// Content-Security-Policy. Goldmark's safe defaults are the whole defence.
func TestHostileMarkdownRendersInert(t *testing.T) {
	hostile := strings.Join([]string{
		"<script>alert(1)</script>",
		"<img src=x onerror=alert(1)>",
		"[a](javascript:alert(1))",
		"[b](&#106;avascript:alert(1))",
		"![c](javascript:alert(1))",
		"[d](data:text/html,<script>alert(1)</script>)",
		"[e](VBScript:msgbox(1))",
	}, "\n\n")
	// A handler inside a tag. Escaped text saying onerror= is harmless.
	handler := regexp.MustCompile(`(?i)<[a-z][^>]*\son[a-z]+=`)
	bad := regexp.MustCompile(`(?i)(href|src)="\s*(javascript:|&#|data:text|vbscript:)`)
	check := func(where, body string) {
		t.Helper()
		if strings.Contains(body, "<script>") || strings.Contains(body, "<script>alert") {
			t.Errorf("%s carries a script element", where)
		}
		if handler.MatchString(body) {
			t.Errorf("%s carries an event handler", where)
		}
		if m := bad.FindString(body); m != "" {
			t.Errorf("%s carries a live URL: %s", where, m)
		}
		// The page's own script is the only <script that may appear.
		if n := strings.Count(body, "<script"); n != 1 {
			t.Errorf("%s has %d <script occurrences, want the bar's one", where, n)
		}
	}

	check("markdown()", string(markdown(hostile))+`<script src="/assets/bar.js">`)

	q := openQuestion("MUS-Q-0001", "Hostile")
	q.Body = hostile
	q.Data = append(q.Data, record.Field{Key: question.FieldOption,
		Value: "Pick :: line :: [x](javascript:alert(1)) <img src=x onerror=alert(1)>"})
	srv, _ := serveQuestions(t, q)
	check("the queue", getFrom(t, srv, "/questions"))

	rsrv := serveRecords(t, "", decision("MUS-D-0001", "Hostile", hostile))
	page, _ := fetch(t, rsrv, "/records/MUS-D-0001")
	check("the records page", page)
}

// Bodies cite records as links into the exported tree. Those still reach the
// record once rendered, and the badges found in the text are unchanged.
func TestACitationInARenderedBodyStillResolves(t *testing.T) {
	srv := serveRecords(t, "",
		decision("MUS-D-0001", "The cited one", "First."),
		record.Record{ID: "HRD-W-0001", Kind: "work-unit", Title: "A unit", At: "2026-08-20"},
		decision("MUS-D-0009", "Cites",
			"As [MUS-D-0001](decisions.md#mus-d-0001) said, and [the unit](work-units/HRD-W-0001.md), "+
				"and [elsewhere](https://example.com/x.md#mus-d-0001x), [off site](https://example.com/y.md#mus-d-0001), and MUS-D-0001 bare."),
	)
	body, _ := fetch(t, srv, "/records/MUS-D-0009")
	for _, want := range []string{
		`<a href="/records/MUS-D-0001">MUS-D-0001</a>`,
		`<a href="/records/HRD-W-0001">the unit</a>`,
		`<a href="https://example.com/x.md#mus-d-0001x">`,
		// A fragment that spells a record does not pull a link off site.
		`<a href="https://example.com/y.md#mus-d-0001">off site</a>`,
		`<summary class="badge">MUS-D-0001</summary>`,
		"The cited one",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the records page does not carry %q", want)
		}
	}
	res, code := fetch(t, srv, "/records/MUS-D-0001")
	if code != 200 || !strings.Contains(res, "The cited one") {
		t.Errorf("the rewritten citation does not resolve: %d", code)
	}
}

// MUS-F-0163: LinkCtrl's phase records nest headings to ######, and left to the
// browser an h6 is two thirds of the text beneath it while a body's "## " took
// the page's own faded uppercase section style. Every level a body can write is
// sized by the markdown rules, on both surfaces that render one.
func TestABodysHeadingsAreNeverSmallerThanItsText(t *testing.T) {
	src := "## Two\n\ntext\n\n#### Four\n\ntext\n\n###### Six\n\ntext\n"
	rec := decision("MUS-D-0001", "Headed", src)
	srv := serveRecords(t, "", rec)
	page, _ := fetch(t, srv, "/records/MUS-D-0001")
	q := openQuestion("MUS-Q-0001", "Headed")
	q.Body = src
	qsrv, _ := serveQuestions(t, q)
	queue := getFrom(t, qsrv, "/questions")

	for where, body := range map[string]string{"the records page": page, "the queue": queue} {
		for _, want := range []string{"<h2>Two</h2>", "<h4>Four</h4>", "<h6>Six</h6>"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s does not render %q", where, want)
			}
		}
		i := strings.Index(body, ".md h1, .md h2, .md h3, .md h4, .md h5, .md h6 {")
		if i < 0 {
			t.Errorf("%s leaves a body's headings to the browser's sizes", where)
			continue
		}
		rule := body[i : i+strings.Index(body[i:], "}")]
		for _, want := range []string{"font-size: 1em", "text-transform: none", "opacity: 1"} {
			if !strings.Contains(rule, want) {
				t.Errorf("%s: the heading rule lacks %q: %s", where, want, rule)
			}
		}
	}
}

// headingStyle is what a body heading of one level declares: the rule every
// level shares, overridden by that level's own rule.
func headingStyle(t *testing.T, css, level string) map[string]string {
	t.Helper()
	decls := func(sel string) map[string]string {
		i := strings.Index(css, "\n  "+sel+" {")
		if i < 0 {
			return nil
		}
		body := css[i+len(sel)+5:]
		body = body[:strings.Index(body, "}")]
		out := map[string]string{}
		for _, d := range strings.Split(body, ";") {
			if k, v, ok := strings.Cut(d, ":"); ok {
				out[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
		return out
	}
	style := decls(".md h1, .md h2, .md h3, .md h4, .md h5, .md h6")
	if style == nil {
		t.Fatal("no rule is shared by every body heading")
	}
	for k, v := range decls(".md " + level) {
		style[k] = v
	}
	return style
}

// The review of MUS-F-0163's fix found h4, h5 and h6 sharing one rule, so a
// body nested to ###### still read flat, and the test above passed anyway
// because it read only the shared rule. Each level is its own set of
// declarations now, none smaller than the text it heads and none larger than
// the record title over it (MUS-D-0204, on MUS-Q-0166).
func TestABodysHeadingLevelsAreDistinctAndBetweenTextAndTitle(t *testing.T) {
	// 1em is the text. The title is 1.1rem over a 17px block: 1.035em.
	ceiling := 1.1 * 16 / 17
	seen := map[string]string{}
	for _, level := range []string{"h1", "h2", "h3", "h4", "h5", "h6"} {
		s := headingStyle(t, markdownCSS, level)
		var em float64
		if _, err := fmt.Sscanf(strings.TrimSuffix(s["font-size"], "em"), "%g", &em); err != nil {
			t.Fatalf("%s: font-size %q is not in em", level, s["font-size"])
		}
		if em < 1 || em > ceiling {
			t.Errorf("%s: font-size %gem is outside [1em, %.3fem]", level, em, ceiling)
		}
		key := fmt.Sprintf("size %s, weight %s, opacity %s, style %s",
			s["font-size"], s["font-weight"], s["opacity"], s["font-style"])
		if other, dup := seen[key]; dup {
			t.Errorf("%s declares the same as %s: %s", level, other, key)
		}
		seen[key] = level
	}
	for level, want := range map[string][4]string{
		"h4": {"1em", "600", "1", "normal"},
		"h5": {"1em", "600", ".75", "normal"},
		"h6": {"1em", "500", "1", "italic"},
	} {
		s := headingStyle(t, markdownCSS, level)
		if got := [4]string{s["font-size"], s["font-weight"], s["opacity"], s["font-style"]}; got != want {
			t.Errorf("%s declares %v, want %v", level, got, want)
		}
	}

	page, _ := fetch(t, serveRecords(t, "", decision("MUS-D-0001", "Titled", "text")), "/records/MUS-D-0001")
	if !strings.Contains(page, "article h3 { font-size: 1.1rem;") {
		t.Error("the record title is not 1.1rem, so the ceiling above is not the page's")
	}
}

// A reserved identifier's underscore is not an emphasis delimiter; an
// underscore elsewhere still is (review of #107, nit 8). One run into it is
// given up as text rather than split the identifier.
func TestAReservedIdentifierIsNotTakenForEmphasis(t *testing.T) {
	for in, want := range map[string]string{
		"a _IB-F-0001_ b":           "<p>a _IB-F-0001_ b</p>",
		"_IB-F-0001 and _IB-F-0002": "<p>_IB-F-0001 and _IB-F-0002</p>",
		"__IB-F-0001_":              "<p>__IB-F-0001_</p>",
		"`_IB-F-0001`":              "<p><code>_IB-F-0001</code></p>",
		"_plain emphasis_":          "<p><em>plain emphasis</em></p>",
		"X_IB-F-0001_":              "<p>X_IB-F-0001_</p>",
	} {
		if got := strings.TrimSpace(string(markdown(in))); got != want {
			t.Errorf("markdown(%q) = %q, want %q", in, got, want)
		}
	}
}

// A citation in italics is still a citation: the page reads identifiers the
// way the export check does, and a \b pattern read `_MUS-D-0001_` as nothing.
func TestACitationInItalicsIsStillACitation(t *testing.T) {
	srv := serveRecords(t, "",
		decision("MUS-D-0001", "The cited one", "First."),
		decision("MUS-D-0009", "Cites", "As _MUS-D-0001_ said."),
	)
	body, _ := fetch(t, srv, "/records/MUS-D-0009")
	if !strings.Contains(body, `<summary class="badge">MUS-D-0001</summary>`) {
		t.Error("a citation in italics has no badge")
	}
}

// A reserved prefix is an identifier like any other on the page: its export
// anchor and file are pointed at its record, and naming it bare is a citation.
func TestAReservedIdentifierInABodyResolves(t *testing.T) {
	srv := serveRecords(t, "",
		record.Record{ID: "_IB-F-0001", Kind: "finding", Title: "In the box", At: "2026-09-18"},
		decision("MUS-D-0009", "Cites the box",
			"See [it](findings.md#_ib-f-0001), [its file](findings/_IB-F-0001.md), and _IB-F-0001 bare."),
	)
	body, _ := fetch(t, srv, "/records/MUS-D-0009")
	for _, want := range []string{
		`<a href="/records/_IB-F-0001">it</a>`,
		`<a href="/records/_IB-F-0001">its file</a>`,
		`<summary class="badge">_IB-F-0001</summary>`,
		"In the box",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the records page does not carry %q", want)
		}
	}
	res, code := fetch(t, srv, "/records/_ib-f-0001")
	if code != 200 || !strings.Contains(res, "In the box") {
		t.Errorf("a reserved identifier's page does not resolve: %d", code)
	}
}
