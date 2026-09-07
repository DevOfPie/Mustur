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
//
// The cost was accepted rather than discovered — when the tmux server dies, so
// does every session — and half of it has since been paid back. A reboot took
// the owner's sessions and Mustur could not afterwards say they had existed, so
// Start now writes down what it launched and Stop deletes what it ended
// (MUS-Q-0083). That is a note about the past, not a mirror: **nothing here
// reads it**, listing is still a live query, and what the note buys is a
// surface that can offer a lost session back rather than a session that comes
// back on its own. Mustur still restarts nothing.
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
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
)

// Prefix names every tmux session Mustur started, so a person running `tmux ls`
// can tell which are Mustur's. It is legibility, not enforcement — anyone can
// name a session this.
const Prefix = "mustur/"

// OwnedOption is the tmux user option Start sets on a session it creates, and
// the only thing "Mustur started this" rests on. A session that does not carry
// it is not ours however it is named.
const OwnedOption = "@mustur_started"

// PaneWidth and PaneHeight are the geometry Mustur gives a session it starts.
//
// A detached tmux session is 80x24, and an agent CLI runs on the alternate
// screen — measured: alternate_on=1, history_size=0 — which means tmux keeps no
// scrollback for it at all. Not a little: none. So on a default pane the whole
// of what could ever be shown was twenty-four rows, and there was nothing to
// scroll back through because there was nothing behind them.
//
// A tall pane is the answer, and it is nearly free. The CLI redraws its whole
// conversation into whatever height it is given, and a capture costs what the
// content costs rather than what the height is: measured on the same session,
// 60 rows was 4.8KB, 200 rows was 6.9KB and 300 rows was 7.0KB, because the
// difference is blank padding. The padding is trimmed before it is sent.
//
// The cost is real and worth naming: window-size is manual, so a person who
// attaches to one of these with tmux gets a 300-row window in their terminal
// rather than one sized to it. These are sessions Mustur starts and watches
// through the browser, and that is the trade.
const (
	PaneWidth  = 100
	PaneHeight = 300
)

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
	// Activity is when the session's pane last produced output, as tmux
	// reports it.
	//
	// window_activity, not session_activity. They are not the same thing and
	// the difference is hours: measured on a live session, session_activity was
	// 4.3 hours stale while window_activity was three minutes old, because
	// session_activity tracks the session rather than what is happening inside
	// it. Reading the wrong one made the quiet counter say a session that had
	// just spoken had been silent since it started (MUS-F-0042, again).
	//
	// It exists so that "the route row defaults to the last active session" can
	// be true. That clause is MUS-D-0013's and went unbuilt through milestone 5
	// because nothing here read activity: the list came back in tmux's own
	// order, which is by name, and alphabetical-as-default was described in the
	// records as last-active. Zero means tmux reported nothing parseable.
	Activity time.Time
}

// Runner runs a command and returns its combined output. Injected so the
// package can be tested without tmux, and so the one place that shells out is
// one place.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// Adapter starts and inspects sessions on this machine.
type Adapter struct {
	// buffers numbers the tmux paste buffers this adapter creates, so two
	// concurrent sends cannot name the same one.
	buffers atomic.Uint64

	// Run shells out. Nil means the real tmux on this machine.
	Run Runner
	// Stat checks a directory exists. Nil means the real filesystem; injected
	// so the check itself is testable.
	Stat func(dir string) error
	// HookDir is where sub-agent events are logged, and the switch that turns
	// sub-agent visibility on. Empty means Start installs no hook and the
	// session shows no sub-agents — which is what every session did before
	// milestone 4c, and what a session started by hand still does.
	HookDir string
	// Exe is the Mustur binary the hook calls back. Empty means this one.
	Exe string
	// DB is the store the SessionStart hook writes the CLI's conversation
	// identifier into. Empty leaves the path off the hook's command line, so it
	// falls back to this machine's default — which is right for every process
	// that did not override it, and wrong to guess at for one that did.
	DB string
	// Remember is told what Start launched and what Stop ended, so a session
	// lost with the machine can be offered back (MUS-Q-0083). Nil remembers
	// nothing.
	//
	// It is not a mirror of what is running and nothing here reads it back:
	// List, Alive and Stop still ask tmux, which is the source of truth
	// (MUS-D-0062). This only writes down what was launched.
	Remember Rememberer
}

