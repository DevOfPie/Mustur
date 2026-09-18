// Package status is what a finding's State and Status mean (MUS-D-0196).
//
// A finding carries a fixed State — open, done or dropped, the one thing
// everything filters on — and a Status word from its project's own list. Each
// project declares that list on its routing record as repeatable data:
//
//	Status word   fixed = done :: a defect repaired
//
// a word, "=", the State it maps to, "::", and what the word means. Space
// around either separator is ignored and nothing else is accepted: a value that
// does not read that way declares nothing, and ParseWord says why.
//
// This is the one place a Status word is read. The intake that files a jot, the
// correction that retires one, the gate that checks the store and the records
// page that filters on State all ask here, or they are four definitions of the
// same three words that will disagree.
package status

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
)

// The three States. Nothing else is one.
const (
	Open    = "open"
	Done    = "done"
	Dropped = "dropped"
)

// States is every State, in the order a picker offers them.
var States = []string{Open, Done, Dropped}

// The fields a finding and a project carry.
const (
	StateField  = "State"
	StatusField = "Status"
	WordField   = "Status word"
	// PrefixField is the routing record's identifier prefix, which is how a
	// finding finds its project's list.
	PrefixField = "Prefix"
)

// The words intake and reroute write. Every project's list declares both.
const (
	Unreviewed = "unreviewed"
	Superseded = "superseded"
)

// ValidState reports whether s is one of the three States.
func ValidState(s string) bool {
	return s == Open || s == Done || s == Dropped
}

// A Word is one entry in a project's list.
type Word struct {
	Word       string
	State      string
	Definition string
}

// ParseWord reads a Status word value, "word = state :: definition".
func ParseWord(v string) (Word, error) {
	head, def, ok := strings.Cut(v, "::")
	if !ok {
		return Word{}, fmt.Errorf("%s %q is not WORD = STATE :: definition", WordField, v)
	}
	word, state, ok := strings.Cut(head, "=")
	if !ok {
		return Word{}, fmt.Errorf("%s %q is not WORD = STATE :: definition", WordField, v)
	}
	w := Word{Word: strings.TrimSpace(word), State: strings.TrimSpace(state), Definition: strings.TrimSpace(def)}
	switch {
	case w.Word == "" || strings.ContainsAny(w.Word, " \t"):
		return Word{}, fmt.Errorf("%s %q: the word must be one word", WordField, v)
	case !ValidState(w.State):
		return Word{}, fmt.Errorf("%s %q: %q is not a State (open, done or dropped)", WordField, v, w.State)
	case w.Definition == "":
		return Word{}, fmt.Errorf("%s %q says nothing about what the word means", WordField, v)
	}
	return w, nil
}

// Words is one project's list, in the order it declares them.
type Words []Word

// Of reads a project record's list. Values that do not parse, and a word
// declared twice, are returned as problems rather than guessed at.
func Of(project record.Record) (Words, []error) {
	var ws Words
	var problems []error
	seen := map[string]bool{}
	for _, f := range project.Data {
		if f.Key != WordField {
			continue
		}
		w, err := ParseWord(f.Value)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", project.ID, err))
			continue
		}
		if seen[w.Word] {
			problems = append(problems, fmt.Errorf("%s declares the Status word %q twice", project.ID, w.Word))
			continue
		}
		seen[w.Word] = true
		ws = append(ws, w)
	}
	return ws, problems
}

// State is the State a word maps to, and whether the list declares the word.
func (ws Words) State(word string) (string, bool) {
	for _, w := range ws {
		if w.Word == word {
			return w.State, true
		}
	}
	return "", false
}

// Projects maps an identifier prefix to its project's list: the prefix is the
// project record's Prefix field, and a finding belongs to the project whose
// prefix its identifier carries (MUS-D-0093). The intake box's jots resolve
// through MUS-P-0002 the same way, by whatever prefix that record declares.
type Projects map[string]Words

// Index builds the map from every project record among rs.
func Index(rs []record.Record) (Projects, []error) {
	out := Projects{}
	var problems []error
	for _, r := range rs {
		if r.Kind != "project" {
			continue
		}
		prefix, ok := r.Get(PrefixField)
		if !ok || strings.TrimSpace(prefix) == "" {
			continue
		}
		ws, errs := Of(r)
		problems = append(problems, errs...)
		out[strings.TrimSpace(prefix)] = ws
	}
	return out, problems
}

// For is the list a record's identifier resolves to, and whether any project
// declares its prefix.
func (p Projects) For(id string) (Words, bool) {
	parsed, err := ident.Parse(id)
	if err != nil {
		return nil, false
	}
	ws, ok := p[parsed.Project]
	return ws, ok
}

// StateOf is a record's State field, trimmed; empty when it has none.
func StateOf(r record.Record) string {
	v, _ := r.Get(StateField)
	return strings.TrimSpace(v)
}

// WordOf is a record's Status word, trimmed; empty when it has none.
func WordOf(r record.Record) string {
	v, _ := r.Get(StatusField)
	return strings.TrimSpace(v)
}

// Set replaces a record's State and Status in place, or appends them.
func Set(r *record.Record, state, word string) {
	put(r, StatusField, word)
	put(r, StateField, state)
}

func put(r *record.Record, key, value string) {
	for i := range r.Data {
		if r.Data[i].Key == key {
			r.Data[i].Value = value
			return
		}
	}
	r.Data = append(r.Data, record.Field{Key: key, Value: value})
}

// Check reports every finding among rs whose State or Status is not what its
// project declares: no State, a State outside the three, no Status word, a word
// its project's list does not declare, a word mapping to a State other than
// the one the finding carries, or a prefix no project declares. The lists
// themselves are checked too, since a word that does not parse declares
// nothing. One line a problem, sorted.
func Check(rs []record.Record) []string {
	projects, errs := Index(rs)
	var problems []string
	for _, err := range errs {
		problems = append(problems, err.Error())
	}
	for _, r := range rs {
		if r.Kind != "finding" {
			continue
		}
		state, word := StateOf(r), WordOf(r)
		switch {
		case state == "":
			problems = append(problems, fmt.Sprintf("%s has no State", r.ID))
		case !ValidState(state):
			problems = append(problems, fmt.Sprintf("%s has State %q, which is not open, done or dropped", r.ID, state))
		}
		ws, ok := projects.For(r.ID)
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s: no project record declares its prefix, so its Status word means nothing", r.ID))
		case word == "":
			problems = append(problems, fmt.Sprintf("%s has no Status word", r.ID))
		default:
			mapped, declared := ws.State(word)
			if !declared {
				problems = append(problems, fmt.Sprintf("%s has Status %q, which its project does not declare", r.ID, word))
			} else if ValidState(state) && mapped != state {
				problems = append(problems, fmt.Sprintf("%s has Status %q, which means %s, and State %s", r.ID, word, mapped, state))
			}
		}
	}
	sort.Strings(problems)
	return problems
}
