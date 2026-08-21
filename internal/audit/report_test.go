package audit

import (
	"strings"
	"testing"
)

func rendered(t *testing.T, render func(*Report, *strings.Builder) error) string {
	t.Helper()
	rep := run(t, build(t, satisfying()))
	var b strings.Builder
	if err := render(rep, &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// A run reporting only ok and finding is hiding something, so every state is
// named with its count even when the count is zero. The absence of a state is
// a claim, and it gets made out loud.
func TestBothFormsStateAllFiveCounts(t *testing.T) {
	text := rendered(t, func(r *Report, b *strings.Builder) error { return r.Text(b) })
	markdown := rendered(t, func(r *Report, b *strings.Builder) error { return r.Markdown(b) })
	for _, s := range States {
		if !strings.Contains(text, string(s)) {
			t.Errorf("the text form never names %q", s)
		}
		if !strings.Contains(markdown, "`"+string(s)+"`") {
			t.Errorf("the markdown form never names %q", s)
		}
	}
	if !strings.Contains(text, "0 waived") {
		t.Errorf("a zero count was omitted rather than stated:\n%s", text)
	}
}

// Details carry the regular expressions a check matched on, so they carry
// pipes. Unescaped, one would open a column the header never declared.
func TestMarkdownEscapesPipesInDetails(t *testing.T) {
	files := satisfying()
	files["records/one.md"] = "# One\n\nSee [the doc](../gone.md).\n"
	rep := run(t, build(t, files))
	var b strings.Builder
	if err := rep.Markdown(&b); err != nil {
		t.Fatal(err)
	}
	// Every row in a table has to carry the delimiters its own header declared.
	// Details hold the regular expressions a check matched on, so they hold
	// pipes, and one unescaped opens a column the header never named.
	want, tables := 0, 0
	for _, line := range strings.Split(b.String(), "\n") {
		if !strings.HasPrefix(line, "| ---") {
			if !strings.HasPrefix(line, "|") {
				want = 0
				continue
			}
			if want == 0 {
				continue
			}
			if got := strings.Count(strings.ReplaceAll(line, `\|`, ""), "|"); got != want {
				t.Errorf("row has %d delimiters against a %d-delimiter header: %s", got, want, line)
			}
			continue
		}
		want = strings.Count(line, "|")
		tables++
	}
	if tables == 0 {
		t.Fatal("no table was rendered at all")
	}
}

func TestJudgmentQuestionsReachBothForms(t *testing.T) {
	text := rendered(t, func(r *Report, b *strings.Builder) error { return r.Text(b) })
	markdown := rendered(t, func(r *Report, b *strings.Builder) error { return r.Markdown(b) })
	const question = "Is any of this any good?"
	if !strings.Contains(text, question) {
		t.Error("the text form drops the judgment question")
	}
	if !strings.Contains(markdown, question) {
		t.Error("the markdown form drops the judgment question")
	}
}

func TestFindingsAreCountedButAreNotTheExitStatus(t *testing.T) {
	files := satisfying()
	files["records/one.md"] = "# One\n\nSee [the doc](../gone.md).\n"
	rep := run(t, build(t, files))
	if rep.Findings() == 0 {
		t.Fatal("a broken link produced no finding")
	}
	// Run returning without error is the point: the audit ran, so findings are
	// output rather than failure. Gating is the caller's to ask for.
	if !strings.Contains(rep.Summary(), "finding") {
		t.Errorf("summary does not name findings: %s", rep.Summary())
	}
}

// Drift is not a finding and not a count, so nothing else in the report would
// carry it. Pull-only propagation makes this line the only thing that will ever
// tell an adopter a module moved.
func TestNoticesReachBothForms(t *testing.T) {
	files := satisfying()
	files["strucgu.yaml"] = adoption(strings.Replace(demoAdopted, `version: "0.1.0"`, `version: "0.0.1"`, 1))
	rep := run(t, build(t, files))
	var text, markdown strings.Builder
	if err := rep.Text(&text); err != nil {
		t.Fatal(err)
	}
	if err := rep.Markdown(&markdown); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "notice:") {
		t.Error("the text form drops version drift")
	}
	if !strings.Contains(markdown.String(), "## Notices") {
		t.Error("the markdown form drops version drift")
	}
}
