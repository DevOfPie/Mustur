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

// Declared reports whether any project among rs declares a Status word. A
// store where none does predates MUS-D-0196 — a fresh `make seed` is one — and
// Check over it would report every finding it holds.
func Declared(rs []record.Record) bool {
	for _, r := range rs {
		if r.Kind != "project" {
			continue
		}
		if _, ok := r.Get(WordField); ok {
			return true
		}
	}
	return false
}

// Declared reports whether any project in the index has a list. An index with
// none is a store that predates MUS-D-0196, where nothing is checked.
func (p Projects) Declared() bool {
	for _, ws := range p {
		if len(ws) > 0 {
			return true
		}
	}
	return false
}

// A Refusal is why one finding cannot be written as it stands. Problem says
// what is wrong; Words is the list the finding's project declares, so the
// caller can say what to pass instead in its own terms.
type Refusal struct {
	ID      string // the finding, or its prefix when it has no identifier yet
	Prefix  string
	Problem string
	Words   Words
}

func (r *Refusal) Error() string {
	return fmt.Sprintf("%s: %s. Status is one of: %s", r.ID, r.Problem, r.Words.List())
}

// List is the words as "word (state)", in the order the project declares them.
func (ws Words) List() string {
	if len(ws) == 0 {
		return "(none declared)"
	}
	parts := make([]string, len(ws))
	for i, w := range ws {
		parts[i] = w.Word + " (" + w.State + ")"
	}
	return strings.Join(parts, ", ")
}

// keys reports a record's State and Status fields that are spelled other
// than exactly, or given more than once. Either is a second field the check
// and the filters would read past: `status: garbage` beside `Status:
// unreviewed` passed the gate, and so did two Status fields.
func keys(r record.Record) string {
	seen := map[string]int{}
	for _, f := range r.Data {
		for _, want := range []string{StatusField, StateField} {
			if strings.EqualFold(strings.TrimSpace(f.Key), want) {
				if f.Key != want {
					return fmt.Sprintf("a field is named %q, which is spelled %s", f.Key, want)
				}
				seen[want]++
			}
		}
	}
	for _, want := range []string{StatusField, StateField} {
		if seen[want] > 1 {
			return fmt.Sprintf("%s is given %d times", want, seen[want])
		}
	}
	return ""
}

// Finding checks one finding about to be written under prefix, the way Check
// checks the store. It returns nil for any other kind, and when no project in
// the store declares a list at all — a store that predates MUS-D-0196.
//
// Where the finding's own project declares no list, only what can be known
// without one is checked: the fields are spelled once each, and a State, if
// there is one, is one of the three. A project that has not declared its
// words yet is not locked out of its own findings (review of #109, m5).
// Unlisted says so when a Status is given anyway.
//
// Where it does, the Status is a word from the list, the State is present
// and one of the three, and it is the State the word means.
func Finding(prefix string, r record.Record, p Projects) *Refusal {
	if r.Kind != "finding" || !p.Declared() {
		return nil
	}
	id := r.ID
	if id == "" {
		id = "a new " + prefix + " finding"
	}
	ws := p[prefix]
	refuse := func(problem string) *Refusal {
		return &Refusal{ID: id, Prefix: prefix, Problem: problem, Words: ws}
	}
	if problem := keys(r); problem != "" {
		return refuse(problem)
	}
	word, state := WordOf(r), StateOf(r)
	if len(ws) == 0 {
		if state != "" && !ValidState(state) {
			return refuse(fmt.Sprintf("State %q is not open, done or dropped", clip(state)))
		}
		return nil
	}
	mapped, declared := ws.State(word)
	switch {
	case word == "":
		return refuse("it has no Status word")
	case !declared:
		return refuse(fmt.Sprintf("Status %q is not a word %s declares", clip(word), prefix))
	case state == "":
		return refuse(fmt.Sprintf("it has no State; %s means %s", word, mapped))
	case !ValidState(state):
		return refuse(fmt.Sprintf("State %q is not open, done or dropped; %s means %s", clip(state), word, mapped))
	case state != mapped:
		return refuse(fmt.Sprintf("Status %s means %s, and State says %s", word, mapped, state))
	}
	return nil
}

// Unlisted is what to tell somebody writing a Status word into a finding whose
// project declares no list: the word means nothing yet, and which record to
// give the list to. Empty when there is nothing to say.
func Unlisted(prefix string, r record.Record, rs []record.Record) string {
	if r.Kind != "finding" || WordOf(r) == "" {
		return ""
	}
	p, _ := Index(rs)
	if !p.Declared() || len(p[prefix]) > 0 {
		return ""
	}
	for _, pr := range rs {
		if v, _ := pr.Get(PrefixField); pr.Kind == "project" && strings.TrimSpace(v) == prefix {
			return fmt.Sprintf("%s declares no Status word, so %q is kept as written and means nothing yet; "+
				"add %q fields (WORD = STATE :: what it means) to %s", pr.ID, WordOf(r), WordField, pr.ID)
		}
	}
	return fmt.Sprintf("no project record has the prefix %s, so %q is kept as written and means nothing yet; "+
		"give %s's project record a %s field and %q fields", prefix, WordOf(r), prefix, PrefixField, WordField)
}

// Fill gives a finding the State its Status word means, when the word is one
// its project declares and no State was given with it. A word alone is enough
// to say what State a finding is in; making somebody spell out both is asking
// them to repeat the list back (review of #109, n6).
func Fill(prefix string, r *record.Record, p Projects) {
	if r.Kind != "finding" || StateOf(*r) != "" {
		return
	}
	if mapped, ok := p[prefix].State(WordOf(*r)); ok {
		put(r, StateField, mapped)
	}
}

// Trim takes the space off a finding's State and Status values, so what is
// stored is what is checked.
func Trim(r *record.Record) {
	for i := range r.Data {
		if r.Data[i].Key == StatusField || r.Data[i].Key == StateField {
			r.Data[i].Value = strings.TrimSpace(r.Data[i].Value)
		}
	}
}

// clip keeps a quoted value to one readable line: what gets refused is most
// often a paragraph put where a word goes.
func clip(s string) string {
	if r := []rune(s); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}

// Check reports every finding among rs that Finding would refuse to write,
// one line each, with the lists themselves: a word that does not parse
// declares nothing. Sorted. It is Finding over the store, so what the gate
// passes and what add and amend accept cannot drift apart.
//
// only, when not empty, is the one prefix whose findings and whose list are
// checked. The store is shared between projects, and a project's gate failing
// on another project's finding is failing on data its branch never touched.
func Check(rs []record.Record, only string) []string {
	projects, errs := Index(rs)
	var problems []string
	if only == "" {
		for _, err := range errs {
			problems = append(problems, err.Error())
		}
	} else {
		for _, r := range rs {
			if p, _ := r.Get(PrefixField); r.Kind == "project" && strings.TrimSpace(p) == only {
				_, own := Of(r)
				for _, err := range own {
					problems = append(problems, err.Error())
				}
			}
		}
	}
	for _, r := range rs {
		if r.Kind != "finding" {
			continue
		}
		id, err := ident.Parse(r.ID)
		if err != nil || (only != "" && id.Project != only) {
			continue
		}
		if no := Finding(id.Project, r, projects); no != nil {
			problems = append(problems, no.ID+": "+no.Problem)
		}
	}
	sort.Strings(problems)
	return problems
}
