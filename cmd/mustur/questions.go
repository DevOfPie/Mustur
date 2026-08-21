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

func cmdSurfaced(args []string) error {
	fs := flag.NewFlagSet("surfaced", flag.ContinueOnError)
	db := dbFlag(fs)
	at := fs.String("at", "", "when it was surfaced (default now)")
	actor := fs.String("actor", defaultActor(), "who surfaced it")
	id, err := parseWithPositional(fs, args, "surfaced needs one identifier")
	if err != nil {
		return err
	}
	if *at == "" {
		*at = time.Now().Format("2006-01-02 15:04")
	}
	return setField(*db, id, *actor, func(r *record.Record) error {
		if r.Kind != question.Kind {
			return fmt.Errorf("%s is a %s, not a question", r.ID, r.Kind)
		}
		question.MarkSurfaced(r, *at)
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
	id, err := parseWithPositional(fs, args, "answer needs one identifier")
	if err != nil {
		return err
	}
	if strings.TrimSpace(*answer) == "" && !*withdraw {
		return fmt.Errorf("answer needs --answer, or --withdraw to close it without one")
	}
	if *at == "" {
		*at = time.Now().Format("2006-01-02 15:04")
	}
	return setField(*db, id, *actor, func(r *record.Record) error {
		if r.Kind != question.Kind {
			return fmt.Errorf("%s is a %s, not a question", r.ID, r.Kind)
		}
		if *withdraw {
			// Withdrawing your own question is honest: it is overtaken, or no
			// longer worth asking, and the record keeps saying it was asked.
			question.Withdraw(r, *at)
			return nil
		}
		// Answering your own is not. The owner's rule: the raiser may withdraw,
		// never answer, so the gate cannot be closed by the thing it is
		// enforcing against.
		if asker := question.AskedBy(*r); asker != "" && asker == *actor {
			return fmt.Errorf("%s was asked by %s, and %s cannot answer it.\n"+
				"An answer comes from the owner, through /questions or from someone else's hand.\n"+
				"If it is overtaken rather than answered, close it with --withdraw.",
				r.ID, asker, *actor)
		}
		question.Answer(r, *answer, *at)
		dctx, cancel := context.WithTimeout(context.Background(), session.DeliverTimeout)
		defer cancel()
		question.Set(r, question.FieldDelivered,
			session.Deliver(dctx, &session.Adapter{},
				question.ProjectOf(*r), r.ID, *answer))
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
