package record

// Retired identifiers: the ones MUS-D-0192 renamed away, and the ones named and
// never issued.
//
// MUS-D-0192 renamed six jots in place and kept the records describing the
// rename word for word, so those records still cite identifiers nothing
// defines. The owner chose, on MUS-Q-0159 (MUS-D-0197), to declare them on the
// renaming decision itself rather than rewrite the history: the citation check
// accepts them, and in the records describing the rename they are shown as
// plain text — so they never link to whatever Idea Warehouse later issues under
// the same spelling. Everywhere else an identifier links as usual.
//
// The declaration is two data fields, each repeatable:
//
//	Renamed   IDW-F-0001 = _IB-F-0001
//	Retired   IDW-F-0007 :: named in MUS-Q-0153 and never issued
//
// A Renamed value is two identifiers separated by the first "="; a Retired value
// is an identifier, then "::", then a reason that is not empty. Space around
// either separator is ignored, and nothing else is accepted: a value that does
// not read that way declares nothing, and ParseRenamed or ParseRetired says why.

import (
	"fmt"
	"strings"

	"github.com/DevOfPie/Mustur/internal/ident"
)

// RenamedField and RetiredField are the data fields a record declares
// retirements with.
const (
	RenamedField = "Renamed"
	RetiredField = "Retired"
)

// ParseRenamed reads a Renamed value, "OLD = NEW".
func ParseRenamed(v string) (old, new string, err error) {
	o, n, ok := strings.Cut(v, "=")
	if !ok {
		return "", "", fmt.Errorf("%s %q is not OLD = NEW", RenamedField, v)
	}
	o, n = strings.TrimSpace(o), strings.TrimSpace(n)
	if !ident.Valid(o) || !ident.Valid(n) {
		return "", "", fmt.Errorf("%s %q: both sides must be identifiers", RenamedField, v)
	}
	if o == n {
		return "", "", fmt.Errorf("%s %q renames nothing", RenamedField, v)
	}
	return o, n, nil
}

// ParseRetired reads a Retired value, "ID :: why".
func ParseRetired(v string) (id, why string, err error) {
	i, w, ok := strings.Cut(v, "::")
	if !ok {
		return "", "", fmt.Errorf("%s %q is not ID :: why", RetiredField, v)
	}
	i, w = strings.TrimSpace(i), strings.TrimSpace(w)
	if !ident.Valid(i) {
		return "", "", fmt.Errorf("%s %q: %q is not an identifier", RetiredField, v, i)
	}
	if w == "" {
		return "", "", fmt.Errorf("%s %q says no why", RetiredField, v)
	}
	return i, w, nil
}

// RetiredBy returns the identifiers one record declares retired: the old side
// of each Renamed field and each Retired identifier. Values that do not parse
// are returned as problems rather than guessed at.
func RetiredBy(r Record) (ids []string, problems []error) {
	for _, f := range r.Data {
		switch f.Key {
		case RenamedField:
			old, _, err := ParseRenamed(f.Value)
			if err != nil {
				problems = append(problems, fmt.Errorf("%s: %w", r.ID, err))
				continue
			}
			ids = append(ids, old)
		case RetiredField:
			id, _, err := ParseRetired(f.Value)
			if err != nil {
				problems = append(problems, fmt.Errorf("%s: %w", r.ID, err))
				continue
			}
			ids = append(ids, id)
		}
	}
	return ids, problems
}

// Retirements is every retirement a set of records declares, and where the
// retired identifiers are shown as plain text.
type Retirements struct {
	// IDs holds every retired identifier, from any record.
	IDs map[string]bool
	// plain maps a record to the retired identifiers shown as plain text in
	// it: those its carrier declares, in the carrier and every record the
	// carrier cites.
	plain map[string]map[string]bool
}

// Retire collects the retirements declared across rs.
func Retire(rs []Record) Retirements {
	t := Retirements{IDs: map[string]bool{}, plain: map[string]map[string]bool{}}
	for _, r := range rs {
		ids, _ := RetiredBy(r)
		if len(ids) == 0 {
			continue
		}
		scope := append([]string{r.ID}, citedOutside(r)...)
		for _, id := range ids {
			t.IDs[id] = true
			for _, where := range scope {
				if t.plain[where] == nil {
					t.plain[where] = map[string]bool{}
				}
				t.plain[where][id] = true
			}
		}
	}
	return t
}

// PlainIn returns the retired identifiers shown as plain text in the record
// with this identifier. Nil for almost every record.
func (t Retirements) PlainIn(id string) map[string]bool { return t.plain[id] }

// citedOutside is what a carrier cites apart from its declarations. The
// Renamed fields name the new identifiers too, and those records describe
// nothing about the rename.
func citedOutside(r Record) []string {
	rest := r
	rest.Data = nil
	for _, f := range r.Data {
		if f.Key != RenamedField && f.Key != RetiredField {
			rest.Data = append(rest.Data, f)
		}
	}
	return rest.Cites()
}
