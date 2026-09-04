package main

// The question lifecycle, at the command line. `ask` raises one, `surfaced`
// records that it reached a prompt, `answer` closes it, and `questions --gate`
// is the thing an agent runs into instead of reporting work complete.
//
// `surfaced` and `answer` set one field and keep the rest, which is not what
// `amend` does — an amendment states a record afresh, deliberately, so that
// `amend --title` cannot quietly carry forward data the writer never saw. That
// is right for a correction and wrong for a state change, so these read the
// record first and write it back whole.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/question"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/session"
)

// values is a repeatable flag whose argument is the whole value. `fields`
// demands key=value, which an option's prose would have to escape around.
type values []string

func (v *values) String() string { return strings.Join(*v, ", ") }

func (v *values) Set(s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("an empty option is not an answer anyone can pick")
	}
	// A label on its own is the bare question the contract asks nobody to send,
	// one level down: the owner is handed a word and left to work out what it
	// costs. --data refuses a value that is not key=value at this same boundary
	// (MUS-F-0058).
	if !strings.Contains(s, question.OptionSep) {
		return fmt.Errorf("%q is a label with nothing behind it: an option is "+
			"\"Label :: one line on what it costs :: the paragraph behind it\"", s)
	}
	*v = append(*v, s)
	return nil
}

func cmdAsk(args []string) error {
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	db := dbFlag(fs)
	title := fs.String("title", "", "the question, in one line")
	body := fs.String("body", "", "one short paragraph of context, from the window you already have")
	blocks := fs.String("blocks", "", "what is stopped until this is answered, in words")
	needed := fs.Bool("needed", false, "the work in hand cannot proceed without the answer; the gate will not pass on surfacing alone")
	var options values
	fs.Var(&options, "option", "an answer the owner can pick: \"Label :: one line :: the paragraph behind it\", repeatable")
	sessionID := fs.String("session", "", "identifies the session raising it, in whatever form that session has an identity")
	inProject := fs.String("in", "", "the Mustur-owned session to type the answer into, if this was raised from one")
	at := fs.String("at", "", "the date (default today)")
	project := fs.String("project", "MUS", "identifier prefix")
	actor := fs.String("actor", defaultActor(), "who is asking")
	var refs fields
	fs.Var(&refs, "ref", "a k=IDENTIFIER citation, repeatable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*title) == "" {
		return fmt.Errorf("ask needs a --title: the question itself, in one line")
	}
	if *at == "" {
		*at = time.Now().Format("2006-01-02")
	}

	s, ctx, err := openStore(*db)
	if err != nil {
		return err
	}
	defer s.Close()

	r := record.Record{
		Kind:  question.Kind,
		Title: *title,
		Body:  *body,
		At:    *at,
		Refs:  refs,
		Data:  []record.Field{{Key: question.FieldStatus, Value: question.StatusOpen}},
	}
	if strings.TrimSpace(*blocks) != "" {
		r.Data = append(r.Data, record.Field{Key: question.FieldBlocks, Value: *blocks})
	}
	if *needed {
		r.Data = append(r.Data, record.Field{Key: question.FieldNeeded, Value: question.Yes})
	}
	for _, o := range options {
		r.Data = append(r.Data, record.Field{Key: question.FieldOption, Value: o})
	}
	// Recorded so `answer` can refuse the raiser. Without it the gate is one
	// command away from being walked around by whoever it is enforcing against.
	r.Data = append(r.Data, record.Field{Key: question.FieldAskedBy, Value: *actor})
	if strings.TrimSpace(*sessionID) != "" {
		r.Data = append(r.Data, record.Field{Key: question.FieldSession, Value: *sessionID})
	}
	if strings.TrimSpace(*inProject) != "" {
		r.Data = append(r.Data, record.Field{Key: question.FieldProject, Value: *inProject})
	}

	role, ok := ident.RoleFor(question.Kind)
	if !ok {
		return fmt.Errorf("no role letter for kind %q", question.Kind)
	}
	written, err := s.Create(ctx, r, *project, role, *actor)
	if err != nil {
		return err
	}
	fmt.Println(written.ID)
	fmt.Fprintln(os.Stderr, "raised, and not yet surfaced. Put it in a prompt, then: mustur surfaced "+written.ID)
	return nil
}

