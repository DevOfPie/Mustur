// Package session is the per-machine adapter: it starts long-lived agent
// sessions inside tmux, reports which are running, can type into one, and can
// stop one.
//
// **It still does not restart anything.** Start, List, Alive, Send and Stop are
// what this file provides; Start verifies the session is there afterwards,
// because tmux reports success whether or not the command survived and an agent
// CLI crashing on startup used to read as a started session.
//
// Output capture and noticing a death arrived with milestone 4b and live in
// stream.go, next door. They apply **while a viewer is attached** — the reader
// is opened by a viewer and lingers after one leaves, so a session nobody has
// looked at is not being watched. An earlier version of this comment said the
// package did neither at all, which stopped being true in the same package.
//
// Two decisions shape the whole package.
//
// **tmux is the source of truth** (MUS-Q-0013). tmux already knows which
// sessions exist, which are alive and what they last printed, and it knows it a
// second before any mirror would. So nothing here keeps a table of sessions:
// listing is a live query, and the store holds only what outlives a session.
// The cost, accepted rather than discovered: when the tmux server dies, so does
// every session and everything Mustur knew about them.
//
// **Mustur starts sessions and never attaches to one it did not start**
// (MUS-D-0007). Enforcement is by provenance, not by name: Start sets a tmux
// user option on the session it creates, and List returns only sessions
// carrying it. Send and Stop go through the same filter.
//
// The first version of this package filtered on the `mustur/` name prefix
// alone, which a review broke in one command — `tmux new-session -s
// mustur/anything` by hand, and Mustur listed it, typed into it and killed it.
// A name is something anyone can write; the option is something only a session
// this package started has. The prefix is kept for legibility, so a person
// running `tmux ls` can see which sessions are Mustur's, and it is no longer
// what the rule rests on.
package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Prefix names every tmux session Mustur started, so a person running `tmux ls`
// can tell which are Mustur's. It is legibility, not enforcement — anyone can
// name a session this.
const Prefix = "mustur/"

// OwnedOption is the tmux user option Start sets on a session it creates, and
// the only thing "Mustur started this" rests on. A session that does not carry
// it is not ours however it is named.
const OwnedOption = "@mustur_started"

// DeliverTimeout bounds a delivery. Without one, an unresponsive tmux holds an
// answer unwritten for as long as it likes, and the answer is the part that
// must not wait.
const DeliverTimeout = 10 * time.Second

// Session is one agent session, as tmux reports it.
type Session struct {
	// Name is the tmux session name, including the prefix.
	Name string
	// Project is the part after the prefix: which project this is for.
	Project string
	// Windows is how many windows the session holds; 0 means tmux reported
	// none, which should not happen for a live session.
	Windows int
	// Attached reports whether a terminal is attached right now. A session
	// Mustur started is usually detached, and that is not a health signal.
	Attached bool
}

