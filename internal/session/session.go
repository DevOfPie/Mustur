// Package session is the per-machine adapter: it starts long-lived agent
// sessions inside tmux, supervises them there, and can type into one.
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
// (MUS-D-0006). Every session this package can see carries a name prefix it
// wrote itself, and anything without that prefix is invisible here — including
// the tmux session a person left running an hour ago. That is the week-one
// surprise the decision names, and it is enforced rather than documented:
// List filters by prefix, and Send refuses a target that does not carry it.
package session

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Prefix marks every tmux session Mustur started. It is how "never attach to a
// session it did not start" is enforced: a session without it is not ours, and
// no method here will act on one.
const Prefix = "mustur/"

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

// Adapter supervises sessions on this machine.
type Adapter struct {
	// Run shells out. Nil means the real tmux on this machine.
	Run Runner
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

// safeProject is what a project name may contain in a tmux session name. tmux
// treats `:` and `.` as target separators, so a name carrying either would
// address a window or a pane instead of a session.
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
	if strings.TrimSpace(dir) != "" {
		args = append(args, "-c", dir)
	}
	args = append(args, cmd)
	if out, err := a.runner().Run(ctx, "tmux", args...); err != nil {
		return Session{}, fmt.Errorf("tmux new-session: %w: %s", err, strings.TrimSpace(out))
	}
	return Session{Name: name, Project: project}, nil
}

// List returns every session Mustur started on this machine, and nothing else.
func (a *Adapter) List(ctx context.Context) ([]Session, error) {
	out, err := a.runner().Run(ctx, "tmux", "list-sessions", "-F", "#{session_name}\t#{session_windows}\t#{session_attached}")
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
		if !strings.HasPrefix(name, Prefix) {
			continue // Somebody else's. Not ours to see.
		}
		s := Session{Name: name, Project: strings.TrimPrefix(name, Prefix)}
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &s.Windows)
		}
		if len(parts) > 2 {
			s.Attached = strings.TrimSpace(parts[2]) == "1"
		}
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
