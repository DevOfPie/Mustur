// Package question is the open-decision lifecycle and the gate that makes it
// impossible to report work complete around one.
//
// The constraint the package is shaped by: **a question the owner never saw is
// worse than no question.** An agent that writes a well-formed decision into a
// pull request body, with options and costs and a recommendation, has done the
// thorough thing and still failed — completeness is not what makes a request
// findable, arriving on the prompt surface is. So the record tracks not whether
// a question was *asked* but whether it was *surfaced*, and the gate reads that
// field and nothing else.
//
// A question is not a decision. Some become one, once answered; the ones that
// were only instructions do not, which is why they are a separate kind rather
// than a status on an append-only record.
package question

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DevOfPie/Mustur/internal/record"
)

// Kind is the record kind this package reads.
const Kind = "question"

// The fields a question carries beyond title and body.
const (
	// FieldStatus is open, answered or withdrawn.
	FieldStatus = "Status"
	// FieldSurfaced is when the question reached a prompt. Empty means it
	// never did, which is the only thing the gate cares about.
	FieldSurfaced = "Surfaced"
	// FieldAnswer is what the owner said.
	FieldAnswer = "Answer"
	// FieldAnswered is when they said it.
	FieldAnswered = "Answered"
	// FieldSession identifies the session that raised the question, so an
	// answer can be routed back to it once anything can route to a session.
	FieldSession = "Session"
	// FieldBlocks says what is stopped until this is answered, in words. The
	// identifier form is a ref; this is for a reader on a phone deciding
	// whether a question holds up a milestone or a sentence.
	FieldBlocks = "Blocks"
	// FieldNeeded marks a question the work in hand cannot proceed without.
	// Surfacing is enough for everything else; for these the answer itself is
	// required, because reporting work complete that depended on an answer
	// nobody gave is the same lie as never having asked.
	FieldNeeded = "Needed to proceed"
	// FieldAskedBy is who raised it. The raiser may withdraw a question but may
	// not answer one, so the gate cannot be walked around in a single command.
	FieldAskedBy = "Asked by"
)

// Yes is the affirmative value for the boolean-ish fields above.
const Yes = "yes"

// The values FieldStatus takes.
const (
	StatusOpen      = "open"
	StatusAnswered  = "answered"
	StatusWithdrawn = "withdrawn"
)

// Status reports a question's status, defaulting to open. A record with no
// status field is open rather than invalid: the failure this package exists to
// prevent is a question going unseen, and treating a malformed one as closed
// would be that failure with extra steps.
func Status(r record.Record) string {
	v, ok := r.Get(FieldStatus)
	if !ok {
		return StatusOpen
	}
	switch s := strings.ToLower(strings.TrimSpace(v)); s {
	case StatusAnswered, StatusWithdrawn:
		return s
	default:
		return StatusOpen
	}
}

// Surfaced reports whether the question ever reached a prompt.
func Surfaced(r record.Record) bool {
	v, _ := r.Get(FieldSurfaced)
	return strings.TrimSpace(v) != ""
}

// IsOpen reports whether the question still wants an answer.
func IsOpen(r record.Record) bool { return Status(r) == StatusOpen }

// Open returns every open question, in identifier order.
func Open(records []record.Record) []record.Record {
	var out []record.Record
	for _, r := range records {
		if r.Kind == Kind && IsOpen(r) {
			out = append(out, r)
		}
	}
	sortByID(out)
	return out
}

// Needed reports whether the work in hand depends on this question's answer.
func Needed(r record.Record) bool {
	v, _ := r.Get(FieldNeeded)
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "no", "false":
		return false
	default:
		return true
	}
}

// AskedBy is who raised the question, empty if the record does not say.
func AskedBy(r record.Record) string {
	v, _ := r.Get(FieldAskedBy)
	return strings.TrimSpace(v)
}

// Buried returns the open questions that block reporting work complete.
//
// Two ways to qualify. One was never surfaced as a prompt, so nobody was asked.
// The other was surfaced and is marked as one the work in hand cannot proceed
// without — the owner's qualification when they ratified the rule: being asked
// is enough "as long as the work it is doing doesn't depend on the question's
// answer". Reporting complete on work that turned on an answer nobody gave is
// the same lie as never having asked.
func Buried(records []record.Record) []record.Record {
	var out []record.Record
	for _, r := range Open(records) {
		if !Surfaced(r) || Needed(r) {
			out = append(out, r)
		}
	}
	return out
}

// Gate returns an error naming every buried question, and nil when there are
// none. The error is the whole point: it is what an agent runs into instead of
// reporting work complete.
func Gate(records []record.Record) error {
	buried := Buried(records)
	if len(buried) == 0 {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d open question(s) block reporting this work complete:\n", len(buried))
	var anyUnsurfaced, anyNeeded bool
	for _, r := range buried {
		why := "never surfaced as a prompt"
		if Surfaced(r) {
			why = "the work depends on the answer, and there is none yet"
			anyNeeded = true
		} else {
			anyUnsurfaced = true
		}
		fmt.Fprintf(&b, "  %s  %s\n", r.ID, r.Title)
		fmt.Fprintf(&b, "         %s\n", why)
		if blocks, ok := r.Get(FieldBlocks); ok && strings.TrimSpace(blocks) != "" {
			fmt.Fprintf(&b, "         blocks: %s\n", strings.TrimSpace(blocks))
		}
	}
	if anyUnsurfaced {
		b.WriteString("\nPut each unsurfaced one in a prompt, then record that it was surfaced:\n")
		b.WriteString("  mustur surfaced <ID>\n\n")
		b.WriteString("Writing the question into prose, a report or a pull request body is not\n")
		b.WriteString("surfacing it. Being asked is what this wants.\n")
	}
	if anyNeeded {
		b.WriteString("\nThe ones marked as needed cannot be waited out. Either the owner answers\n")
		b.WriteString("them, or the work that depends on them is not what gets reported complete:\n")
		b.WriteString("do everything independent of the answer first, which is what the contract\n")
		b.WriteString("asks for anyway.\n")
	}
	return fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
}

// MarkSurfaced records that the question reached a prompt. It says nothing
// about whether it was answered, deliberately: the gate turns on being asked,
// not on being answered, because an owner who is away must not stop the work.
func MarkSurfaced(r *record.Record, at string) { Set(r, FieldSurfaced, at) }

// Answer closes a question with what the owner said.
func Answer(r *record.Record, answer, at string) {
	Set(r, FieldStatus, StatusAnswered)
	Set(r, FieldAnswer, answer)
	Set(r, FieldAnswered, at)
}

// Withdraw closes a question that no longer wants an answer: overtaken by
// events, or answered by something other than the owner saying so.
func Withdraw(r *record.Record, at string) {
	Set(r, FieldStatus, StatusWithdrawn)
	Set(r, FieldAnswered, at)
}

// Set replaces a field in place, keeping its position, or appends it. Position
// matters because these are read on a phone, and a status that moves around
// the table between renders is one a reader has to hunt for.
func Set(r *record.Record, key, value string) {
	for i := range r.Data {
		if r.Data[i].Key == key {
			r.Data[i].Value = value
			return
		}
	}
	r.Data = append(r.Data, record.Field{Key: key, Value: value})
}

func sortByID(rs []record.Record) {
	sort.Slice(rs, func(i, j int) bool { return rs[i].ID < rs[j].ID })
}
