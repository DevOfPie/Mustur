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
)

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

// Buried returns the open questions that were never surfaced as a prompt.
// These are the ones that block reporting work complete.
func Buried(records []record.Record) []record.Record {
	var out []record.Record
	for _, r := range Open(records) {
		if !Surfaced(r) {
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
	fmt.Fprintf(&b, "%d open question(s) never surfaced as a prompt:\n", len(buried))
	for _, r := range buried {
		fmt.Fprintf(&b, "  %s  %s\n", r.ID, r.Title)
		if blocks, ok := r.Get(FieldBlocks); ok && strings.TrimSpace(blocks) != "" {
			fmt.Fprintf(&b, "         blocks: %s\n", strings.TrimSpace(blocks))
		}
	}
	b.WriteString("\nPut each one in a prompt, then record that it was surfaced:\n")
	b.WriteString("  mustur surfaced <ID>\n\n")
	b.WriteString("Writing the question into prose, a report or a pull request body is not\n")
	b.WriteString("surfacing it. An answer is not required to proceed; the owner may be away.\n")
	b.WriteString("Being asked is.")
	return fmt.Errorf("%s", b.String())
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
