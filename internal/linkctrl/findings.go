package linkctrl

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
)

var findingRow = regexp.MustCompile(`^\| F([0-9]+) \|`)

// Findings reads deferred-findings.md: one finding per F row, from the Open and
// Closed tables. A row names no date of its own, so the earliest date in it is
// used, and today when it has none — which the record then says.
func Findings(r io.Reader, today string) ([]record.Record, error) {
	var out []record.Record
	seen := map[int]bool{}
	section := ""
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for line := 0; sc.Scan(); {
		line++
		text := sc.Text()
		if strings.HasPrefix(text, "## ") {
			section = strings.TrimSpace(strings.TrimPrefix(text, "## "))
			continue
		}
		m := findingRow.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if seen[n] {
			return nil, fmt.Errorf("deferred-findings.md:%d: F%d appears twice", line, n)
		}
		seen[n] = true
		c := cells(text)
		if len(c) != 7 && len(c) != 8 {
			return nil, fmt.Errorf("deferred-findings.md:%d: F%d has %d cells, want 7 or 8", line, n, len(c))
		}
		rec := record.Record{
			ID:    ident.ID{Project: Prefix, Role: ident.Finding, Serial: n}.String(),
			Kind:  "finding",
			Title: titleOf(c[2]),
			Body:  delink(c[2]),
			At:    firstDate(text),
		}
		if rec.At == "" {
			rec.At = today
			rec.Data = append(rec.Data, record.Field{Key: "Dated", Value: "on import: the row carries no date"})
		}
		keys := []string{"", "Found in", "", "Where", "Evidence", "Severity", "Reviewed", "Closed by"}
		for i, k := range keys {
			if k == "" || i >= len(c) || c[i] == "" || c[i] == "—" {
				continue
			}
			rec.Data = append(rec.Data, record.Field{Key: k, Value: delink(c[i])})
		}
		rec.Data = append(rec.Data, record.Field{Key: "Status", Value: strings.ToLower(section)},
			record.Field{Key: "LinkCtrl", Value: "F" + m[1]})
		if err := rec.Validate(); err != nil {
			return nil, fmt.Errorf("deferred-findings.md:%d: %w", line, err)
		}
		out = append(out, rec)
	}
	return out, sc.Err()
}
