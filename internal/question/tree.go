package question

// Reading questions out of the exported tree rather than out of a store.
//
// The gate has to run "offline against the working tree" — workflow.md's rule
// for everything in `make check`. Reading the store broke that in a way that
// was worse than not having the gate: the store is machine-local, so on a
// clone and in CI the check could only skip, while CLAUDE.md told every session
// it could not report work complete around an open question. It also left the
// gate unable to tell "no store" from "no buried question", and the Makefile
// guard meant to cover that probed a path the binary did not read.
//
// Against the tree none of that arises. A missing or empty questions.md is not
// an absence of information — it is the tree saying there are no questions, and
// every clone and CI run reads the same file the reviewer does.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
)

// TreeFile is where the export puts questions, relative to the export root.
const TreeFile = "questions.md"

// FromTree reads the questions out of an exported records directory.
//
// A missing file yields no questions and no error: the export only writes a
// file for a kind the store holds, so its absence means none, which is a fact
// about the tree rather than a failure to read it.
func FromTree(dir string) ([]record.Record, error) {
	path := filepath.Join(dir, TreeFile)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var out []record.Record
	var cur *record.Record
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}

	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimSpace(line)

		// `## MUS-Q-0001` opens a record. Any other heading closes one.
		if strings.HasPrefix(trimmed, "#") {
			head := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			flush()
			if ident.Valid(head) {
				cur = &record.Record{ID: head, Kind: Kind}
			}
			continue
		}
		if cur == nil {
			continue
		}

		// The title is the first bolded line under the heading.
		if cur.Title == "" && strings.HasPrefix(trimmed, "**") && strings.HasSuffix(trimmed, "**") && len(trimmed) > 4 {
			cur.Title = strings.TrimSpace(strings.Trim(trimmed, "*"))
			continue
		}

		// `| Key | Value |` is how the export renders a record's fields.
		if key, value, ok := tableRow(trimmed); ok {
			cur.Data = append(cur.Data, record.Field{Key: key, Value: value})
		}
	}
	flush()

	for i := range out {
		if out[i].Title == "" {
			return nil, fmt.Errorf("%s: %s has no title", path, out[i].ID)
		}
	}
	return out, nil
}

// tableRow splits `| Key | Value |` into its two cells. The header and its
// dashed separator are not rows, and are dropped.
func tableRow(line string) (key, value string, ok bool) {
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return "", "", false
	}
	cells := strings.Split(strings.Trim(line, "|"), "|")
	if len(cells) != 2 {
		return "", "", false
	}
	key = strings.TrimSpace(cells[0])
	value = strings.TrimSpace(cells[1])
	if key == "" || key == "Field" || strings.Trim(key, "- ") == "" {
		return "", "", false
	}
	return key, value, true
}
