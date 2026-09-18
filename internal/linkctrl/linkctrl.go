// Package linkctrl reads LinkCtrl's records out of its tree, for MUS-M-0009.
//
// Unlike the seed, this copies bodies rather than linking to them: the files
// leave LinkCtrl once the import has its verdict (MUS-D-0164), so a link back
// would point at nothing. Findings and D numbers keep the number LinkCtrl gave
// them (MUS-D-0163); the log's entries follow them from 445 (MUS-D-0166) and
// milestones are renumbered (MUS-D-0167). A link whose target is a file in
// LinkCtrl's tree keeps its text and loses its target, because the target is
// one of the files that leave.
package linkctrl

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/status"
	"github.com/DevOfPie/Mustur/internal/store"
)

// Prefix is the project LinkCtrl's records file under (MUS-P-0004).
const Prefix = "LNK"

// Actor is recorded against every imported event, so the log tells the import
// apart from what is written afterwards.
const Actor = "import-linkctrl"

// place gives every finding among rs the Status word and State LNK's own list
// declares for it (MUS-D-0196), then checks each the way add and amend do, so
// the import writes nothing the findings gate would reject. It is a no-op on
// a store where no project declares a list, which is every store the import
// ran against before the lists existed.
//
// The reader sets Status to the section a row sat under. The file's two
// sections are read as the sweep read them (sectionWords): Closed is fixed and
// Open is unreviewed. Any other section that is a word LNK declares becomes
// that word and the State it means; one that is not arrives unreviewed and
// open with the section kept in the Note, the owner's rule for anything on the
// fence (MUS-D-0201).
func place(rs []record.Record, existing []record.Record) error {
	index, _ := status.Index(existing)
	if !index.Declared() {
		return nil
	}
	ws := index[Prefix]
	for i := range rs {
		r := &rs[i]
		if r.Kind != "finding" {
			continue
		}
		section := status.WordOf(*r)
		word := section
		if w, ok := sectionWords[section]; ok {
			word = w
		}
		if mapped, ok := ws.State(word); ok {
			status.Set(r, mapped, word)
		} else if len(ws) > 0 {
			word := status.Unreviewed
			state, ok := ws.State(word)
			if !ok {
				state = status.Open
			}
			status.Set(r, state, word)
			if section != "" {
				r.Data = append(r.Data, record.Field{Key: "Note", Value: "LinkCtrl section: " + section})
			}
		}
		if no := status.Finding(Prefix, *r, index); no != nil {
			return fmt.Errorf("%s would be written with a State or Status LNK does not declare, so nothing was: %s", r.ID, no.Problem)
		}
	}
	return nil
}

// copied returns rs with fields of their own, so placing them leaves the
// caller's records as the reader made them: a repair run with the same sources
// after an import must read the same sections the import read.
func copied(rs []record.Record) []record.Record {
	out := append([]record.Record(nil), rs...)
	for i := range out {
		out[i].Data = append([]record.Field(nil), out[i].Data...)
	}
	return out
}

// sectionWords is deferred-findings.md's two sections in LNK's words, as the
// owner-reviewed sweep read them (MUS-D-0201): a row under Closed was closed by
// work, which LNK calls fixed and maps to done; a row under Open has not been
// triaged into Mustur, which is unreviewed. Only a section outside these two
// keeps its name in the Note.
var sectionWords = map[string]string{
	"closed": "fixed",
	"open":   status.Unreviewed,
}

// Source is what one file or directory yielded.
type Source struct {
	Path    string
	Records []record.Record
}

// Apply writes records into a store that holds nothing under LNK yet, all or
// nothing. An import that runs twice has stopped being an import.
func Apply(ctx context.Context, s *store.Store, sources []Source) (int, error) {
	existing, err := s.List(ctx, "")
	if err != nil {
		return 0, err
	}
	for _, r := range existing {
		if strings.HasPrefix(r.ID, Prefix+"-") {
			return 0, fmt.Errorf("store already holds %s: the import runs once", r.ID)
		}
	}
	var all []record.Record
	for _, src := range sources {
		all = append(all, src.Records...)
	}
	all = copied(all)
	if err := place(all, existing); err != nil {
		return 0, err
	}
	if err := s.AppendAll(ctx, all, Actor); err != nil {
		return 0, err
	}
	return len(all), nil
}

// RepairActor marks an amendment the import made to its own records.
const RepairActor = "import-linkctrl-repair"

