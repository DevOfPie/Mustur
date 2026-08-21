package audit

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Rendering a run. Two forms, one content: a terminal form for someone
// watching, and a markdown form for a record that has to be readable by
// somebody who was not.
//
// Both state all five counts. A run reporting only ok and finding is hiding
// something, so a zero count is printed rather than omitted — the absence of a
// state is a claim, and it should be made out loud.

// Text writes the terminal form.
func (r *Report) Text(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "audit of %s\n", r.Root)
	fmt.Fprintf(&b, "against %s\n\n", r.Catalog)
	for _, n := range r.Notices {
		fmt.Fprintf(&b, "notice: %s\n", n)
	}
	if len(r.Notices) > 0 {
		b.WriteString("\n")
	}
	for _, mod := range r.modules() {
		fmt.Fprintf(&b, "== %s ==\n", mod)
		for _, res := range r.of(mod) {
			fmt.Fprintf(&b, "  %-9s %-34s %s\n", res.State, res.ID, paths(res.Paths))
			if res.Question != "" {
				fmt.Fprintf(&b, "%s\n", indent(res.Question))
			}
			if res.Detail != "" {
				fmt.Fprintf(&b, "%s\n", indent(res.Detail))
			}
		}
		b.WriteString("\n")
	}
	b.WriteString(r.summary() + "\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// Markdown writes the record form.
func (r *Report) Markdown(w io.Writer) error {
	var b strings.Builder
	b.WriteString("# Conformance audit\n\n")
	fmt.Fprintf(&b, "Read-only. Nothing in this run wrote to the tree it audited.\n\n")
	b.WriteString("| | |\n| --- | --- |\n")
	fmt.Fprintf(&b, "| Audited | `%s` |\n", r.Root)
	fmt.Fprintf(&b, "| Against | `%s` |\n", r.Catalog)
	if len(r.Notices) > 0 {
		// Drift is about the catalog, not the tree, so it is not a finding and
		// does not appear in the counts. It still has to be seen: pull-only
		// propagation means this line is the only thing that will ever tell an
		// adopter a module moved.
		b.WriteString("\n## Notices\n\n")
		for _, n := range r.Notices {
			fmt.Fprintf(&b, "- %s\n", n)
		}
	}
	b.WriteString("\n## Summary\n\n| State | Count | Means |\n| --- | --- | --- |\n")
	for _, s := range States {
		fmt.Fprintf(&b, "| `%s` | %d | %s |\n", s, r.Counts[s], meanings[s])
	}

	for _, mod := range r.modules() {
		fmt.Fprintf(&b, "\n## %s\n\n| State | Check | Read | Detail |\n| --- | --- | --- | --- |\n", mod)
		for _, res := range r.of(mod) {
			detail := res.Detail
			if res.Question != "" {
				detail = strings.TrimSpace(res.Question + " " + detail)
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n",
				res.State, cell(res.ID), cell(paths(res.Paths)), cell(detail))
		}
	}
	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())
	return err
}

var meanings = map[State]string{
	OK:            "The check passed.",
	Finding:       "The check failed. Carries a location and evidence.",
	Skip:          "The role is unmapped, unreadable, or its module is not adopted. \"I could not tell\", which is not the same as \"fine\".",
	Waived:        "An accepted deviation. The reason is echoed every run, not silently suppressed.",
	NeedsJudgment: "Needs a person. Emitted whether or not one is present.",
}

// Findings is how many checks failed. Not the exit status: findings are
// output, and a consumer who wants to gate on them has to ask.
func (r *Report) Findings() int { return r.Counts[Finding] }

func (r *Report) summary() string {
	parts := make([]string, 0, len(States))
	for _, s := range States {
		parts = append(parts, fmt.Sprintf("%d %s", r.Counts[s], s))
	}
	return fmt.Sprintf("%d results: %s", r.Total(), strings.Join(parts, ", "))
}

// Summary is the one-line form, for a caller that wants to print it itself.
func (r *Report) Summary() string { return r.summary() }

func (r *Report) modules() []string {
	seen := map[string]bool{}
	var out []string
	for _, res := range r.Results {
		if !seen[res.Module] {
			seen[res.Module] = true
			out = append(out, res.Module)
		}
	}
	sort.Strings(out)
	return out
}

func (r *Report) of(module string) []Result {
	var out []Result
	for _, res := range r.Results {
		if res.Module == module {
			out = append(out, res)
		}
	}
	return out
}

func indent(s string) string {
	var b strings.Builder
	for i, line := range strings.Split(wrap(s, 88), "\n") {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("            " + line)
	}
	return b.String()
}

func wrap(s string, width int) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	line := words[0]
	for _, w := range words[1:] {
		if len(line)+1+len(w) > width {
			b.WriteString(line + "\n")
			line = w
			continue
		}
		line += " " + w
	}
	b.WriteString(line)
	return b.String()
}

func paths(p []string) string {
	if len(p) == 0 {
		return "—"
	}
	if len(p) > 3 {
		return fmt.Sprintf("%s and %d more", strings.Join(p[:3], ", "), len(p)-3)
	}
	return strings.Join(p, ", ")
}

// cell escapes a table cell. A pipe in a detail would otherwise open a column
// the header never declared — and details here carry regular expressions,
// which is where the pipes come from.
func cell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "—"
	}
	return s
}
