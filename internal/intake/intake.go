// Package intake is the capture path: a line of text becomes a finding record
// without anyone having to decide anything first.
//
// The constraint the whole package is shaped by: **filing must never require a
// decision.** Naming a thing requires understanding it, and at capture time you
// do not — so the title is derived rather than asked for, and the destination
// is guessed rather than chosen. Both are recorded as what they are, so a
// reader can tell a guess from a statement.
package intake

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

// Destination is where a jot was routed, and why.
type Destination struct {
	ID   string // A routing record's identifier.
	Name string
	Why  string
	// Prefix is the identifier prefix a jot routed here is filed under, when
	// the routing record names one. Empty means the store's own.
	//
	// It exists because the owner filed a jot that routed correctly to the idea
	// inbox and still read as mis-routed: it was called MUS-F-0025, which is
	// indistinguishable at a glance from a record about Mustur itself. A prefix
	// that says where a record belongs is the difference between a store you
	// can scan and one you have to open (MUS-Q-0030).
	Prefix string
}

// PrefixField names the identifier prefix a routing record's jots are filed
// under. A field on the record rather than a constant here, for the same reason
// the default is: the routing registry is something the store holds and a
// reader can see, not something compiled in.
const PrefixField = "Prefix"

// DefaultField marks the routing record a jot falls back to. It is a field on
// a record rather than a constant here, so the fallback is something the store
// holds and a reader can see, not something compiled in.
const DefaultField = "Intake"

// DefaultValue is the value that field carries on the fallback destination.
const DefaultValue = "default"

// routingKinds are the record kinds a jot can be routed to.
var routingKinds = map[string]bool{"repository": true, "machine": true, "project": true}

// Route decides where a jot goes. It returns the destination and never an
// error: a jot that cannot be routed still gets filed, because refusing to file
// something because its destination is unclear is the failure this whole
// surface exists to avoid.
func Route(text string, routing []record.Record) Destination {
	var fallback *record.Record
	matches := map[string]record.Record{}

	for _, r := range routing {
		if !routingKinds[r.Kind] {
			continue
		}
		if v, ok := r.Get(DefaultField); ok && strings.EqualFold(strings.TrimSpace(v), DefaultValue) {
			candidate := r
			fallback = &candidate
		}
		for _, name := range namesOf(r) {
			if mentions(text, name) {
				matches[r.ID] = r
			}
		}
	}

	narrowed := narrow(matches)

	switch len(narrowed) {
	case 1:
		for _, r := range narrowed {
			return Destination{ID: r.ID, Name: r.Title, Prefix: prefixOf(r), Why: fmt.Sprintf("the jot names %s", r.Title)}
		}
	case 0:
		// Nothing obvious. That is the ordinary case and not a problem.
		if fallback != nil {
			return Destination{ID: fallback.ID, Name: fallback.Title, Prefix: prefixOf(*fallback), Why: "no destination is obvious"}
		}
		return Destination{Why: "no destination is obvious, and the routing registry declares no default"}
	}

	// More than one, and none of them contains the others. That is an ambiguous
	// hint rather than an obvious one, and choosing between them would be the
	// decision this surface refuses to ask for. It says what it saw either way:
	// an ambiguity reported as "nothing obvious" would hide the one case where
	// the routing registry needs an alias.
	named := make([]string, 0, len(narrowed))
	for _, r := range narrowed {
		named = append(named, r.Title)
	}
	sort.Strings(named)
	why := "the jot names more than one destination: " + strings.Join(named, ", ")
	if fallback != nil {
		return Destination{ID: fallback.ID, Name: fallback.Title, Prefix: prefixOf(*fallback), Why: why}
	}
	return Destination{Why: why + ", and the routing registry declares no default"}
}

// narrow drops a matched destination that contains another matched one.
//
// A project lists its repositories, so a jot naming "Mustur" matches the
// project MUS-P-0001 and the repository DevOfPie/Mustur inside it — which is
// not an ambiguity but the same place at two scales. Without this, no jot
// naming this repository could ever route to it: every one fell back to the
// inbox reporting two destinations, including a jot giving the full
// unambiguous name. Found by review, after an earlier version of this file
// documented the rule without containing it.
//
// A container is recognised by citing the other's identifier in its own
// fields, not by a rule about kinds. The registry is data; code that knew
// projects contain repositories would be a second place to keep that true.
func narrow(matches map[string]record.Record) map[string]record.Record {
	if len(matches) < 2 {
		return matches
	}
	out := map[string]record.Record{}
	for id, r := range matches {
		contains := false
		for otherID := range matches {
			if otherID != id && cites(r, otherID) {
				contains = true
				break
			}
		}
		if !contains {
			out[id] = r
		}
	}
	if len(out) == 0 {
		// Every match cites another: a cycle in the registry. Report the
		// ambiguity rather than picking one arbitrarily.
		return matches
	}
	return out
}

