package audit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testCatalog(t *testing.T) *Catalog {
	t.Helper()
	cat, err := LoadCatalog("testdata/catalog")
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

// build writes a tree from a path-to-content map and returns its root.
func build(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func run(t *testing.T, root string) *Report {
	t.Helper()
	rep, err := Run(root, testCatalog(t), time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("the audit could not run: %v", err)
	}
	return rep
}

func state(t *testing.T, rep *Report, id string) Result {
	t.Helper()
	for _, r := range rep.Results {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no result for %s", id)
	return Result{}
}

func adoption(modules string) string {
	return "schema: strucgu/adoption@1\nmodules:\n" + modules
}

const demoAdopted = `  demo:
    version: "0.1.0"
    adopted: 2026-08-20
    form: one-file
    effective_from: 2026-08-20
    roles:
      doc: doc.md
      records: records/
      by_form_role: doc.md
`

// A tree that satisfies everything the demo module asks for.
func satisfying() map[string]string {
	return map[string]string{
		"strucgu.yaml": adoption(demoAdopted),
		"doc.md": "# Doc\n\n## Risks\n\nThis file is append-only.\n\n" +
			"A link to [the records](records/one.md) and to [risks](#risks).\n",
		"records/one.md":   "# One\n\nSee [the doc](../doc.md).\n",
		"records/_tmpl.md": "# Template\n\nTODO: fill this in.\n",
	}
}

func TestSatisfyingTreePasses(t *testing.T) {
	rep := run(t, build(t, satisfying()))
	for _, id := range []string{"D-01", "D-02", "D-03", "D-04", "D-05", "D-06"} {
		if got := state(t, rep, id); got.State != OK {
			t.Errorf("%s = %s (%s), want ok", id, got.State, got.Detail)
		}
	}
}

// The excluded template carries TODO, and D-04 forbids it. If the exclusion
// were not applied the satisfying tree above would fail, so this asserts the
// exclusion is doing work rather than being decorative.
func TestRoleExclusionIsApplied(t *testing.T) {
	files := satisfying()
	files["records/two.md"] = "# Two\n\nTODO: this one is not excluded.\n"
	rep := run(t, build(t, files))
	if got := state(t, rep, "D-04"); got.State != Finding {
		t.Fatalf("D-04 = %s, want finding once an unexcluded record carries TODO", got.State)
	}
}

func TestEachCheckDetectsItsOwnViolation(t *testing.T) {
	cases := []struct {
		check  string
		mutate func(map[string]string)
	}{
		{"D-02", func(f map[string]string) { f["doc.md"] = strings.ReplaceAll(f["doc.md"], "## Risks", "## Hazards") }},
		{"D-03", func(f map[string]string) { f["doc.md"] = strings.ReplaceAll(f["doc.md"], "append-only", "editable") }},
		{"D-04", func(f map[string]string) { f["records/one.md"] += "\nTBD\n" }},
		{"D-05", func(f map[string]string) { f["records/one.md"] = "# One\n\nSee [the doc](../gone.md).\n" }},
		{"D-06", func(f map[string]string) { f["records/one.md"] = "# One\n\nNo link at all.\n" }},
	}
	for _, c := range cases {
		t.Run(c.check, func(t *testing.T) {
			files := satisfying()
			c.mutate(files)
			rep := run(t, build(t, files))
			if got := state(t, rep, c.check); got.State != Finding {
				t.Errorf("%s = %s, want finding; detail %q", c.check, got.State, got.Detail)
			}
		})
	}
}

// An anchor whose heading held an em dash between two spaces keeps both spaces
// through the punctuation strip, so its real anchor carries two hyphens.
// Collapsing runs is the obvious implementation and it rejects correct anchors.
func TestAnchorRunsAreNotCollapsed(t *testing.T) {
	files := satisfying()
	files["doc.md"] += "\n## 2026-08-20 — corrections\n\nBack to [them](#2026-08-20--corrections).\n"
	rep := run(t, build(t, files))
	if got := state(t, rep, "D-05"); got.State != OK {
		t.Fatalf("D-05 = %s (%s); a two-hyphen anchor was rejected", got.State, got.Detail)
	}
}

// A module's own patterns are written in fences, and a bracketed alternation
// after `in` contains the byte sequence a link extractor matches on. A checker
// without this rule reports findings against the regular expressions that told
// it what to do.
func TestLinksInsideCodeAreNotChecked(t *testing.T) {
	files := satisfying()
	files["doc.md"] += "\n```yaml\nin: [decision_log, decision_index](not-a-file.md)\n```\n" +
		"\nAnd inline: `](also-not-a-file.md)`.\n"
	rep := run(t, build(t, files))
	if got := state(t, rep, "D-05"); got.State != OK {
		t.Fatalf("D-05 = %s (%s); a link inside code was followed", got.State, got.Detail)
	}
}

// The opposite rule, and the specification is explicit that it is opposite:
// a pattern matching inside a code block is a known false positive of the
// pattern kinds, which makes it a match rather than a bug.
func TestPatternsDoMatchInsideCode(t *testing.T) {
	files := satisfying()
	files["doc.md"] = "# Doc\n\n## Risks\n\n```\nappend-only\n```\n\n[risks](#risks)\n"
	rep := run(t, build(t, files))
	if got := state(t, rep, "D-03"); got.State != OK {
		t.Fatalf("D-03 = %s (%s); a pattern inside a fence was not counted", got.State, got.Detail)
	}
}

func TestUnmappedRoleSkipsEveryCheckBoundOnlyToIt(t *testing.T) {
	files := satisfying()
	files["strucgu.yaml"] = adoption(strings.Replace(demoAdopted, "doc: doc.md", "doc: ~", 1))
	rep := run(t, build(t, files))
	for _, id := range []string{"D-01", "D-02", "D-03"} {
		got := state(t, rep, id)
		if got.State != Skip {
			t.Errorf("%s = %s, want skip when its only role is unmapped", id, got.State)
		}
	}
	// D-05 reads two roles; one still resolved, so it read something and must
	// not claim it could not tell.
	if got := state(t, rep, "D-05"); got.State == Skip {
		t.Error("D-05 skipped though one of its two roles resolved")
	}
}

// The judgment entry names doc, which is unmapped here. The line is still
// emitted — a mechanical run must never look complete — and it says what it
// could not read rather than sending a person to a document that is not there.
func TestJudgmentIsEmittedEvenWithNothingToRead(t *testing.T) {
	files := satisfying()
	files["strucgu.yaml"] = adoption(strings.Replace(demoAdopted, "doc: doc.md", "doc: ~", 1))
	rep := run(t, build(t, files))
	got := state(t, rep, "demo.needs-a-person")
	if got.State != NeedsJudgment {
		t.Fatalf("judgment entry = %s, want judgment", got.State)
	}
	if !strings.Contains(got.Detail, "nothing to read") {
		t.Errorf("the line does not say what it could not read: %q", got.Detail)
	}
}

func TestCrossModuleRoleSkipsWhenItsModuleIsNotAdopted(t *testing.T) {
	rep := run(t, build(t, satisfying()))
	got := state(t, rep, "D-07")
	if got.State != Skip {
		t.Fatalf("D-07 = %s, want skip: extra is not adopted", got.State)
	}
	if !strings.Contains(got.Detail, "does not adopt") {
		t.Errorf("the skip does not say why: %q", got.Detail)
	}
}

func TestCrossModuleRoleResolvesWhenItIs(t *testing.T) {
	files := satisfying()
	files["strucgu.yaml"] = adoption(demoAdopted + "  extra:\n    version: \"0.1.0\"\n    adopted: 2026-08-20\n    roles:\n      extra_doc: extra.md\n")
	files["extra.md"] = "# Extra\n"
	files["doc.md"] += "\nAnd [the extra one](extra.md).\n"
	rep := run(t, build(t, files))
	if got := state(t, rep, "D-07"); got.State != OK {
		t.Fatalf("D-07 = %s (%s), want ok once extra is adopted and linked", got.State, got.Detail)
	}
	if got := state(t, rep, "X-01"); got.State != OK {
		t.Errorf("X-01 = %s, want ok", got.State)
	}
}

func TestHistoryChecksSkipWithoutEffectiveFrom(t *testing.T) {
	files := satisfying()
	files["strucgu.yaml"] = adoption(strings.Replace(demoAdopted, "    effective_from: 2026-08-20\n", "", 1))
	rep := run(t, build(t, files))
	got := state(t, rep, "D-08")
	if got.State != Skip {
		t.Fatalf("D-08 = %s, want skip: history is not read unbounded", got.State)
	}
	if !strings.Contains(got.Detail, "effective_from") {
		t.Errorf("the skip does not name what is missing: %q", got.Detail)
	}
}

func TestByFormCardinalityFollowsTheDeclaredForm(t *testing.T) {
	files := satisfying()
	files["strucgu.yaml"] = adoption(strings.Replace(demoAdopted, "form: one-file", "form: many-files", 1))
	files["strucgu.yaml"] = strings.Replace(files["strucgu.yaml"], "by_form_role: doc.md", "by_form_role: records/", 1)
	rep := run(t, build(t, files))
	if got := state(t, rep, "D-09"); got.State != OK {
		t.Fatalf("D-09 = %s (%s): the role should have resolved as a directory", got.State, got.Detail)
	}
}

func TestDeviationWaivesAndExpires(t *testing.T) {
	files := satisfying()
	files["doc.md"] = strings.ReplaceAll(files["doc.md"], "append-only", "editable")
	deviation := `
deviations:
  - check: D-03
    title: This document is not append-only
    accepted: 2026-08-01
    by: owner@example.org
    reason: >
      It is a working note, and the obligation is about logs.
    scope: doc.md
    review_by: %s
`
	t.Run("within its review date", func(t *testing.T) {
		f := map[string]string{}
		for k, v := range files {
			f[k] = v
		}
		f["strucgu.yaml"] = adoption(demoAdopted) + strings.Replace(deviation, "%s", "2027-01-01", 1)
		rep := run(t, build(t, f))
		got := state(t, rep, "D-03")
		if got.State != Waived {
			t.Fatalf("D-03 = %s, want waived", got.State)
		}
		if !strings.Contains(got.Detail, "working note") {
			t.Errorf("the reason is not echoed: %q", got.Detail)
		}
	})
	t.Run("past it", func(t *testing.T) {
		f := map[string]string{}
		for k, v := range files {
			f[k] = v
		}
		f["strucgu.yaml"] = adoption(demoAdopted) + strings.Replace(deviation, "%s", "2026-08-19", 1)
		rep := run(t, build(t, f))
		if got := state(t, rep, "D-03"); got.State != Finding {
			t.Fatalf("D-03 = %s, want finding: a deviation past its expiry does not apply", got.State)
		}
	})
}

func TestARunThatCannotStartIsAnErrorRatherThanAFinding(t *testing.T) {
	cases := []struct {
		name, adoption, want string
	}{
		{"floating version", adoption("  demo:\n    version: \"0.1\"\n    adopted: 2026-08-20\n    roles:\n      doc: doc.md\n"), "exact"},
		{"unknown module", adoption("  nosuch:\n    version: \"0.1.0\"\n    adopted: 2026-08-20\n    roles: {}\n"), "does not hold"},
		{"wrong schema", "schema: strucgu/adoption@2\nmodules: {}\n", "strucgu/adoption@1"},
		{"version the catalog does not hold", adoption("  demo:\n    version: \"9.9.9\"\n    adopted: 2026-08-20\n    roles:\n      doc: doc.md\n"), "Audit the version you adopted"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := build(t, map[string]string{"strucgu.yaml": c.adoption, "doc.md": "# Doc\n"})
			_, err := Run(root, testCatalog(t), time.Now())
			if err == nil {
				t.Fatal("the run started and should not have")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not say %q", err, c.want)
			}
		})
	}
}

// A role mapped at a path git has never seen exists for its author and for
// nobody who clones it.
func TestUntrackedPathsAreNotReadInARepository(t *testing.T) {
	root := build(t, satisfying())
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "strucgu.yaml", "records"},
		{"-c", "user.email=t@example.org", "-c", "user.name=T", "commit", "-qm", "first"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git unavailable: %v %s", err, out)
		}
	}
	rep := run(t, root)
	got := state(t, rep, "D-01")
	if got.State != Skip {
		t.Fatalf("D-01 = %s, want skip: doc.md exists but is untracked", got.State)
	}
	if !strings.Contains(got.Detail, "not tracked") {
		t.Errorf("the skip does not say why: %q", got.Detail)
	}
}

func TestAllFiveStatesAreCounted(t *testing.T) {
	rep := run(t, build(t, satisfying()))
	for _, s := range States {
		if _, ok := rep.Counts[s]; !ok {
			rep.Counts[s] = 0
		}
	}
	if len(States) != 5 {
		t.Fatalf("%d states, want 5", len(States))
	}
	if rep.Counts[OK] == 0 || rep.Counts[Skip] == 0 || rep.Counts[NeedsJudgment] == 0 {
		t.Errorf("counts look folded: %v", rep.Counts)
	}
}
