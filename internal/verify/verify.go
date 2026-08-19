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
func Tree(dir string) ([]string, int, error) {
	files, err := markdown(dir)
	if err != nil {
		return nil, 0, err
	}
	defined := map[string]string{} // identifier -> file that defines it
	cited := map[string][]string{} // identifier -> files citing it
	var problems []string

	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, 0, err
		}
		rel, _ := filepath.Rel(dir, path)
		for _, line := range strings.Split(string(b), "\n") {
			head := strings.TrimSpace(strings.TrimLeft(line, "#"))
			if line != head && strings.HasPrefix(line, "#") && ident.Valid(head) {
				if first, dup := defined[head]; dup {
					problems = append(problems, fmt.Sprintf("%s defines %s, already defined in %s", rel, head, first))
					continue
				}
				defined[head] = rel
			}
			for _, id := range ident.Cited(line) {
				cited[id] = append(cited[id], rel)
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
		where := cited[id]
		sort.Strings(where)
		problems = append(problems, fmt.Sprintf("%s is cited in %s and defined nowhere", id, strings.Join(dedup(where), ", ")))
	}
	sort.Strings(problems)
	return problems, len(defined), nil
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
