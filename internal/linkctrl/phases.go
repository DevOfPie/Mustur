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

var phaseHead = regexp.MustCompile(`^# Phase ([0-9]+) — (.+)$`)

// Phases reads phase-details/phase-N.md into phase records (MUS-D-0173). A
// phase keeps its own prose and every table Mustur holds nowhere else. The
// tables Mustur already holds are dropped, each leaving a line that says where
// it went: a milestone table is on the milestones the phase cites, and a
// decision table's rows are on the D-number decisions. It cites its milestones
// in the order its status table gives, and a phase whose milestones are all
// done says it is closed.
func Phases(src MilestoneSources, renumber map[string]int, today string) ([]record.Record, error) {
	rows := phaseRows(src)
	byPhase := map[string][]string{}
	for n, p := range rows {
		byPhase[p[0]] = append(byPhase[p[0]], n)
	}
	status, err := columnByMilestone(src.Phases, "Status")
	if err != nil {
		return nil, err
	}

	var names []string
	for name := range src.Phases {
		if phaseFile.MatchString(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var out []record.Record
	for _, name := range names {
		text := src.Phases[name]
		lines := strings.Split(text, "\n")
		m := phaseHead.FindStringSubmatch(strings.TrimSpace(lines[0]))
		if m == nil {
			return nil, fmt.Errorf("%s: first line is not '# Phase <n> — <title>'", name)
		}
		n, _ := strconv.Atoi(m[1])
		rec := record.Record{
			ID:    ident.ID{Project: Prefix, Role: ident.Phase, Serial: n}.String(),
			Kind:  "phase",
			Title: "Phase " + m[1] + " — " + delink(strings.TrimSpace(m[2])),
			Body:  prose(heldElsewhere(lines[1:])),
			At:    firstDate(text),
		}
		if rec.At == "" {
			rec.At = today
			rec.Data = append(rec.Data, record.Field{Key: "Dated", Value: "on import: nothing dates it"})
		}

		held := byPhase[m[1]]
		sort.Slice(held, func(i, j int) bool {
			a, _ := strconv.Atoi(rows[held[i]][1])
			b, _ := strconv.Atoi(rows[held[j]][1])
			return a < b
		})
		closed := len(held) > 0
		for _, old := range held {
			serial, ok := renumber[old]
			if !ok {
				return nil, fmt.Errorf("%s: M%s is in its status table and has no milestone record", name, old)
			}
			rec.Refs = append(rec.Refs, record.Field{Key: "milestone", Value: ident.ID{Project: Prefix, Role: ident.Milestone, Serial: serial}.String()})
			if s := status[old]; m[1] != "1" && !strings.HasPrefix(s, "done") {
				closed = false
			}
		}
		state := "live"
		if closed {
			state = "closed: every milestone it holds is done"
		}
		rec.Data = append(rec.Data,
			record.Field{Key: "Status", Value: state},
			record.Field{Key: "Milestones", Value: strconv.Itoa(len(held))},
			record.Field{Key: "LinkCtrl", Value: "phase-details/" + name})
		if err := rec.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out = append(out, rec)
	}
	return out, nil
}

// heldElsewhere drops the tables a phase file shares with records Mustur holds
// under another kind, leaving one line in each one's place.
func heldElsewhere(lines []string) []string {
	var out []string
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if !strings.HasPrefix(l, "|") {
			out = append(out, l)
			continue
		}
		header := cells(l)
		end := i
		for end+1 < len(lines) && strings.HasPrefix(lines[end+1], "|") {
			end++
		}
		if note := whereHeld(header); note != "" {
			out = append(out, note)
			i = end
			continue
		}
		out = append(out, lines[i:end+1]...)
		i = end
	}
	return out
}

func whereHeld(header []string) string {
	if len(header) < 2 {
		return ""
	}
	switch {
	case header[0] == "#" && header[1] == "Milestone":
		return "*This table's rows are held on the milestones this phase cites: each milestone's Status, Phase order, Depends on and Discharges.*"
	case header[0] == "Milestone" && header[1] == "State":
		return "*This table's rows are held on the milestones this phase cites, as each one's Status.*"
	case header[0] == "#" && header[1] == "Decision":
		return "*This table's rows are held on the D-number decisions they name, each carrying its row as Decision and Outcome.*"
	}
	return ""
}
