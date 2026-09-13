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

// MilestoneSources is where LinkCtrl says what its milestones are.
type MilestoneSources struct {
	Files  map[string]string // phase-details/m<N>.md, by base name
	Phase1 string            // phase-1.md: M18 to M20, which have no file
	Phases map[string]string // phase-N.md, for the status tables
	Plan   string            // Plan.md, for the Phase 4 ordering rows
}

var (
	milestoneFile = regexp.MustCompile(`^m([0-9]+(?:\.[0-9]+)?)\.md$`)
	milestoneHead = regexp.MustCompile(`^# M[0-9.]+ — (.+)$`)
	phase1Row     = regexp.MustCompile(`^\| \*\*M([0-9]+) — (.+?)\*\* \| (.+) \|$`)
	milestoneRow  = regexp.MustCompile(`^\| \[M([0-9]+(?:\.[0-9]+)?)\]\(`)
	dependsOn     = regexp.MustCompile(`\*\*Depends on:\*\*\s*(.*?)(?:\s*\*\*Discharges:\*\*|$)`)
	discharges    = regexp.MustCompile(`\*\*Discharges:\*\*\s*(.*)$`)
	milestoneRef  = regexp.MustCompile(`\bM([0-9]+(?:\.[0-9]+)?)\b`)
)

// order sorts LinkCtrl milestone numbers the way LinkCtrl orders them: M24.5
// after M24 and before M25, and M26.6 after M26.5.
func order(nums []string) {
	parse := func(s string) (int, int) {
		major, minor := s, "-1"
		if i := strings.IndexByte(s, '.'); i >= 0 {
			major, minor = s[:i], s[i+1:]
		}
		a, _ := strconv.Atoi(major)
		b, _ := strconv.Atoi(minor)
		return a, b
	}
	sort.Slice(nums, func(i, j int) bool {
		ai, bi := parse(nums[i])
		aj, bj := parse(nums[j])
		if ai != aj {
			return ai < aj
		}
		return bi < bj
	})
}

// Milestones renumbers every milestone LinkCtrl has a source for from one, in
// LinkCtrl's order, and returns the renumbering so every reference can follow
// it (MUS-D-0167). Each keeps its old number in its LinkCtrl field; each file
// becomes a work unit at its milestone's serial.
func Milestones(src MilestoneSources, today string) ([]record.Record, map[string]int, error) {
	type phase1 struct{ title, state string }
	fromPhase1 := map[string]phase1{}
	for _, l := range strings.Split(src.Phase1, "\n") {
		if m := phase1Row.FindStringSubmatch(l); m != nil {
			fromPhase1[m[1]] = phase1{title: delink(m[2]), state: delink(m[3])}
		}
	}
	fileOf := map[string]string{}
	for name := range src.Files {
		m := milestoneFile.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		fileOf[m[1]] = name
	}

	var nums []string
	for n := range fromPhase1 {
		if _, dup := fileOf[n]; dup {
			return nil, nil, fmt.Errorf("M%s has both a file and a phase-1 row", n)
		}
		nums = append(nums, n)
	}
	for n := range fileOf {
		nums = append(nums, n)
	}
	order(nums)
	renumber := map[string]int{}
	for i, n := range nums {
		renumber[n] = i + 1
	}

	status, err := columnByMilestone(src.Phases, "Status")
	if err != nil {
		return nil, nil, err
	}
	plan, err := columnByMilestone(map[string]string{"Plan.md": src.Plan}, "Discharges")
	if err != nil {
		return nil, nil, err
	}

	var out []record.Record
	for _, n := range nums {
		serial := renumber[n]
		ms := record.Record{
			ID:   ident.ID{Project: Prefix, Role: ident.Milestone, Serial: serial}.String(),
			Kind: "milestone",
		}
		name, hasFile := fileOf[n]
		if hasFile {
			text := src.Files[name]
			lines := strings.Split(text, "\n")
			m := milestoneHead.FindStringSubmatch(strings.TrimSpace(lines[0]))
			if m == nil {
				return nil, nil, fmt.Errorf("%s: first line is not '# M<N> — <title>'", name)
			}
			ms.Title = delink(strings.TrimSpace(m[1]))
			ms.Body = section(lines, "## Done means")
			ms.At = firstDate(text)
			for _, l := range lines {
				if d := dependsOn.FindStringSubmatch(l); d != nil && !has(ms, "Depends on") {
					ms.Data = append(ms.Data, record.Field{Key: "Depends on", Value: delink(strings.TrimSpace(d[1]))})
				}
				if d := discharges.FindStringSubmatch(l); d != nil && !has(ms, "Discharges") {
					ms.Data = append(ms.Data, record.Field{Key: "Discharges", Value: delink(strings.TrimSpace(d[1]))})
				}
			}
		} else {
			p := fromPhase1[n]
			ms.Title = p.title
			ms.Body = section(strings.Split(src.Phase1, "\n"), "## M"+n+" in detail")
			if ms.Body == "" {
				ms.Body = p.state
			}
			ms.At = firstDate(p.state + "\n" + ms.Body)
			ms.Data = append(ms.Data, record.Field{Key: "Status", Value: p.state})
		}
		if s, ok := status[n]; ok {
			ms.Data = append(ms.Data, record.Field{Key: "Status", Value: s})
		}
		if p, ok := plan[n]; ok {
			ms.Data = append(ms.Data, record.Field{Key: "Plan.md row", Value: p})
		}
		if ms.At == "" {
			ms.At = today
			ms.Data = append(ms.Data, record.Field{Key: "Dated", Value: "on import: nothing dates it"})
		}
		ms.Data = append(ms.Data, record.Field{Key: "LinkCtrl", Value: "M" + n})
		if err := ms.Validate(); err != nil {
			return nil, nil, fmt.Errorf("M%s: %w", n, err)
		}
		out = append(out, ms)

		if !hasFile {
			continue
		}
		lines := strings.Split(src.Files[name], "\n")
		wu := record.Record{
			ID:    ident.ID{Project: Prefix, Role: ident.WorkUnit, Serial: serial}.String(),
			Kind:  "work-unit",
			Title: ms.Title,
			At:    ms.At,
			Body:  prose(lines[1:]),
			Refs:  []record.Field{{Key: "milestone", Value: ms.ID}},
			Data:  []record.Field{{Key: "LinkCtrl", Value: "phase-details/" + name}},
		}
		if err := wu.Validate(); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		out = append(out, wu)
	}
	return out, renumber, nil
}