// stamped is the time a lifecycle verb records, and it is the clock rather than
// whatever the caller typed.
//
// Every question raised in this repository up to 2026-08-24 records an answer
// timestamped before the question existed — MUS-Q-0034 was created at 09:53 and
// says it was answered at 09:00 — because the times were typed in by hand from
// a conversation that had already happened. The record is what says surfacing
// preceded the answer, and the gate leans on that; a field the writer chooses
// cannot carry it.
//
// So `surfaced` and `answer` stamp the clock. A --at in the past is refused
// rather than ignored, because silently overriding what somebody typed is how
// the next person learns nothing. Backdating a question raised offline is the
// use this gives up, and it has never happened. The owner took this on
// MUS-Q-0037.
func stamped(at string, now time.Time) (string, error) {
	if strings.TrimSpace(at) == "" {
		return now.Format("2006-01-02 15:04"), nil
	}
	when, err := time.ParseInLocation("2006-01-02 15:04", strings.TrimSpace(at), time.Local)
	if err != nil {
		when, err = time.ParseInLocation("2006-01-02", strings.TrimSpace(at), time.Local)
		if err != nil {
			return "", fmt.Errorf("--at %q is not a time this understands: use 2006-01-02 or 2006-01-02 15:04", at)
		}
	}
	// A minute of slack, so a run that straddles the boundary is not refused
	// for being a second stale.
	if when.Before(now.Add(-time.Minute)) {
		return "", fmt.Errorf("--at %s is in the past; this verb records when it ran, "+
			"because a hand-set time cannot show that surfacing came before the answer", at)
	}
	return when.Format("2006-01-02 15:04"), nil
}

func cmdSurfaced(args []string) error {
	fs := flag.NewFlagSet("surfaced", flag.ContinueOnError)
	db := dbFlag(fs)
	at := fs.String("at", "", "when it was surfaced (default now)")
	actor := fs.String("actor", defaultActor(), "who surfaced it")
	id, err := parseWithPositional(fs, args, "surfaced needs one identifier")
	if err != nil {
		return err
	}
	when, err := stamped(*at, time.Now())
	if err != nil {
		return err
	}
	return setField(*db, id, *actor, func(r *record.Record) error {
		if r.Kind != question.Kind {
			return fmt.Errorf("%s is a %s, not a question", r.ID, r.Kind)
		}
		question.MarkSurfaced(r, when)
		return nil
	})
}

func cmdAnswer(args []string) error {
	fs := flag.NewFlagSet("answer", flag.ContinueOnError)
	db := dbFlag(fs)
	answer := fs.String("answer", "", "what the owner said")
	withdraw := fs.Bool("withdraw", false, "close it without an answer: overtaken, or no longer worth asking")
	at := fs.String("at", "", "when (default now)")
	actor := fs.String("actor", defaultActor(), "who is recording the answer")
	// The asker may not answer, and may write down an answer given elsewhere.
	// The flag takes where, not a bare yes: an unattributed relay would be
	// worse than none, because it would read exactly like the owner having
	// answered here (MUS-Q-0059).
	relay := fs.String("from-owner", "",
		"record an answer the owner gave elsewhere, naming where they gave it")
	again := fs.Bool("reanswer", false,
		"write over an answer that is already recorded")
	id, err := parseWithPositional(fs, args, "answer needs one identifier")
	if err != nil {
		return err
	}
	if strings.TrimSpace(*answer) == "" && !*withdraw {
		return fmt.Errorf("answer needs --answer, or --withdraw to close it without one")
	}
	when, err := stamped(*at, time.Now())
	if err != nil {
		return err
	}
	return setField(*db, id, *actor, func(r *record.Record) error {
		if r.Kind != question.Kind {
			return fmt.Errorf("%s is a %s, not a question", r.ID, r.Kind)
		}
		if *withdraw {
			// Withdrawing your own question is honest: it is overtaken, or no
			// longer worth asking, and the record keeps saying it was asked.
			question.Withdraw(r, when)
			return nil
		}
		// Answering your own is not. The owner's rule: the raiser may withdraw,
		// never answer, so the gate cannot be closed by the thing it is
		// enforcing against.
		if asker := question.AskedBy(*r); asker != "" && asker == *actor && strings.TrimSpace(*relay) == "" {
			return fmt.Errorf("%s was asked by %s, and %s cannot answer it.\n"+
				"An answer comes from the owner, through /questions or from someone else's hand.\n"+
				"If the owner answered somewhere else, write it down with --from-owner naming where.\n"+
				"If it is overtaken rather than answered, close it with --withdraw.",
				r.ID, asker, *actor)
		}
		// Refusing to write over an answer that is already there.
		//
		// Nothing stopped this, and it cost a real record: MUS-Q-0056 had been
		// answered by the owner through the queue, and a relay written over it
		// replaced their words, moved the timestamp four hours, and added a
		// Relayed line claiming they had answered somewhere they had not. It
		// was restored from the log, which is the only reason this is a story
		// rather than a loss.
		//
		// An answered question is the one thing in the store that should be
		// hard to change by accident, because it is what everything downstream
		// was allowed to proceed on.
		if had, ok := r.Get(question.FieldAnswer); ok && strings.TrimSpace(had) != "" && !*again {
			return fmt.Errorf("%s is already answered:\n  %s\n"+
				"Writing over an answer replaces what the owner said. If that is really meant, pass --reanswer.",
				r.ID, strings.TrimSpace(had))
		}
		if where := strings.TrimSpace(*relay); where != "" {
			question.AnswerRelayed(r, *answer, when, *actor, where)
		} else {
			question.Answer(r, *answer, when)
		}
		dctx, cancel := context.WithTimeout(context.Background(), session.DeliverTimeout)
		defer cancel()
		// The provenance travels with the answer. An agent writing down the
		// owner's words, into a session the question names with --in, would
		// otherwise have its own line typed back at it saying "The owner
		// answered" (MUS-F-0085).
		var relayed string
		if v, ok := r.Get(question.FieldRelayed); ok {
			relayed = v
		}
		question.Set(r, question.FieldDelivered,
			session.DeliverRelayed(dctx, &session.Adapter{},
				question.ProjectOf(*r), r.ID, *answer, relayed))
		return nil
	})
}

