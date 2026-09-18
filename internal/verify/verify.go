// Package verify checks the exported tree.
//
// The export is generated and committed, and nothing about a markdown file
// stops a person editing it. Two things are checkable without trusting the
// binary that wrote it: that every identifier the tree cites is an identifier
// the tree defines, and — when a store is at hand — that rendering the store
// again produces the tree that is there.
package verify

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DevOfPie/Mustur/internal/export"
	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
)

// Tree checks the exported tree against itself and returns one line per
// problem, plus how many identifiers it defined.
//
// Identifiers a record declares retired (MUS-D-0197) are known rather than
// dangling, under three conditions the review of #107 asked for (finding 4):
// the declaration is a row of the declaring record's own field table, not a
// table somebody wrote into a body; a retired identifier nothing defines is
// cited only inside the declaring record and the records it cites, which are
// where it renders as plain text; and the new side of a Renamed row is
// defined. A retired identifier that is defined is not a problem: Idea
// Warehouse is meant to issue IDW-F-0001 again, and then it resolves.
func Tree(dir string) ([]string, int, error) {
	files, err := markdown(dir)
	if err != nil {
		return nil, 0, err
	}
	defined := map[string]string{}          // identifier -> file that defines it
	cited := map[string][]string{}          // identifier -> files citing it
	citedBy := map[string]map[string]bool{} // identifier -> records whose text cites it ("" for none)
	cites := map[string]map[string]bool{}   // record -> identifiers its text cites
	type declared struct{ carrier, old, new, file string }
	var decls []declared
	var problems []string

	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, 0, err
		}
		rel, _ := filepath.Rel(dir, path)
		lines := strings.Split(string(b), "\n")
		owner, own := sections(lines)
		for i, line := range lines {
			head := strings.TrimSpace(strings.TrimLeft(line, "#"))
			if line != head && strings.HasPrefix(line, "#") && ident.Valid(head) {
				if first, dup := defined[head]; dup {
					problems = append(problems, fmt.Sprintf("%s defines %s, already defined in %s", rel, head, first))
				} else {
					defined[head] = rel
				}
			}
			who := owner[i]
			if row := rowRecord(line); row != "" {
				who = row
			}
			if own[i] {
				old, new, isDecl, problem := retiredRow(line)
				if problem != "" {
					problems = append(problems, rel+": "+problem)
				}
				if isDecl {
					if problem == "" {
						decls = append(decls, declared{carrier: who, old: old, new: new, file: rel})
					}
					continue // A declaration is not a citation of what it retires.
				}
			}
			for _, id := range ident.Cited(line) {
				cited[id] = append(cited[id], rel)
				if citedBy[id] == nil {
					citedBy[id] = map[string]bool{}
				}
				citedBy[id][who] = true
				if cites[who] == nil {
					cites[who] = map[string]bool{}
				}
				cites[who][id] = true
			}
		}
	}

	// Where each retired identifier may stand: its declaring records and what
	// they cite.
	scope := map[string]map[string]bool{} // retired identifier -> records it is plain text in
	for _, d := range decls {
		if scope[d.old] == nil {
			scope[d.old] = map[string]bool{}
		}
		scope[d.old][d.carrier] = true
		for id := range cites[d.carrier] {
			scope[d.old][id] = true
		}
		if d.new != "" {
			if _, ok := defined[d.new]; !ok {
				problems = append(problems, fmt.Sprintf("%s renames %s to %s in %s, and %s is defined nowhere", d.carrier, d.old, d.new, d.file, d.new))
			}
		}
	}

	var ids []string
	for id := range cited {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, ok := defined[id]; ok {
			continue
		}
		if scope[id] != nil {
			var outside []string
			for who := range citedBy[id] {
				if !scope[id][who] {
					if who == "" {
						who = "text belonging to no record"
					}
					outside = append(outside, who)
				}
			}
			if len(outside) > 0 {
				sort.Strings(outside)
				problems = append(problems, fmt.Sprintf("%s is retired and cited by %s, outside the records that declare it and what they cite", id, strings.Join(outside, ", ")))
			}
			continue
		}
		where := cited[id]
		sort.Strings(where)
		problems = append(problems, fmt.Sprintf("%s is cited in %s and defined nowhere", id, strings.Join(dedup(where), ", ")))
	}
	sort.Strings(problems)
	return problems, len(defined), nil
}

