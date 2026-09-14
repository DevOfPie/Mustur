package web

import (
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
