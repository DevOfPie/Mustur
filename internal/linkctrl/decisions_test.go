package linkctrl

import (
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

const logFixture = `# Decision log

Preamble about the file.

---

## 2026-07-29 — Phase 1 planning

Planning prose that defines nothing. D5 is mentioned.

---

## 2026-07-31 — Six decisions taken ahead of the run

### Delegability stops being a per-permission conversation (D18)

Why D18.

### D25 — verification tooling is not shipped code

Why D25, with [a link](phase-details/m45.md).

` + "```go\n## not a heading\n```" + `

### D313 implemented

A mention, not a definition.

**D26: an invite is bound to its address.** Why D26.

**D124 stands as the reasoning it was**, a mention.

**D135–D138.** The upload surface.

### D135 — the caps

Why D135 on its own.

The repair D269 scheduled for F286.
Keys are matched exactly. **D271.**

**D283, owner-answered, superseding D270.** The base set goes.

**D211.** The owner's answers of 2026-08-18 — two of them.

**D1.** Prose after the number, which the table's title outranks.

**D241.** [D240](#2026-08-19--m62-two-functions)
settled that the log is neutralized. It left three things open.

## D123 — a panel is a route first

An undated section inside this entry.

---
`

const phase2Fixture = `### Phase 2 decisions

Taken 2026-07-31, before the plan was finalised.

| # | Decision | Outcome |
| --- | --- | --- |
| D1 | Mailer | Ships. |
| D18 | Delegability | A rule. |

### Phase 2 decisions taken after the plan was finalised

| # | Decision | Date | Outcome |
| --- | --- | --- | --- |
| D24 | Header menu mechanism | 2026-08-02 | **The Popover API** |
`

func byID(rs []record.Record) map[string]record.Record {
	m := map[string]record.Record{}
	for _, r := range rs {
		m[r.ID] = r
	}
	return m
}

func TestDecisionsImportEntriesWholeAndNumbersAsExtracts(t *testing.T) {
	got, err := Decisions(logFixture, map[string]string{"phase-2.md": phase2Fixture}, "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	m := byID(got)

	first, second := m["LNK-D-0445"], m["LNK-D-0446"]
	if first.At != "2026-07-29" || first.Title != "Phase 1 planning" {
		t.Fatalf("first entry %q at %s", first.Title, first.At)
	}
	if !strings.Contains(second.Body, "undated section inside this entry") {
		t.Fatal("an undated ## heading split the entry it sits in")
	}
	if strings.Contains(second.Body, "\n## D123") || !strings.Contains(second.Body, "#### D123") {
		t.Fatal("headings inside a body were not demoted")
	}
	if !strings.Contains(second.Body, "## not a heading") || strings.Contains(second.Body, "#### not a heading") {
		t.Fatal("a line inside a code fence was treated as a heading")
	}
	if _, ok := m["LNK-D-0447"]; ok {
		t.Fatal("more entries than the fixture has")
	}

	want := map[string]string{
		"LNK-D-0018": "Delegability stops being a per-permission conversation",
		"LNK-D-0025": "verification tooling is not shipped code",
		"LNK-D-0026": "an invite is bound to its address",
		"LNK-D-0135": "the caps",
		"LNK-D-0123": "a panel is a route first",
		"LNK-D-0001": "Mailer",
		"LNK-D-0024": "Header menu mechanism",
	}
	for id, title := range want {
		r, ok := m[id]
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if r.Title != title {
			t.Fatalf("%s titled %q, want %q", id, r.Title, title)
		}
	}
	for _, id := range []string{"LNK-D-0313", "LNK-D-0124", "LNK-D-0005"} {
		if _, ok := m[id]; ok {
			t.Fatalf("%s was only mentioned and was imported as a definition", id)
		}
	}
	for _, id := range []string{"LNK-D-0136", "LNK-D-0137", "LNK-D-0138"} {
		if r := m[id]; !strings.Contains(r.Body, "upload surface") {
			t.Fatalf("%s did not take its range's body: %q", id, r.Body)
		}
	}
	d25 := m["LNK-D-0025"]
	if !strings.Contains(d25.Body, "Why D25") || strings.Contains(d25.Body, "D313") || strings.Contains(d25.Body, "](") {
		t.Fatalf("D25's extract is wrong: %q", d25.Body)
	}
	if e, _ := d25.Get(""); e != "" {
		t.Fatal("unexpected empty key")
	}
	if len(d25.Refs) == 0 || d25.Refs[0].Value != "LNK-D-0446" {
		t.Fatalf("D25 does not cite its entry: %v", d25.Refs)
	}
	if r := m["LNK-D-0271"]; r.Title != "The repair D269 scheduled for F286" || !strings.Contains(r.Body, "matched exactly") {
		t.Fatalf("a paragraph closing with its own number: %q, %q", r.Title, r.Body)
	}
	if r := m["LNK-D-0241"]; r.Title != "D240 settled that the log is neutralized" {
		t.Fatalf("a lead-in whose claim wraps past its line took %q", r.Title)
	}
	if r := m["LNK-D-0211"]; r.Title != "The owner's answers of 2026-08-18 — two of them" {
		t.Fatalf("a bold lead-in holding only its number took %q as its title", r.Title)
	}
	if r := m["LNK-D-0283"]; r.Title != "owner-answered, superseding D270" {
		t.Fatalf("a lead-in followed by a comma: %q", r.Title)
	}
	if v, _ := m["LNK-D-0135"].Get("Also defined"); v == "" {
		t.Fatal("D135's range lead-in was dropped rather than recorded")
	}
	if v, _ := m["LNK-D-0018"].Get("Outcome"); v != "A rule." {
		t.Fatalf("D18's table row was not kept beside its definition: %q", v)
	}
	if d1 := m["LNK-D-0001"]; d1.At != "2026-07-31" {
		t.Fatalf("D1 at %s", d1.At)
	}
	if v, _ := m["LNK-D-0001"].Get("Outcome"); v != "Ships." {
		t.Fatalf("D1's table outcome %q", v)
	}
	if m["LNK-D-0024"].At != "2026-08-02" {
		t.Fatal("a dated row did not keep its own date")
	}
}

func TestDecisionsRefuseANumberInTheEntryRange(t *testing.T) {
	log := "## 2026-07-29 — x\n\n### D445 — collides\n"
	if _, err := Decisions(log, nil, "2026-09-13"); err == nil {
		t.Fatal("D445 was accepted while serial 445 is the first entry")
	}
}
