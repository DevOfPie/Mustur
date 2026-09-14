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
// phase keeps its file whole, tables included: the review of Mustur PR 70
// found the milestone tables carry notes, and the decision tables dates, that
// no milestone or decision record holds, so dropping a table because its rows
// are "held elsewhere" lost history (MUS-D-0179). The file is frozen, so the
// copy cannot drift.
//
// A phase cites its milestones in the order its status table gives, is dated
// by the earliest of those milestones, and says it is closed when every one
// is done.
func Phases(src MilestoneSources, renumber map[string]int, milestones []record.Record, today string) ([]record.Record, error) {
	rows := phaseRows(src)
	byPhase := map[string][]string{}
	for n, p := range rows {
		byPhase[p[0]] = append(byPhase[p[0]], n)
	}
	status, err := columnByMilestone(src.Phases, "Status")
	if err != nil {
		return nil, err
	}
	for _, l := range strings.Split(src.Phase1, "\n") {
		if m := phase1Row.FindStringSubmatch(l); m != nil {
			status[m[1]] = strings.TrimSpace(strings.Trim(m[3], "* "))
		}
	}
	dated := map[string]string{}
	for _, r := range milestones {
		if _, onImport := r.Get("Dated"); !onImport && r.Kind == "milestone" {
			dated[r.ID] = r.At
		}
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
			Body:  prose(lines[1:]),
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
			id := ident.ID{Project: Prefix, Role: ident.Milestone, Serial: serial}.String()
			rec.Refs = append(rec.Refs, record.Field{Key: "milestone", Value: id})
			if !strings.HasPrefix(strings.ToLower(status[old]), "done") {
				closed = false
			}
			// A date written inside the file can be anything a row mentions — phase
			// 2's earliest is 2018, from a dependency's history — so a phase is dated
			// by the milestones it holds.
			if d := dated[id]; d != "" && (rec.At == "" || d < rec.At) {
				rec.At = d
			}
		}
		if rec.At == "" {
			rec.At = today
			rec.Data = append(rec.Data, record.Field{Key: "Dated", Value: "on import: no milestone it holds is dated"})
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
