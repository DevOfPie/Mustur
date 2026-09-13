// Package linkctrl reads LinkCtrl's records out of its tree, for MUS-M-0009.
//
// Unlike the seed, this copies bodies rather than linking to them: the files
// leave LinkCtrl once the import has its verdict (MUS-D-0164), so a link back
// would point at nothing. Every record keeps the number LinkCtrl gave it
// (MUS-D-0163). A link whose target is a file in LinkCtrl's tree keeps its text
// and loses its target, because the target is one of the files that leave.
package linkctrl

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

// Prefix is the project LinkCtrl's records file under (MUS-P-0004).
const Prefix = "LNK"

// Actor is recorded against every imported event, so the log tells the import
// apart from what is written afterwards.
const Actor = "import-linkctrl"

// Source is what one file or directory yielded.
type Source struct {
	Path    string
	Records []record.Record
}

// Apply writes records into a store that holds nothing under LNK yet. An
// import that runs twice has stopped being an import.
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
	n := 0
	for _, src := range sources {
		for _, r := range src.Records {
			if err := s.Append(ctx, r, "create", Actor); err != nil {
				return n, fmt.Errorf("%s: %w", src.Path, err)
			}
			n++
		}
	}
	return n, nil
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