// Rememberer is told what Start launched and what Stop ended.
//
// An interface rather than the store itself, so this package keeps shelling out
// to tmux as its only dependency and a caller with no store open — the answer
// path, the delivery path — passes nothing and remembers nothing.
type Rememberer interface {
	RememberSession(ctx context.Context, project, dir, cmd string) error
	ForgetSession(ctx context.Context, project string) error
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

// nextBuffer hands out a number no other send in this process is using.
func (a *Adapter) nextBuffer() uint64 { return a.buffers.Add(1) }

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
//
// A refused name says what is wrong with it and then what is allowed, in that
// order. The first version said only the second half and then explained tmux's
// target separators — so a name refused for containing a space was answered
// with a sentence about colons and full stops, which is true of the rule and
// irrelevant to what was typed. The reason the rule exists is above; a person
// who has just been refused wants the fact, not the derivation.
func NameFor(project string) (string, error) {
	if project == "" {
		return "", fmt.Errorf("a name is needed: letters, digits, dash or underscore")
	}
	if !safeProject.MatchString(project) {
		return "", fmt.Errorf("a name cannot contain %s: use letters, digits, dash or underscore", offending(project))
	}
	return Prefix + project, nil
}

// offending names what is wrong with a name, in the words somebody would use.
//
// One thing, not a list: the first character that is not allowed. A name with a
// space and a colon in it has two problems and fixing either is progress, and a
// refusal that enumerates is a refusal nobody finishes reading.
func offending(project string) string {
	for _, r := range project {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			continue
		case r == ' ':
			return "a space"
		case r == '\t':
			return "a tab"
		case unicode.IsSpace(r):
			return "whitespace"
		default:
			return fmt.Sprintf("%q", string(r))
		}
	}
	// Every character is allowed, so the name failed the pattern for its
	// length. The regexp requires at least one, and the empty case is caught
	// before this is reached.
	return "what it contains"
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
	// The command the owner configured, plus the hook that makes this session's
	// sub-agents visible — appended here and nowhere else, so nothing is
	// written to the owner's configuration (MUS-Q-0024). A command this package
	// does not recognise comes back unchanged.
	// Last session's sub-agents are not this one's. Cleared before the command
	// runs, so the first hook to fire writes into an empty log rather than
	// underneath rows belonging to a session that has already ended.
	ForgetSubagents(a.HookDir, project)
	args = append(args, withHook(cmd, a.exe(), a.HookDir, project, a.DB))
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

	// Written down after the session is believed in, so a command that died on
	// startup is not offered back as something to restore. The command recorded
	// is the one that was given, without the hook this function appended: the
	// hook is rebuilt at the next start, and a stale one baked into a stored
	// command line would outlive the binary that answers it.
	//
	// A failure to remember is not a failure to start. The session is running
	// and saying otherwise would be worse than the thing that went wrong; what
	// is lost is the offer to start it again after a reboot.
	if a.Remember != nil {
		if err := a.Remember.RememberSession(ctx, project, dir, cmd); err != nil {
			fmt.Fprintf(os.Stderr, "mustur: %s started but was not written down, so a reboot will lose it: %v\n", name, err)
		}
	}
	return Session{Name: name, Project: project}, nil
}