func cites(r record.Record, id string) bool {
	for _, cited := range r.Cites() {
		if cited == id {
			return true
		}
	}
	return false
}

// namesOf is what a jot can say to name a routing record: its title, and any
// aliases the record itself declares. Aliases live on the record so that adding
// one is a record written rather than code changed.
func namesOf(r record.Record) []string {
	names := []string{strings.TrimSpace(r.Title)}
	if aliases, ok := r.Get("Aliases"); ok {
		for _, a := range strings.Split(aliases, ",") {
			if a = strings.TrimSpace(a); a != "" {
				names = append(names, a)
			}
		}
	}
	// A repository named `DevOfPie/Mustur` is named by "Mustur" too. The owner
	// types the short name and should not have to know the long one.
	for _, n := range append([]string{}, names...) {
		if _, short, ok := strings.Cut(n, "/"); ok && short != "" {
			names = append(names, short)
		}
	}
	return names
}

// mentions reports whether text names something, on word boundaries and
// case-insensitively. Without the boundaries "Mustur" would match inside a
// longer word and route a jot about something else.
func mentions(text, name string) bool {
	if len(name) < 3 {
		// Too short to be a hint rather than a coincidence.
		return false
	}
	re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(name) + `\b`)
	if err != nil {
		return false
	}
	return re.MatchString(text)
}

var whitespace = regexp.MustCompile(`\s+`)

// Title derives a record's one-line claim from the jot, because asking for one
// is asking for a decision. It is the first sentence or line, whichever comes
// first, trimmed to something a listing can show.
func Title(text string) string {
	flat := strings.TrimSpace(text)
	if flat == "" {
		return ""
	}
	if line, _, ok := strings.Cut(flat, "\n"); ok {
		flat = line
	}
	if sentence, rest, ok := strings.Cut(flat, ". "); ok && len(sentence) > 10 {
		_ = rest
		flat = sentence
	}
	flat = whitespace.ReplaceAllString(strings.TrimSpace(flat), " ")
	const limit = 96
	if len(flat) <= limit {
		return flat
	}
	cut := strings.LastIndex(flat[:limit], " ")
	if cut < limit/2 {
		cut = limit
	}
	return strings.TrimSpace(flat[:cut]) + "…"
}

// Request is one filing. A struct rather than a parameter list because the
// caller is a form, and a form grows fields.
type Request struct {
	Project string
	Text    string
	Actor   string
	// To is a routing record's identifier when the filer chose one, and empty
	// when they left it to Mustur. An explicit choice is never overruled by the
	// guess: the point of offering it is that somebody knows something the text
	// does not say.
	To  string
	Now time.Time
	// Deliberate says this filing is somebody's decision rather than a browser
	// repeating itself, and turns the duplicate window off.
	//
	// The window exists for a phone on a flaky connection sending the same POST
	// three times. A correction re-files a record's own body on purpose, so it
	// matches its own original exactly — and was handed that original back and
	// told it was already routed there, which was false and unfixable for the
	// first minute after filing. That is the minute in which somebody notices
	// the routing was wrong (MUS-F-0056).
	Deliberate bool
}

