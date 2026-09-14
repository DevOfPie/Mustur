package linkctrl

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
)

// FirstEntrySerial is where the log's dated entries start numbering: one past
// the highest D number LinkCtrl issued, so an entry never takes a number a
// citation in the prose already means (MUS-D-0166).
const FirstEntrySerial = 445

var (
	entryHead = regexp.MustCompile(`^## (20[0-9]{2}-[0-9]{2}-[0-9]{2}) — (.+)$`)
	defHead   = regexp.MustCompile(`^#{2,3} D([0-9]+) — (.+)$`)
	defParen  = regexp.MustCompile(`^### (.+) \(D([0-9]+)\)$`)
	defBold   = regexp.MustCompile(`^\*\*D([0-9]+)(?:–D([0-9]+))?(?::| —|\.|,)(.*)$`)
	defTrail  = regexp.MustCompile(`\*\*D([0-9]+)\.\*\*\s*$`) // a paragraph that closes by naming itself
	anyHead   = regexp.MustCompile(`^#{2,3} `)
	tableRow  = regexp.MustCompile(`^\| D([0-9]+)\b`) // D11's cell also says what reversed it
)

// A definition's form decides which wins when a number is defined more than
// once: a heading of its own is the author naming the decision, a range
// lead-in is the author naming four at once.
const (
	formHeading = iota
	formParen
	formBold
	formRange
)

type definition struct {
	number, form, line int
	title, body        string
	afterBold          bool   // the title is prose after a bold holding only the number
	entry              string // the entry record it came from
	at                 string
}

// Decisions reads decisions.md and the phase decision tables: every dated
// entry whole, and every D number once, holding its definition and citing
// its entry, or its table row when the log never defined it.
func Decisions(log string, tables map[string]string, today string) ([]record.Record, error) {
	lines := strings.Split(log, "\n")
	var out []record.Record
	defs := map[int][]definition{}

	type span struct{ start, end int }
	var entries []span
	fence := false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
		}
		if !fence && entryHead.MatchString(l) {
			if n := len(entries); n > 0 {
				entries[n-1].end = i
			}
			entries = append(entries, span{start: i})
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("decisions.md: no dated entry")
	}
	entries[len(entries)-1].end = len(lines)

	for k, e := range entries {
		m := entryHead.FindStringSubmatch(lines[e.start])
		id := ident.ID{Project: Prefix, Role: ident.Decision, Serial: FirstEntrySerial + k}.String()
		body := lines[e.start+1 : e.end]
		rec := record.Record{
			ID:    id,
			Kind:  "decision",
			Title: titleOf(m[2]),
			At:    m[1],
			Body:  prose(body),
			Data:  []record.Field{{Key: "LinkCtrl", Value: fmt.Sprintf("decisions.md:%d", e.start+1)}},
		}
		if err := rec.Validate(); err != nil {
			return nil, fmt.Errorf("decisions.md:%d: %w", e.start+1, err)
		}
		out = append(out, rec)
		for _, d := range definitions(body, e.start+2) {
			d.entry, d.at = id, m[1]
			defs[d.number] = append(defs[d.number], d)
		}
	}

	rows, err := tableRows(tables, today)
	if err != nil {
		return nil, err
	}

	numbers := map[int]bool{}
	for n := range defs {
		numbers[n] = true
	}
	for n := range rows {
		numbers[n] = true
	}
	var sorted []int
	for n := range numbers {
		if n >= FirstEntrySerial {
			return nil, fmt.Errorf("D%d is defined, and serials from %d are the log's entries", n, FirstEntrySerial)
		}
		sorted = append(sorted, n)
	}
	sort.Ints(sorted)

	for _, n := range sorted {
		rec := record.Record{
			ID:   ident.ID{Project: Prefix, Role: ident.Decision, Serial: n}.String(),
			Kind: "decision",
		}
		row, hasRow := rows[n]
		if ds := defs[n]; len(ds) > 0 {
			sort.SliceStable(ds, func(i, j int) bool { return ds[i].form < ds[j].form })
			d := ds[0]
			rec.Title, rec.Body, rec.At = d.title, d.body, d.at
			rec.Refs = append(rec.Refs, record.Field{Key: "entry", Value: d.entry})
			rec.Data = append(rec.Data, record.Field{Key: "LinkCtrl", Value: fmt.Sprintf("D%d, decisions.md:%d", n, d.line)})
			for _, other := range ds[1:] {
				rec.Data = append(rec.Data, record.Field{Key: "Also defined", Value: fmt.Sprintf("decisions.md:%d", other.line)})
				if other.entry != d.entry {
					rec.Refs = append(rec.Refs, record.Field{Key: "entry", Value: other.entry})
				}
			}
			if hasRow && (rec.Title == "" || d.afterBold) {
				rec.Title = row.title
			}
			if rec.Title == "" {
				rec.Title = titleOf(rec.Body)
			}
		} else {
			rec.Title, rec.Body, rec.At = row.title, row.outcome, row.at
			rec.Data = append(rec.Data, record.Field{Key: "LinkCtrl", Value: fmt.Sprintf("D%d, %s", n, row.where)})
		}
		if hasRow {
			rec.Data = append(rec.Data,
				record.Field{Key: "Decision", Value: row.title},
				record.Field{Key: "Outcome", Value: row.outcome},
				record.Field{Key: "Phase table", Value: row.where})
		}
		if rec.At == "" {
			rec.At = today
			rec.Data = append(rec.Data, record.Field{Key: "Dated", Value: "on import: nothing dates it"})
		}
		if err := rec.Validate(); err != nil {
			return nil, fmt.Errorf("D%d: %w", n, err)
		}
		out = append(out, rec)
	}
	record.Sort(out)
	return out, nil
}

