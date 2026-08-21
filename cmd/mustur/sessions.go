package main

// `mustur session` — the per-machine adapter at the command line.
//
// There is no `session attach`. Mustur starts sessions and never attaches to
// one it did not start, and a person wanting to look at one Mustur *did* start
// uses `tmux attach -t mustur/<project>` — the arrow points one way, and adding
// a verb here that looked symmetrical would suggest otherwise.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/DevOfPie/Mustur/internal/session"
)

func cmdSession(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("session needs a verb: start, list, send, stop")
	}
	verb, rest := args[0], args[1:]
	a := &session.Adapter{}
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

	case "send":
		fs := flag.NewFlagSet("session send", flag.ContinueOnError)
		text := fs.String("text", "", "what to type into the session")
		project, err := parseWithPositional(fs, rest, "session send needs a project")
		if err != nil {
			return err
		}
		if strings.TrimSpace(*text) == "" {
			return fmt.Errorf("session send needs --text")
		}
		if err := a.Send(ctx, project, *text); err != nil {
			return err
		}
		fmt.Printf("sent to %s%s\n", session.Prefix, project)
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
		return fmt.Errorf("session has no verb %q: start, list, send, stop", verb)
	}
}