func cmdQuestions(args []string) error {
	fs := flag.NewFlagSet("questions", flag.ContinueOnError)
	db := dbFlag(fs)
	all := fs.Bool("all", false, "include answered and withdrawn questions")
	gate := fs.Bool("gate", false, "exit non-zero if any open question was never surfaced")
	// The gate's source of truth is the exported tree, not the store. workflow.md
	// requires every gate to run offline against the working tree, and the store
	// is machine-local: reading it meant the check could only skip on a clone and
	// in CI, while CLAUDE.md told every session the gate was binding.
	records := fs.String("records", "", "read questions from this exported tree instead of the store")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *records != "" {
		qs, err := question.FromTree(*records)
		if err != nil {
			return err
		}
		if *gate {
			return question.Gate(qs)
		}
		return listQuestions(qs, *all)
	}

	s, ctx, err := openStore(*db)
	if err != nil {
		return err
	}
	defer s.Close()

	stored, err := s.List(ctx, "")
	if err != nil {
		return err
	}

	if *gate {
		return question.Gate(stored)
	}
	return listQuestions(stored, *all)
}

func listQuestions(records []record.Record, all bool) error {

	var shown []record.Record
	for _, r := range records {
		if r.Kind != question.Kind {
			continue
		}
		if !all && !question.IsOpen(r) {
			continue
		}
		shown = append(shown, r)
	}
	if len(shown) == 0 {
		if all {
			fmt.Println("no questions")
		} else {
			fmt.Println("no open questions")
		}
		return nil
	}
	for _, r := range shown {
		mark := " "
		if question.IsOpen(r) && !question.Surfaced(r) {
			mark = "!"
		}
		fmt.Printf("%s %s  %s  %s\n", mark, r.ID, question.Status(r), r.Title)
		if blocks, ok := r.Get(question.FieldBlocks); ok && strings.TrimSpace(blocks) != "" {
			fmt.Printf("    blocks: %s\n", strings.TrimSpace(blocks))
		}
		if ans, ok := r.Get(question.FieldAnswer); ok && strings.TrimSpace(ans) != "" {
			fmt.Printf("    answer: %s\n", strings.TrimSpace(ans))
		}
	}
	fmt.Println()
	fmt.Println("! means open and never surfaced as a prompt. Those block reporting work complete.")
	return nil
}

// setField reads a record, lets mutate change it, and writes it back whole.
//
// The op is "amend", because the store's log vocabulary is create and amend and
// widening it for a state change would make every reader of the log learn a
// third word to find out what the record's own fields already say.
func setField(db, id, actor string, mutate func(*record.Record) error) error {
	s, ctx, err := openStore(db)
	if err != nil {
		return err
	}
	defer s.Close()

	r, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := mutate(&r); err != nil {
		return err
	}
	if err := s.Append(ctx, r, "amend", actor); err != nil {
		return err
	}
	fmt.Println(r.ID)
	return nil
}
