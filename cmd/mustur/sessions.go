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
	"encoding/json"
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
		db := dbFlag(fs)
		project, err := parseWithPositional(fs, rest, "session start needs a project")
		if err != nil {
			return err
		}
		// The store is opened so the session can be written down, not so it can
		// be consulted: what is running is still tmux's answer. A store that
		// will not open is not a reason to refuse to start a session, so it
		// costs the note and nothing else.
		if st, sctx, err := openStore(*db); err == nil {
			defer st.Close()
			a.Remember, a.DB, ctx = st, *db, sctx
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
		gate := fs.String("gate", "", "tools to hold in front of the owner, comma-separated")
		if err := fs.Parse(rest); err != nil || *dir == "" || *project == "" {
			return nil
		}
		payload, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
		if err != nil {
			return nil
		}
		now := time.Now()
		session.RecordHookEvent(*dir, *project, payload, now)

		// And the half that answers rather than records. A tool call in the
		// gate is held here, in this process, until the owner presses on the
		// surface or the CLI's own timeout kills this process and draws the
		// dialog it would have drawn (MUS-D-0153). Everything else returns now
		// and decides nothing, which is what every call did before milestone 8.
		ask, ok := session.AskFromPayload(payload, now)
		if !ok || !session.Gated(session.ParseGate(*gate), ask.Mode, ask.Tool) {
			return nil
		}
		if err := session.RaiseAsk(*dir, *project, ask); err != nil {
			return nil // Total, like the recording half: no gate beats no session.
		}
		defer session.DropAsk(*dir, *project, ask.ID)
		answer, answered := session.AwaitAnswer(ctx, *dir, *project, ask.ID)
		if !answered {
			return nil
		}
		fmt.Println(session.Decision(answer))
		return nil

	case "cli-started":
		// The other hook, and the only route to a conversation's identifier.
		//
		// The CLI names its own conversation in the payload it hands every hook
		// and nowhere else a caller can reach — not the command line, not the
		// pane. Written down here so that after a reboot the surface can offer
		// to open that conversation again rather than a fresh one
		// (MUS-Q-0083).
		//
		// Total, like the hook next door and for the same reason: a hook that
		// fails is a hook interfering with the agent it is watching. A missing
		// store, a payload that will not parse and a project nobody is holding
		// all end the same way, with nothing written and nothing said.
		fs := flag.NewFlagSet("session cli-started", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		db := dbFlag(fs)
		project := fs.String("project", "", "the session the conversation belongs to")
		if err := fs.Parse(rest); err != nil || *project == "" {
			return nil
		}
		payload, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
		if err != nil {
			return nil
		}
		var p struct {
			SessionID  string `json:"session_id"`
			Transcript string `json:"transcript_path"`
		}
		if err := json.Unmarshal(payload, &p); err != nil || p.SessionID == "" {
			return nil
		}
		s, sctx, err := openStore(*db)
		if err != nil {
			return nil
		}
		defer s.Close()
		_ = s.NoteSessionCLI(sctx, *project, p.SessionID, p.Transcript)
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
		db := dbFlag(fs)
		project, err := parseWithPositional(fs, rest, "session stop needs a project")
		if err != nil {
			return err
		}
		// So a session the owner ended stops being offered back. Same as start:
		// a store that will not open costs the note, not the command.
		if st, sctx, err := openStore(*db); err == nil {
			defer st.Close()
			a.Remember, ctx = st, sctx
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