// Resume rewrites a command so it continues a conversation the CLI already has.
//
// Only a command this package recognises, for the reason withHook only appends
// to one: the flag belongs to a vendor, and adding it to something else
// produces a session that will not start.
//
// Any --resume or --continue already on the line is dropped first. A restored
// session's stored command carries the one from last time, and two --resume
// flags is a command that resumes the wrong thing or nothing at all.
// A command carrying a quote is left exactly as it is. This walks the line as
// whitespace-separated fields and rebuilds it, which is right for `claude` and
// `claude --resume <id>` — everything this actually stores — and would quietly
// re-word `--model "opus 5"`. A session that starts without its conversation is
// a disappointment; a session that starts with its arguments rearranged is a
// defect, so the disappointment is the one chosen.
func Resume(cmd, cli string) string {
	if !isClaude(cmd) || strings.ContainsAny(cmd, `"'`) {
		return cmd
	}
	fields := strings.Fields(cmd)
	out := fields[:0:0]
	for i := 0; i < len(fields); i++ {
		switch fields[i] {
		case "--resume", "-r":
			// The identifier is a separate argument, and skipping it is the
			// point: left behind it becomes a prompt the CLI is asked to answer.
			if i+1 < len(fields) && !strings.HasPrefix(fields[i+1], "-") {
				i++
			}
			continue
		case "--continue", "-c":
			continue
		}
		out = append(out, fields[i])
	}
	cmd = strings.Join(out, " ")
	if strings.TrimSpace(cli) == "" {
		return cmd
	}
	return cmd + " --resume " + cli
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
		"#{session_name}\t#{session_windows}\t#{session_attached}\t#{"+OwnedOption+"}\t#{window_activity}")
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
		// Activity is optional, and treated so on the assumption — not measured
		// — that a tmux which does not know the format leaves the column empty
		// rather than failing the whole listing. What is certain is the
		// handling: a session with no timestamp sorts last rather than becoming
		// the default, so the failure mode is a stale-looking order and never a
		// message sent somewhere unintended.
		//
		// In list-sessions, window_activity resolves against the session's
		// active window. Every session Mustur starts has exactly one.
		if len(parts) > 4 {
			var epoch int64
			if _, err := fmt.Sscanf(strings.TrimSpace(parts[4]), "%d", &epoch); err == nil && epoch > 0 {
				s.Activity = time.Unix(epoch, 0)
			}
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// ByActivity orders sessions most recently active first, and is what "the last
// active session" means anywhere it is offered as a default.
//
// Ties and sessions tmux gave no timestamp for fall back to name order, so two
// runs over the same sessions produce the same list — a default that moved
// between page loads for no reason would be worse than an alphabetical one.
func ByActivity(sessions []Session) []Session {
	out := append([]Session(nil), sessions...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Activity.Equal(out[j].Activity) {
			return out[i].Project < out[j].Project
		}
		return out[i].Activity.After(out[j].Activity)
	})
	return out
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
// Single-line text is sent with `-l`, literally, so a body containing something
// tmux would otherwise read as a key name arrives as characters. Multi-line
// text does not go through `-l` at all — see the paste branch below, and
// MUS-D-0096 for why.
// Agent is what the pane says the CLI is doing.
type Agent string

const (
	// AgentUnknown means the pane could not be read, or shows something this
	// does not recognise. It is not "idle": a surface that treated it as idle
	// would be asserting something about every CLI nobody has looked at.
	AgentUnknown Agent = ""
	// AgentWorking means a turn is in flight.
	AgentWorking Agent = "working"
	// AgentWaiting means the CLI is sitting at its prompt.
	AgentWaiting Agent = "waiting"
)

// workingMark is what Claude Code puts in its status line for exactly as long
// as a turn is running, and takes away the moment it ends.
//
// The owner pointed at this: the CLI already says whether it is working, and a
// timer counting silence is a guess standing in for a fact. A tool call that
// prints nothing for two minutes is working; a session that finished four
// seconds ago is not, and no amount of counting bytes can tell those apart.
const workingMark = "esc to interrupt"

// waitingMark is the input caret the same CLI draws when it wants a person.
// Present while working too, which is why the two are checked in order.
const waitingMark = "❯"

// paneLines is how much of the bottom of the pane to read. The status line and
// the input box are the last few rows; everything above is output.
const paneLines = "-12"

// Fit gives a session the geometry a browser can read, and is safe to call on
// one that already has it.
//
// window-size manual is what makes the size ours rather than the last attached
// client's; without it tmux resizes the window back to 80x24 the moment nobody
// is attached, which is every moment for a session Mustur started.
func (a *Adapter) Fit(ctx context.Context, project string) error {
	name, err := NameFor(project)
	if err != nil {
		return err
	}
	if out, err := a.runner().Run(ctx, "tmux",
		"set-option", "-t", name, "window-size", "manual"); err != nil {
		return fmt.Errorf("tmux set-option window-size %s: %w: %s", name, err, strings.TrimSpace(out))
	}
	if out, err := a.runner().Run(ctx, "tmux", "resize-window", "-t", name,
		"-x", strconv.Itoa(PaneWidth), "-y", strconv.Itoa(PaneHeight)); err != nil {
		return fmt.Errorf("tmux resize-window %s: %w: %s", name, err, strings.TrimSpace(out))
	}
	return nil
}

// Capture reads the pane as tmux has already assembled it.
//
// -e keeps the colour, which is the only escape worth carrying: what
// capture-pane hands back has no cursor movement in it at all, because tmux
// has already applied it. -J unwraps lines the pane wrapped, so a long line
// rewraps to the reader's own width rather than the pane's.
func (a *Adapter) Capture(ctx context.Context, project, from string) (string, error) {
	name, err := NameFor(project)
	if err != nil {
		return "", err
	}
	out, err := a.runner().Run(ctx, "tmux",
		"capture-pane", "-p", "-e", "-J", "-S", from, "-t", name)
	if err != nil {
		return "", fmt.Errorf("tmux capture-pane %s: %w: %s", name, err, strings.TrimSpace(out))
	}
	return out, nil
}

// DoingIn reads what the agent is up to out of a pane that has already been
// captured, so a caller holding one does not fetch it twice.
func DoingIn(pane string) Agent {
	// Working first: the input box is drawn during a turn as well, so looking
	// for the caret first would call every working session idle.
	if strings.Contains(pane, workingMark) {
		return AgentWorking
	}
	if strings.Contains(pane, waitingMark) {
		return AgentWaiting
	}
	return AgentUnknown
}

// Doing reads what the agent in this session is up to, from the pane itself.
//
// One CLI's strings, and it says so. Anything else falls through to
// AgentUnknown and the surface goes back to counting silence — which is worse,
// and is what every session had before this. Degrading to the old guess is the
// right failure; asserting "idle" about a CLI nobody has read would not be.
func (a *Adapter) Doing(ctx context.Context, project string) Agent {
	name, err := NameFor(project)
	if err != nil {
		return AgentUnknown
	}
	out, err := a.runner().Run(ctx, "tmux",
		"capture-pane", "-p", "-J", "-S", paneLines, "-t", name)
	if err != nil {
		return AgentUnknown
	}
	return DoingIn(out)
}

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
	if strings.Contains(text, "\n") {
		// Multi-line goes in as a paste, not as keystrokes.
		//
		// A newline typed into a terminal is Enter, and Enter in an agent's
		// composer submits.
		//
		// **Measured**, against Claude Code: `send-keys -l` with embedded
		// newlines lands every line in the composer, and a bracketed paste
		// does too and arrives as one message. Both work.
		//
		// **Asserted**, and the reason the paste is used anyway: that the TUI
		// reads one burst as a paste, that this is the CLI inferring intent
		// from timing, and that a write arriving split would therefore submit
		// halfway through. No split write has been observed. A bracketed paste
		// states that it is text instead of leaving it to be inferred, which is
		// an argument from reliability rather than from a failure to work.
		//
		// Verified end to end against the real CLI: four lines pasted, one
		// Enter, and the agent answered from all four
		// (records/work-units/MUS-W-0019.md).
		//
		// set-buffer rather than load-buffer, because the buffer content
		// arrives as an argument and this package's runner does not carry
		// stdin — and because a draft written to a temp file to be read back is
		// the owner's prose sitting on disk for no reason.
		// A name per send, and per process. Two sends to the same session used
		// to share one buffer: the second set-buffer overwrote the first, the
		// first paste delivered the wrong text and deleted the buffer, and the
		// second found none. The first fix counted within one Adapter, which
		// left the case its own comment named — the server and `mustur answer`
		// are different processes, each starting at one — so the pid is in the
		// name too.
		buf := fmt.Sprintf("mustur-%s-%d-%d", project, os.Getpid(), a.nextBuffer())
		if out, err := a.runner().Run(ctx, "tmux", "set-buffer", "-b", buf, "--", text); err != nil {
			return fmt.Errorf("tmux set-buffer: %w: %s", err, strings.TrimSpace(out))
		}
		// The owner's prose is in a tmux buffer from here until it is dropped,
		// where anything running as this user can read it — so it is dropped on
		// the way out whatever happens. -d does it on a successful paste; this
		// covers the paste failing, which used to leave the text sitting there.
		defer func() { _, _ = a.runner().Run(ctx, "tmux", "delete-buffer", "-b", buf) }()
		// -p brackets it, so the receiving program is told it is a paste rather
		// than left to infer it from timing.
		if out, err := a.runner().Run(ctx, "tmux", "paste-buffer", "-b", buf, "-t", name, "-p", "-d"); err != nil {
			return fmt.Errorf("tmux paste-buffer: %w: %s", err, strings.TrimSpace(out))
		}
	} else if out, err := a.runner().Run(ctx, "tmux", "send-keys", "-t", name, "-l", text); err != nil {
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
	// A session the owner ended is finished, so it stops being offered back.
	// That is the whole distinction the remembered table encodes: this is how a
	// stop is told apart from a reboot.
	if a.Remember != nil {
		if err := a.Remember.ForgetSession(ctx, project); err != nil {
			fmt.Fprintf(os.Stderr, "mustur: %s was stopped but is still written down: %v\n", name, err)
		}
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

// Keys the session surface may send, and the tmux name for each.
//
// An allowlist rather than a pass-through, and the reason is the runner: this
// package shells out to tmux with the caller's string as an argument, and
// send-keys reads names like `C-c` from that argument. A key the browser could
// name freely would be a browser choosing what tmux does to a pane.
//
// The set is what MUS-Q-0072 chose and no more -- Escape, Enter, the four
// arrows and Ctrl-C: get off a dialog, answer one, move within one, and
// interrupt a turn. There is deliberately no way to send
// an arbitrary control character; the next one that is wanted is a line in this
// map and a decision about what it is for.
var keys = map[string]string{
	"escape": "Escape",
	"enter":  "Enter",
	"up":     "Up",
	"down":   "Down",
	"left":   "Left",
	"right":  "Right",
	"cancel": "C-c",
}

// KeyNames is every key SendKey accepts, for a caller that wants to render
// them. Sorted, so a surface built from it does not reorder between runs.
func KeyNames() []string {
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// SendKey presses one key in a session Mustur started.
//
// Separate from Send rather than a mode of it. Send's whole argument is that a
// message is text and goes in as a paste that says so (MUS-D-0096); this is the
// case that decision did not cover — a pane asking for a keypress rather than a
// sentence — and MUS-Q-0072 is the named exception. Keeping them apart is what
// stops "send this text" quietly growing a way to press Ctrl-C.
//
// No Enter follows. That is the difference: Send types a line and submits it,
// and this presses exactly what it was asked for and nothing else.
func (a *Adapter) SendKey(ctx context.Context, project, key string) error {
	name, err := NameFor(project)
	if err != nil {
		return err
	}
	tmuxKey, ok := keys[strings.ToLower(strings.TrimSpace(key))]
	if !ok {
		return fmt.Errorf("%q is not a key this may send", key)
	}
	live, err := a.Alive(ctx, project)
	if err != nil {
		return err
	}
	if !live {
		return fmt.Errorf("%s has no session Mustur started", project)
	}
	if out, err := a.runner().Run(ctx, "tmux", "send-keys", "-t", name, tmuxKey); err != nil {
		return fmt.Errorf("tmux send-keys %s: %w: %s", tmuxKey, err, strings.TrimSpace(out))
	}
	return nil
}