// Repair re-states imported records the importer now reads differently, and
// creates the ones it now reads that the store has never held, which is how a
// kind added after the import arrives (MUS-D-0173). A record anybody else has
// written since the import is left alone and named: replacing it would erase
// what they wrote. Record by record, and safe to run again, since a record
// already matching is not touched.
func Repair(ctx context.Context, s *store.Store, sources []Source) (amended, skipped, created []string, err error) {
	// A repair corrects an import. Against a store that never had one it would
	// write the whole import record by record, round the run-once refusal and
	// the one transaction Apply exists to give.
	existing, err := s.List(ctx, "")
	if err != nil {
		return nil, nil, nil, err
	}
	imported := false
	for _, r := range existing {
		if strings.HasPrefix(r.ID, Prefix+"-") {
			imported = true
			break
		}
	}
	if !imported {
		return nil, nil, nil, fmt.Errorf("the store holds no %s record: there is no import to repair, run --apply", Prefix)
	}
	var toCreate []record.Record
	for _, src := range sources {
		// Placed before it is compared, so a repair states a finding the way
		// the import would write it today and never writes one the gate
		// rejects.
		placed := copied(src.Records)
		if err := place(placed, existing); err != nil {
			return amended, skipped, created, err
		}
		for _, r := range placed {
			events, err := s.History(ctx, r.ID)
			if err != nil {
				return amended, skipped, created, err
			}
			if len(events) == 0 {
				toCreate = append(toCreate, r)
				continue
			}
			last := events[len(events)-1]
			if last.Actor != Actor && last.Actor != RepairActor {
				skipped = append(skipped, r.ID)
				continue
			}
			// A row dated nowhere is stamped with the day it is read; a repair
			// run another day keeps the stamp the import gave it.
			// The stored copy need not carry the mark: work units only gained it
			// after the import stamped them.
			if _, onImport := r.Get("Dated"); onImport {
				r.At = last.Record.At
			}
			was, err := last.Record.MarshalPayload()
			if err != nil {
				return amended, skipped, created, err
			}
			now, err := r.MarshalPayload()
			if err != nil {
				return amended, skipped, created, err
			}
			if string(was) == string(now) {
				continue
			}
			if err := s.Append(ctx, r, "amend", RepairActor); err != nil {
				return amended, skipped, created, err
			}
			amended = append(amended, r.ID)
		}
	}
	// Records the import reads and the store never held arrive together or not
	// at all, the way the import itself did.
	if len(toCreate) > 0 {
		if err := s.AppendAll(ctx, toCreate, RepairActor); err != nil {
			return amended, skipped, created, err
		}
		for _, r := range toCreate {
			created = append(created, r.ID)
		}
	}
	return amended, skipped, created, nil
}

var (
	link = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	date = regexp.MustCompile(`\b(20[0-9]{2}-[0-9]{2}-[0-9]{2})\b`)
)

// delink drops the target of every link into LinkCtrl's own tree and keeps
// links that leave it.
func delink(s string) string {
	return link.ReplaceAllStringFunc(s, func(m string) string {
		parts := link.FindStringSubmatch(m)
		if strings.HasPrefix(parts[2], "http://") || strings.HasPrefix(parts[2], "https://") {
			return m
		}
		return parts[1]
	})
}

// firstDate is the earliest date written anywhere in s, or "".
func firstDate(s string) string {
	best := ""
	for _, d := range date.FindAllString(s, -1) {
		if best == "" || d < best {
			best = d
		}
	}
	return best
}

// cells splits one markdown table row on its unescaped pipes. An escaped \|
// lives inside a code span and comes back unescaped: the export escapes every
// pipe it renders into a table, and one escaped twice ends the cell.
func cells(row string) []string {
	row = strings.TrimSpace(row)
	row = strings.TrimPrefix(row, "|")
	row = strings.TrimSuffix(row, "|")
	var out []string
	var cur strings.Builder
	cut := func() {
		out = append(out, strings.ReplaceAll(strings.TrimSpace(cur.String()), `\|`, "|"))
		cur.Reset()
	}
	for i := 0; i < len(row); i++ {
		if row[i] == '|' && (i == 0 || row[i-1] != '\\') {
			cut()
			continue
		}
		cur.WriteByte(row[i])
	}
	cut()
	return out
}

// titleOf takes a record's one-line claim from prose: the leading bold span if
// there is one, otherwise the first sentence.
func titleOf(s string) string {
	s = strings.TrimSpace(delink(s))
	if strings.HasPrefix(s, "**") {
		if end := strings.Index(s[2:], "**"); end > 0 {
			s = s[2 : 2+end]
		}
	} else if i := strings.Index(s, ". "); i > 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), ".:,;"))
	if r := []rune(s); len(r) > 200 {
		s = string(r[:199]) + "…"
	}
	return s
}

// outsideCode applies fn to the parts of s that are prose: not inside a fenced
// block, not the fence lines themselves, and not inside a code span. A number
// in a shell line or an identifier is not a citation.
func outsideCode(s string, fn func(string) string) string {
	var out []string
	fence := false
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fence = !fence
			out = append(out, line)
			continue
		}
		if fence {
			out = append(out, line)
			continue
		}
		parts := strings.Split(line, "`")
		for i := 0; i < len(parts); i += 2 { // odd parts are inside a code span
			parts[i] = fn(parts[i])
		}
		out = append(out, strings.Join(parts, "`"))
	}
	return strings.Join(out, "\n")
}