// definitions finds the D numbers a stretch of an entry defines, telling a
// definition from a mention by what follows the number: a dash, a colon or a
// full stop defines it; "D313 implemented" or "**D124 stands" mentions it.
func definitions(body []string, firstLine int) []definition {
	type start struct {
		at   int
		defs []definition
		head bool
	}
	var starts []start
	fence := false
	for i, l := range body {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		line := firstLine + i
		switch {
		case defHead.MatchString(l):
			m := defHead.FindStringSubmatch(l)
			n, _ := strconv.Atoi(m[1])
			starts = append(starts, start{i, []definition{{number: n, form: formHeading, line: line, title: titleOf(m[2])}}, true})
		case defParen.MatchString(l):
			m := defParen.FindStringSubmatch(l)
			n, _ := strconv.Atoi(m[2])
			starts = append(starts, start{i, []definition{{number: n, form: formParen, line: line, title: titleOf(m[1])}}, true})
		case defBold.MatchString(l):
			m := defBold.FindStringSubmatch(l)
			lo, _ := strconv.Atoi(m[1])
			hi := lo
			form := formBold
			if m[2] != "" {
				hi, _ = strconv.Atoi(m[2])
				form = formRange
			}
			rest := m[3]
			afterBold := false
			if end := strings.Index(rest, "**"); end >= 0 {
				after := rest[end+2:]
				rest = rest[:end]
				// "**D211.** The owner's answers…": the bold holds only the
				// number, so the claim is what follows it — unless a phase
				// table names the decision, which Decisions prefers.
				if strings.TrimSpace(rest) == "" {
					// The claim runs on past the line: "**D241.** [D240](…)" then
					// "settled that…" below. Read the paragraph, not the line.
					para := []string{after}
					for j := i + 1; j < len(body) && strings.TrimSpace(body[j]) != ""; j++ {
						para = append(para, body[j])
					}
					rest, afterBold = strings.Join(strings.Fields(strings.Join(para, " ")), " "), true
				}
			}
			var ds []definition
			for n := lo; n <= hi; n++ {
				ds = append(ds, definition{number: n, form: form, line: line, title: titleOf(rest), afterBold: afterBold})
			}
			starts = append(starts, start{i, ds, false})
		case defTrail.MatchString(l):
			m := defTrail.FindStringSubmatch(l)
			n, _ := strconv.Atoi(m[1])
			from := i
			for from > 0 && strings.TrimSpace(body[from-1]) != "" {
				from--
			}
			if k := len(starts); k > 0 && from <= starts[k-1].at {
				from = starts[k-1].at + 1
			}
			starts = append(starts, start{from, []definition{{number: n, form: formBold, line: firstLine + from, title: titleOf(body[from])}}, false})
		case anyHead.MatchString(l):
			starts = append(starts, start{at: i, head: true})
		}
	}
	var out []definition
	for k, s := range starts {
		end := len(body)
		if k+1 < len(starts) {
			end = starts[k+1].at
		}
		text := prose(body[s.at:end])
		for _, d := range s.defs {
			d.body = text
			out = append(out, d)
		}
	}
	return out
}

type row struct {
	title, outcome, at, where string
}

// tableRows reads the D rows of the phase decision tables. A table's columns
// come from its header, and a row dated nowhere takes the date its caption
// gives.
func tableRows(tables map[string]string, today string) (map[int]row, error) {
	out := map[int]row{}
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		var header []string
		caption := ""
		for i, l := range strings.Split(tables[name], "\n") {
			switch {
			case strings.HasPrefix(l, "#"):
				caption, header = "", nil
			case strings.HasPrefix(l, "| # |"):
				header = cells(l)
			case tableRow.MatchString(l) && header != nil:
				c := cells(l)
				if len(c) != len(header) {
					return nil, fmt.Errorf("%s:%d: %d cells under a %d-column header", name, i+1, len(c), len(header))
				}
				n, _ := strconv.Atoi(tableRow.FindStringSubmatch(l)[1])
				if _, dup := out[n]; dup {
					return nil, fmt.Errorf("%s:%d: D%d has a second table row", name, i+1, n)
				}
				r := row{where: fmt.Sprintf("%s:%d", name, i+1)}
				if note := strings.TrimSpace(strings.TrimPrefix(c[0], fmt.Sprintf("D%d", n))); note != "" {
					r.where += ", " + delink(note)
				}
				for j, h := range header {
					switch h {
					case "Decision":
						r.title = delink(c[j])
					case "Outcome":
						r.outcome = delink(c[j])
					}
				}
				r.at = firstDate(l)
				if r.at == "" {
					r.at = firstDate(caption)
				}
				out[n] = r
			case !strings.HasPrefix(l, "|") && header == nil:
				caption += l + "\n"
			}
		}
	}
	return out, nil
}

// prose is a stretch of LinkCtrl markdown made fit to sit inside a record:
// its links into the tree lose their targets, and its headings drop two
// levels so none can pass for a record's own.
func prose(lines []string) string {
	out := make([]string, 0, len(lines))
	fence := false
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
		}
		if !fence && strings.HasPrefix(l, "#") {
			l = "##" + l
		}
		out = append(out, l)
	}
	s := strings.TrimSpace(strings.Join(out, "\n"))
	s = strings.TrimSpace(strings.TrimSuffix(s, "---"))
	return delink(s)
}