// sections works out, for each line of an exported file, which record it
// belongs to — the identifier of the last heading that defined one — and
// which lines are rows of that record's own field table: the last
// "| Field | Value |" table in its section, when nothing but rows, blank lines
// and the record separator follow it. A table written into a body is followed
// by the record's own fields or is not the last thing in the section, so it is
// not read as a declaration.
func sections(lines []string) (owner []string, own []bool) {
	owner = make([]string, len(lines))
	own = make([]bool, len(lines))
	current, start := "", 0
	flush := func(end int) {
		if current == "" {
			return
		}
		for i := end - 2; i >= start; i-- {
			if lines[i] != "| Field | Value |" || lines[i+1] != "| --- | --- |" {
				continue
			}
			tail := true
			for j := i + 2; j < end; j++ {
				t := strings.TrimSpace(lines[j])
				if !strings.HasPrefix(lines[j], "| ") && t != "" && t != "---" {
					tail = false
					break
				}
			}
			if tail {
				for j := i + 2; j < end; j++ {
					own[j] = strings.HasPrefix(lines[j], "| ")
				}
			}
			return
		}
	}
	for i, line := range lines {
		head := strings.TrimSpace(strings.TrimLeft(line, "#"))
		if line != head && strings.HasPrefix(line, "#") && ident.Valid(head) {
			flush(i)
			current, start = head, i
		}
		owner[i] = current
	}
	flush(len(lines))
	return owner, own
}

// rowRecord is the record an index row is about — "| [MUS-Q-0153](…) | … |"
// — whose cells are that record's title, not the file's own text.
func rowRecord(line string) string {
	if !strings.HasPrefix(line, "| [") {
		return ""
	}
	id, _, ok := strings.Cut(strings.TrimPrefix(line, "| ["), "]")
	if !ok || !ident.Valid(id) {
		return ""
	}
	return id
}

// retiredRow reads one field row as a retirement, the way record.RetiredBy
// reads the field: "| Renamed | OLD = NEW |" retires OLD, and
// "| Retired | ID :: why |" retires ID. isDecl is true for a row with either
// key; problem is set when its value does not parse — a declaration that
// silently declared nothing would let the check pass on a typo.
func retiredRow(line string) (old, new string, isDecl bool, problem string) {
	for _, key := range []string{record.RenamedField, record.RetiredField} {
		prefix := "| " + key + " | "
		if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, " |") {
			continue
		}
		value := strings.TrimSuffix(strings.TrimPrefix(line, prefix), " |")
		value = strings.ReplaceAll(value, `\|`, "|")
		value = strings.ReplaceAll(value, `\_`, "_")
		var err error
		if key == record.RenamedField {
			old, new, err = record.ParseRenamed(value)
		} else {
			old, _, err = record.ParseRetired(value)
		}
		if err != nil {
			return "", "", true, err.Error()
		}
		return old, new, true, ""
	}
	return "", "", false, ""
}

// AgainstStore renders the records again and reports every file where the tree
// on disk differs from what the store would produce.
func AgainstStore(dir string, records []record.Record) ([]string, error) {
	want, err := export.Render(records)
	if err != nil {
		return nil, err
	}
	var problems []string
	for name, content := range want {
		path := filepath.Join(dir, filepath.FromSlash(name))
		got, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			problems = append(problems, fmt.Sprintf("%s is in the store's export and missing from the tree", name))
			continue
		}
		if err != nil {
			return nil, err
		}
		if string(got) != string(content) {
			problems = append(problems, fmt.Sprintf("%s differs from what the store renders", name))
		}
	}
	onDisk, err := markdown(dir)
	if err != nil {
		return nil, err
	}
	for _, path := range onDisk {
		rel, _ := filepath.Rel(dir, path)
		if _, ok := want[filepath.ToSlash(rel)]; !ok {
			problems = append(problems, fmt.Sprintf("%s is in the tree and not in the store's export", rel))
		}
	}
	sort.Strings(problems)
	return problems, nil
}

func markdown(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".md") {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

func dedup(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