func has(r record.Record, key string) bool {
	_, ok := r.Get(key)
	return ok
}

// section is the prose under the first heading that starts with head, up to
// the next heading of the same level or higher.
func section(lines []string, head string) string {
	level := strings.IndexByte(head, ' ')
	for i, l := range lines {
		if !strings.HasPrefix(l, head) {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if h := strings.IndexByte(lines[j], ' '); strings.HasPrefix(lines[j], "#") && h > 0 && h <= level && strings.Trim(lines[j][:h], "#") == "" {
				end = j
				break
			}
		}
		return prose(lines[i+1 : end])
	}
	return ""
}

// columnByMilestone reads one named column from every table whose rows start
// with a milestone link, keyed by milestone number.
func columnByMilestone(files map[string]string, column string) (map[string]string, error) {
	out := map[string]string{}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		col := -1
		var width int
		for i, l := range strings.Split(files[name], "\n") {
			switch {
			case strings.HasPrefix(l, "| # |"):
				col, width = -1, 0
				h := cells(l)
				for j, c := range h {
					if c == column {
						col, width = j, len(h)
					}
				}
			case col >= 0 && milestoneRow.MatchString(l):
				c := cells(l)
				if len(c) != width {
					return nil, fmt.Errorf("%s:%d: %d cells under a %d-column header", name, i+1, len(c), width)
				}
				out[milestoneRow.FindStringSubmatch(l)[1]] = delink(c[col])
			case !strings.HasPrefix(l, "|"):
				col = -1
			}
		}
	}
	return out, nil
}

// Rewrite points every milestone reference in what is imported at the new
// number (MUS-D-0167). It leaves code alone, since a milestone number in a
// shell line or an identifier is not a citation, and the LinkCtrl field, which
// exists to keep the old number. What it cannot place is counted, not guessed.
func Rewrite(sources []Source, renumber map[string]int) (int, map[string]int) {
	rewritten, unresolved := 0, map[string]int{}
	swap := func(s string) string {
		var out []string
		fence := false
		for _, line := range strings.Split(s, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				fence = !fence
			}
			if fence || strings.HasPrefix(strings.TrimSpace(line), "```") {
				out = append(out, line)
				continue
			}
			parts := strings.Split(line, "`")
			for i := 0; i < len(parts); i += 2 { // odd parts are inside a code span
				parts[i] = milestoneRef.ReplaceAllStringFunc(parts[i], func(tok string) string {
					n := tok[1:]
					serial, ok := renumber[n]
					if !ok {
						unresolved[tok]++
						return tok
					}
					rewritten++
					return ident.ID{Project: Prefix, Role: ident.Milestone, Serial: serial}.String()
				})
			}
			out = append(out, strings.Join(parts, "`"))
		}
		return strings.Join(out, "\n")
	}
	for si := range sources {
		for ri := range sources[si].Records {
			r := &sources[si].Records[ri]
			r.Title = swap(r.Title)
			r.Body = swap(r.Body)
			for fi := range r.Data {
				if r.Data[fi].Key != "LinkCtrl" {
					r.Data[fi].Value = swap(r.Data[fi].Value)
				}
			}
		}
	}
	return rewritten, unresolved
}
