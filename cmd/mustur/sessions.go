package main

// `mustur session` — the per-machine adapter at the command line.
//
// There is no `session attach`. Mustur starts sessions and never attaches to
// one it did not start, and a person wanting to look at one Mustur *did* start
// uses `tmux attach -t mustur/<project>` — the arrow points one way, and adding
// a verb here that looked symmetrical would suggest otherwise.
//
// There is no `session send` either, and that absence is load-bearing. Typing
// into an agent's input is a capability the answer path needs and nothing else
// does; an operator verb taking arbitrary text made "the only caller is the
// answer path" false in the same commit that claimed it. A person who genuinely
// wants to type into a session has `tmux send-keys`, and does so as themselves
// rather than as Mustur.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/DevOfPie/Mustur/internal/session"
)

func cmdSession(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("session needs a verb: start, list, stop")
	}
	verb, rest := args[0], args[1:]
	a := &session.Adapter{HookDir: session.DefaultHookDir()}
	ctx := context.Background()

	switch verb {
	case "start":
		fs := flag.NewFlagSet("session start", flag.ContinueOnError)
		dir := fs.String("dir", "", "the checkout the session runs in")
		cmd := fs.String("cmd", "", "the CLI to run; the adapter has no default of its own")
		project, err := parseWithPositional(fs, rest, "session start needs a project")
		if err != nil {
			return err
		}
		s, err := a.Start(ctx, project, *dir, *cmd)
		if err != nil {
			return err
		}
		fmt.Printf("%s started\n  attach with  tmux attach -t %s\n", s.Name, s.Name)
		return nil

	case "subagent-event":
		// The hook the CLI calls, once per sub-agent lifecycle event and once
		// per tool call in the session. It is not an operator verb: nobody runs
		// this by hand, and it takes no text — it reads one JSON payload on
		// stdin and appends what is worth keeping.
		//
		// It always succeeds. A hook that fails is a hook interfering with the
		// agent it was watching, and a sub-agent row is not worth that.
		fs := flag.NewFlagSet("session subagent-event", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		dir := fs.String("dir", "", "where sub-agent events are logged")
		project := fs.String("project", "", "the session the event belongs to")
		if err := fs.Parse(rest); err != nil || *dir == "" || *project == "" {
			return nil
		}
		payload, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
		if err != nil {
			return nil
		}
		session.RecordHookEvent(*dir, *project, payload, time.Now())
		return nil

	case "list":
		sessions, err := a.List(ctx)
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			fmt.Println("no sessions Mustur started")
			fmt.Fprintln(os.Stderr, "A session left running in a terminal is not here and will not appear.")
			return nil
		}
		for _, s := range sessions {
			state := "detached"
			if s.Attached {
				state = "attached"
			}
			fmt.Printf("%-28s %s  %d window(s)\n", s.Name, state, s.Windows)
		}
		return nil

	case "stop":
		fs := flag.NewFlagSet("session stop", flag.ContinueOnError)
		project, err := parseWithPositional(fs, rest, "session stop needs a project")
		if err != nil {
			return err
		}
		if err := a.Stop(ctx, project); err != nil {
			return err
		}
		fmt.Printf("%s%s stopped\n", session.Prefix, project)
		return nil

	default:
		return fmt.Errorf("session has no verb %q: start, list, stop", verb)
	}
}