// File writes a jot into the store as a finding and returns the record.
//
// It takes the clock from the request rather than reading it, so a caller can
// be tested and so the record's date is the caller's decision rather than this
// package's.
func File(ctx context.Context, s *store.Store, req Request) (record.Record, Destination, error) {
	project, text, actor, now := req.Project, req.Text, req.Actor, req.Now
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return record.Record{}, Destination{}, fmt.Errorf("nothing to file")
	}
	if existing, err := recentlyFiled(ctx, s, trimmed, actor, now); err != nil {
		return record.Record{}, Destination{}, err
	} else if existing != nil && !req.Deliberate {
		// A retry of the same POST, which is what a phone on a flaky
		// connection actually sends. Post-redirect-get protects a reload after
		// the redirect and nothing at all before it, so the same jot arrived
		// three times and became three records.
		//
		// The cost is that two genuinely identical jots inside the window
		// collapse into one. For a capture box that is the right side to err
		// on: a duplicate the owner has to notice and delete is worse than a
		// second copy of a thought they already had.
		to := Destination{Name: fieldOr(*existing, "Routed to", "nowhere"), Why: fieldOr(*existing, "Routing", "")}
		return *existing, to, nil
	}

	routing, err := routingRecords(ctx, s)
	if err != nil {
		return record.Record{}, Destination{}, err
	}
	to, err := chosen(routing, req.To)
	if err != nil {
		return record.Record{}, Destination{}, err
	}
	if to.ID == "" {
		to = Route(trimmed, routing)
	}

	r := record.Record{
		Kind:  "finding",
		Title: Title(trimmed),
		At:    now.Format("2006-01-02"),
		Body:  trimmed,
		Data: []record.Field{
			{Key: "Evidence", Value: ""},
			{Key: "Status", Value: "unreviewed"},
			{Key: "Routed to", Value: routedTo(to)},
			{Key: "Routing", Value: to.Why},
			{Key: "Filed by", Value: actor},
		},
	}
	if to.ID != "" {
		r.Refs = []record.Field{{Key: "Routed to", Value: to.ID}}
	}
	// Where it routes decides what it is called. A jot in the idea inbox is not
	// a Mustur record and no longer says it is (MUS-Q-0030, MUS-Q-0031).
	under := project
	if to.Prefix != "" {
		under = to.Prefix
	}
	written, err := s.Create(ctx, r, under, ident.Finding, actor)
	if err != nil {
		return record.Record{}, Destination{}, err
	}
	return written, to, nil
}

// Window is how long a repeat of the same text from the same filer is treated
// as a retry rather than a second jot.
const Window = time.Minute

func recentlyFiled(ctx context.Context, s *store.Store, text, actor string, now time.Time) (*record.Record, error) {
	recent, err := s.Since(ctx, "finding", now.Add(-Window))
	if err != nil {
		return nil, err
	}
	for _, r := range recent {
		if r.Body == text && fieldOr(r, "Filed by", "") == actor {
			match := r
			return &match, nil
		}
	}
	return nil, nil
}

func fieldOr(r record.Record, key, fallback string) string {
	if v, ok := r.Get(key); ok {
		return v
	}
	return fallback
}

// chosen resolves an explicitly picked destination. An identifier that is not a
// routing record is an error rather than a silent fallback to the guess: the
// filer said something, and quietly ignoring it would file the jot somewhere
// they did not choose while telling them it was filed.
func chosen(routing []record.Record, id string) (Destination, error) {
	if strings.TrimSpace(id) == "" {
		return Destination{}, nil
	}
	for _, r := range routing {
		if r.ID == id {
			// The prefix comes from the destination however the destination was
			// arrived at. A jot routed to the idea inbox by the guess was filed
			// under IDW while the same jot sent there deliberately was filed
			// under the store's prefix — the identifier depended on how the
			// choice was made rather than on where the record went.
			return Destination{
				ID: r.ID, Name: r.Title, Prefix: prefixOf(r),
				Why: "chosen by the filer",
			}, nil
		}
	}
	return Destination{}, fmt.Errorf("%s is not a destination this registry holds", id)
}

// Destinations returns the routing records a filer may choose between, sorted
// the way every listing here sorts.
func Destinations(ctx context.Context, s *store.Store) ([]record.Record, error) {
	return routingRecords(ctx, s)
}

// prefixOf reads a routing record's identifier prefix, and returns nothing for
// one that does not name a valid prefix rather than filing under a malformed
// one. A jot filed under the store's own prefix is the old behaviour, which is
// wrong in a way somebody can see; a jot filed under "IDWX" is wrong in a way
// that breaks the identifier scheme.
func prefixOf(r record.Record) string {
	v, ok := r.Get(PrefixField)
	if !ok {
		return ""
	}
	v = strings.ToUpper(strings.TrimSpace(v))
	if !ident.ValidProject(v) {
		return ""
	}
	return v
}

func routedTo(d Destination) string {
	if d.ID == "" {
		return "nowhere"
	}
	return fmt.Sprintf("%s (%s)", d.Name, d.ID)
}

func routingRecords(ctx context.Context, s *store.Store) ([]record.Record, error) {
	var out []record.Record
	for kind := range routingKinds {
		rs, err := s.List(ctx, kind)
		if err != nil {
			return nil, err
		}
		out = append(out, rs...)
	}
	record.Sort(out)
	return out, nil
}