// Runner runs a command and returns its combined output. Injected so the
// package can be tested without tmux, and so the one place that shells out is
// one place.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// Adapter starts and inspects sessions on this machine.
type Adapter struct {
	// Run shells out. Nil means the real tmux on this machine.
	Run Runner
	// Stat checks a directory exists. Nil means the real filesystem; injected
	// so the check itself is testable.
	Stat func(dir string) error
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

func (a *Adapter) runner() Runner {
	if a.Run != nil {
		return a.Run
	}
	return execRunner{}
}

// safeProject is what a project name may contain in a tmux session name.
//
// The guard is right and the first reason given for it was wrong. tmux does not
// create a session whose name carries `:` or `.` — measured on 3.6, it
// substitutes `_`, so `new-session -s 'a:0'` yields `a_0`. The danger is on the
// other side: `send-keys -t 'a:0'` reads that as window 0 of session `a` and
// delivers there. So a name tmux quietly rewrote at creation would be a name
// that addresses something else at send time, and the two would not match.
var safeProject = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// NameFor builds the tmux session name for a project.
func NameFor(project string) (string, error) {
	if !safeProject.MatchString(project) {
		return "", fmt.Errorf("project %q must be letters, digits, dash or underscore: tmux reads : and . as target separators", project)
	}
	return Prefix + project, nil
}

// Start launches a session for a project, running cmd in dir. It refuses to
// start a second session for a project that already has one — the milestone
// says a long-lived session *per project*, and two would make "the session for
// Mustur" ambiguous everywhere downstream.
func (a *Adapter) Start(ctx context.Context, project, dir, cmd string) (Session, error) {
	name, err := NameFor(project)
	if err != nil {
		return Session{}, err
	}
	if strings.TrimSpace(cmd) == "" {
		return Session{}, fmt.Errorf("start needs a command: the adapter shells out to whatever CLI is configured and has no default of its own")
	}
	live, err := a.Alive(ctx, project)
	if err != nil {
		return Session{}, err
	}
	if live {
		return Session{}, fmt.Errorf("%s already has a session; one per project", project)
	}
	args := []string{"new-session", "-d", "-s", name}
	if d := strings.TrimSpace(dir); d != "" {
		// tmux falls back to $HOME for a directory that is not there, so a
		// session reported as running in a checkout would be running in the
		// home directory. Refused here rather than discovered later.
		if a.Stat != nil {
			if err := a.Stat(d); err != nil {
				return Session{}, fmt.Errorf("%s is not a directory this session can start in: %w", d, err)
			}
		} else if info, err := os.Stat(d); err != nil || !info.IsDir() {
			return Session{}, fmt.Errorf("%s is not a directory this session can start in", d)
		}
		args = append(args, "-c", d)
	}
	args = append(args, cmd)
	if out, err := a.runner().Run(ctx, "tmux", args...); err != nil {
		return Session{}, fmt.Errorf("tmux new-session: %w: %s", err, strings.TrimSpace(out))
	}

	// Marked before anything else can see it. Until this runs the session is
	// not ours by the only test that counts.
	if out, err := a.runner().Run(ctx, "tmux", "set-option", "-t", name, OwnedOption, "1"); err != nil {
		return Session{}, fmt.Errorf("tmux set-option %s: %w: %s", OwnedOption, err, strings.TrimSpace(out))
	}

	// tmux new-session succeeds whether or not the command survives it, so a
	// CLI that exits immediately — crashing on startup is the case that
	// matters — was reported as a started session and then silently was not
	// one.
	//
	// Asking once is not enough: tmux does not reap the session synchronously,
	// so an immediate exit was still listed for a moment and the check passed.
	// This watches for the short window instead. It catches a command that dies
	// at once, which is the common failure; it is not supervision, and a CLI
	// that crashes a second later is still reported as started.
	if err := a.settle(ctx, project, name, cmd); err != nil {
		return Session{}, err
	}
	return Session{Name: name, Project: project}, nil
}

// SettleFor is how long Start watches a new session before believing in it.
var SettleFor = 400 * time.Millisecond

func (a *Adapter) settle(ctx context.Context, project, name, cmd string) error {
	deadline := time.Now().Add(SettleFor)
	for {
		live, err := a.Alive(ctx, project)
		if err != nil {
			return err
		}
		if !live {
			return fmt.Errorf("%s exited immediately; no session is running. The command was: %s", name, cmd)
		}
		if !time.Now().Before(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// List returns every session Mustur started on this machine, and nothing else.
func (a *Adapter) List(ctx context.Context) ([]Session, error) {
	out, err := a.runner().Run(ctx, "tmux", "list-sessions", "-F",
		"#{session_name}\t#{session_windows}\t#{session_attached}\t#{"+OwnedOption+"}")
	if err != nil {
		// No server running is not an error: it is the honest answer that no
		// session exists. Distinguished by the message tmux gives, because an
		// exit status alone cannot tell it from a real failure.
		if noServer(out) {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-sessions: %w: %s", err, strings.TrimSpace(out))
	}
	var sessions []Session
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		name := parts[0]
		// Provenance, not the name. A session called mustur/anything that this
		// package did not start carries no option and is somebody else's.
		if len(parts) < 4 || strings.TrimSpace(parts[3]) == "" {
			continue
		}
		if !strings.HasPrefix(name, Prefix) {
			continue // Marked but misnamed: not something Start could produce.
		}
		s := Session{Name: name, Project: strings.TrimPrefix(name, Prefix)}
		fmt.Sscanf(parts[1], "%d", &s.Windows)
		s.Attached = strings.TrimSpace(parts[2]) == "1"
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// Alive reports whether a project has a live session.
func (a *Adapter) Alive(ctx context.Context, project string) (bool, error) {
	sessions, err := a.List(ctx)
	if err != nil {
		return false, err
	}
	for _, s := range sessions {
		if s.Project == project {
			return true, nil
		}
	}
	return false, nil
}

// Send types text into a project's session and presses Enter.
//
// This is the capability worth naming plainly rather than discovering later:
// **Mustur can type into a session it started.** It is how an answered decision
// reaches the session that raised it (MUS-Q-0014), and it is indistinguishable
// at the far end from the owner having typed it. Everything it can be made to
// type is therefore something the owner is accountable for, which is why the
// only caller is the answer path and why it refuses a session Mustur did not
// start.
//
// The text is sent with `-l`, literally, so a body containing something tmux
// would otherwise read as a key name arrives as characters.
func (a *Adapter) Send(ctx context.Context, project, text string) error {
	name, err := NameFor(project)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("nothing to send")
	}
	live, err := a.Alive(ctx, project)
	if err != nil {
		return err
	}
	if !live {
		return fmt.Errorf("%s has no session Mustur started", project)
	}
	if out, err := a.runner().Run(ctx, "tmux", "send-keys", "-t", name, "-l", text); err != nil {
		return fmt.Errorf("tmux send-keys: %w: %s", err, strings.TrimSpace(out))
	}
	// Enter is a separate call: sending it with -l would type the word.
	if out, err := a.runner().Run(ctx, "tmux", "send-keys", "-t", name, "Enter"); err != nil {
		return fmt.Errorf("tmux send-keys Enter: %w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

// Stop ends a session Mustur started.
func (a *Adapter) Stop(ctx context.Context, project string) error {
	name, err := NameFor(project)
	if err != nil {
		return err
	}
	live, err := a.Alive(ctx, project)
	if err != nil {
		return err
	}
	if !live {
		return fmt.Errorf("%s has no session Mustur started", project)
	}
	if out, err := a.runner().Run(ctx, "tmux", "kill-session", "-t", name); err != nil {
		return fmt.Errorf("tmux kill-session: %w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

// noServer reports whether tmux failed because nothing is running, rather than
// because something went wrong.
func noServer(out string) bool {
	s := strings.ToLower(out)
	return strings.Contains(s, "no server running") ||
		strings.Contains(s, "error connecting to") ||
		strings.Contains(s, "no such file or directory")
}
