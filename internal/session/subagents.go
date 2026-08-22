package session

// Sub-agents, and the one thing that makes them visible at all.
//
// A sub-agent is a tool call inside the CLI's own process. It is not a process
// the adapter starts, so there is no tmux window to hold it and nothing for
// `list-windows` to enumerate — which is why milestone 4c began by asking
// whether it could be done rather than by building it. The answer, and the four
// routes that were tried and rejected, are in
// docs/investigations/0002-sub-agent-visibility.md.
//
// The route is the CLI's lifecycle hooks. `SubagentStart` and `SubagentStop`
// carry an `agent_id`; a tool-use hook carries the same `agent_id` when the
// call happens inside a sub-agent and omits it in the main conversation. So a
// sub-agent's activity is attributed by an identifier the CLI supplies, never
// by reading the pane and guessing — which is what the investigation ruled out
// in advance, and what the owner declined an inferred status for on MUS-Q-0005.
//
// Everything in milestones 4a and 4b survives this: the session is still a tmux
// pane, still typed into with send-keys, still read with pipe-pane, still
// attachable from a terminal. A structured-output mode exists that would give
// more, and it costs the pane; MUS-D-0087 records why it was not taken.
//
// **Mustur installs the hook per session and persists nothing** (MUS-Q-0024).
// The hook rides in on a `--settings` JSON string appended to the command line
// Start already builds. Nothing is written to the owner's configuration and
// nothing into the checkout the session runs in. The cost, accepted rather than
// discovered: a session started by hand carries no hook and shows no
// sub-agents.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// TailBytes bounds how much of a session's sub-agent log is read back. A row
// whose start scrolled out of it does not appear at all, which is the honest
// failure — a half-read row would say a sub-agent began when it did not.
const TailBytes = 256 << 10

// SaidMax bounds the final message kept for one sub-agent. Long enough for a
// reviewer's verdict, finite so one runaway reply cannot make the log the
// largest thing on the machine.
const SaidMax = 8 << 10

// A Subagent is one row on the surface: what it was asked to do, how long it
// has been at it, what it is doing now, and what it said when it finished.
type Subagent struct {
	ID      string
	Type    string
	Task    string // empty when the launching call could not be identified
	Started time.Time
	Ended   time.Time // zero while it is still running
	Doing   string    // the tool it last reached for, while running
	Said    string    // its final message, once it has ended
}

// Running reports whether this sub-agent has yet to stop.
func (s Subagent) Running() bool { return s.Ended.IsZero() }

// For is how long it ran, or has been running.
func (s Subagent) For(now time.Time) time.Duration {
	if s.Ended.IsZero() {
		return now.Sub(s.Started)
	}
	return s.Ended.Sub(s.Started)
}

// event is the projection of a hook payload that this package keeps. The raw
// payload carries the sub-agent's whole prompt and the session's transcript
// path; neither is needed to draw a row, and a log that holds every prompt
// verbatim is a bigger thing to leave lying in a temp directory than a log that
// does not.
type event struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"`
	ID   string    `json:"id,omitempty"`
	Type string    `json:"type,omitempty"`
	Tool string    `json:"tool,omitempty"`
	Task string    `json:"task,omitempty"`
	Said string    `json:"said,omitempty"`
}

// DefaultHookDir is where sub-agent events are logged when nothing says
// otherwise.
//
// Deliberately not the temp directory the readers use. The unit runs with
// PrivateTmp off, so /tmp is shared with every other user on the machine, and a
// predictable path there is one a stranger can create first and point somewhere
// else. A directory under the owner's own state is not.
func DefaultHookDir() string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "mustur", "sessions")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "mustur", "sessions")
}

// SubagentLog is where one project's events are appended.
func SubagentLog(dir, project string) string {
	return filepath.Join(dir, "subagents", project+".jsonl")
}

// Project reads a hook payload from the CLI and appends the part worth keeping.
//
// It is deliberately total: a hook that fails is a hook that interferes with
// the agent it was watching, and nothing about drawing a row is worth that. A
// malformed payload, an unwritable directory and a disk that is full all end
// the same way, with nothing recorded and no error raised.
func RecordHookEvent(dir, project string, payload []byte, now time.Time) {
	var p struct {
		Event     string `json:"hook_event_name"`
		AgentID   string `json:"agent_id"`
		AgentType string `json:"agent_type"`
		Tool      string `json:"tool_name"`
		Input     struct {
			Description  string `json:"description"`
			SubagentType string `json:"subagent_type"`
		} `json:"tool_input"`
		Said string `json:"last_assistant_message"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return
	}

	var e event
	switch {
	case p.Event == "SubagentStart":
		e = event{Kind: "start", ID: p.AgentID, Type: p.AgentType}
	case p.Event == "SubagentStop":
		e = event{Kind: "stop", ID: p.AgentID, Said: clip(p.Said, SaidMax)}
	case p.AgentID != "":
		// A tool call inside a sub-agent. This is the only signal for what one
		// is doing while it runs.
		e = event{Kind: "doing", ID: p.AgentID, Tool: p.Tool}
	case p.Tool == "Agent":
		// The parent launching one. The description is the only place a
		// sub-agent's task is stated in a documented field.
		e = event{Kind: "launch", Type: p.Input.SubagentType, Task: clip(p.Input.Description, 200)}
	default:
		return // Any other tool call in the main conversation. Not ours.
	}
	e.At = now

	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	path := SubagentLog(dir, project)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	// One write, one line. Hooks run as separate processes and several fire at
	// once when several sub-agents start together, so an append that took two
	// writes would interleave two agents into one unreadable line.
	_, _ = f.Write(append(line, '\n'))
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Subagents folds a project's log into rows, oldest first.
func Subagents(dir, project string) ([]Subagent, error) {
	data, err := tail(SubagentLog(dir, project), TailBytes)
	if err != nil {
		return nil, err
	}

	rows := map[string]*Subagent{}
	var order []string
	// Launches waiting to be claimed by a start, oldest first, keyed by the
	// type the parent asked for. See the note on pairing below.
	pending := map[string][]string{}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e event
		if json.Unmarshal([]byte(line), &e) != nil {
			continue // A half-line at the head of a truncated read.
		}
		switch e.Kind {
		case "launch":
			pending[e.Type] = append(pending[e.Type], e.Task)
		case "start":
			if _, seen := rows[e.ID]; seen {
				continue
			}
			r := &Subagent{ID: e.ID, Type: e.Type, Started: e.At}
			// Pairing a task to an identifier.
			//
			// No documented field connects the parent's launching call to the
			// sub-agent it produced: the call carries a description and a
			// tool-use id, the start carries an agent id and a type, and they
			// share nothing but the type. The one place both appear together is
			// an undocumented file the owner declined to read (MUS-Q-0025).
			//
			// So this pairs by order within a type, and the basis is measured
			// rather than assumed: three sub-agents of one type launched
			// together, over three runs, with each one's own tool call as
			// independent ground truth for which was which — six agents scored,
			// six paired correctly, none wrongly. Small evidence for a label
			// whose failure mode is saying a sub-agent is doing something it is
			// not, which is why an unclaimed start shows no task at all rather
			// than borrowing the nearest one.
			if q := pending[e.Type]; len(q) > 0 {
				r.Task, pending[e.Type] = q[0], q[1:]
			}
			rows[e.ID] = r
			order = append(order, e.ID)
		case "doing":
			if r := rows[e.ID]; r != nil && r.Ended.IsZero() {
				r.Doing = e.Tool
			}
		case "stop":
			// Only a sub-agent that started gets a row. A run against the real
			// CLI produced stops for work of its own that this hook never saw
			// start, carrying text that was never in the session; a fold that
			// made a row from a stop would have shown those as sub-agents.
			if r := rows[e.ID]; r != nil {
				r.Ended, r.Said, r.Doing = e.At, e.Said, ""
			}
		}
	}

	out := make([]Subagent, 0, len(order))
	for _, id := range order {
		out = append(out, *rows[id])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Started.Before(out[j].Started) })
	return out, nil
}

// tail reads at most the last n bytes of a file. A missing file is no events
// rather than an error: a session that has launched nothing has no log.
func tail(path string, n int64) ([]byte, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() <= n {
		buf := make([]byte, info.Size())
		_, err := f.ReadAt(buf, 0)
		if err != nil && err.Error() != "EOF" {
			return nil, err
		}
		return buf, nil
	}
	buf := make([]byte, n)
	if _, err := f.ReadAt(buf, info.Size()-n); err != nil && err.Error() != "EOF" {
		return nil, err
	}
	return buf, nil
}

// HookSettings is the `--settings` value that makes a session's sub-agents
// visible: three hooks, all of them calling this same binary back.
//
// PreToolUse is registered against every tool because the field that identifies
// a sub-agent's own tool call is on the payload rather than in the tool's name,
// so there is nothing narrower to match on. The cost is one short-lived process
// per tool call in the session.
func HookSettings(exe, dir, project string) (string, error) {
	call := fmt.Sprintf("%s session subagent-event --dir %s --project %s",
		shellQuote(exe), shellQuote(dir), shellQuote(project))
	// One command for all three: every payload names its own event, so the hook
	// does not need telling which one it is.
	hooks := []any{map[string]any{"type": "command", "command": call}}
	settings := map[string]any{
		"hooks": map[string]any{
			"SubagentStart": []any{map[string]any{"hooks": hooks}},
			"SubagentStop":  []any{map[string]any{"hooks": hooks}},
			"PreToolUse":    []any{map[string]any{"matcher": "*", "hooks": hooks}},
		},
	}
	b, err := json.Marshal(settings)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// withHook appends the hook to a command line, and only to a command line it
// recognises.
//
// The adapter has no default CLI — it runs whatever the owner configured — and
// the hook interface belongs to one of them. Appending Claude Code's flags to
// something else would produce a session that fails to start, so a command this
// package does not recognise is left exactly as it was given and simply shows
// no sub-agents. Guessing wider would trade a working session for a row.
func withHook(cmd, exe, dir, project string) string {
	if !isClaude(cmd) || exe == "" || dir == "" {
		return cmd
	}
	settings, err := HookSettings(exe, dir, project)
	if err != nil {
		return cmd
	}
	return cmd + " --settings " + shellQuote(settings)
}

// exe is the Mustur binary a hook should call back. A machine that cannot say
// where its own binary is gets no hook rather than a hook that calls something
// else.
func (a *Adapter) exe() string {
	if a.Exe != "" {
		return a.Exe
	}
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return path
}

func isClaude(cmd string) bool {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return false
	}
	return filepath.Base(fields[0]) == "claude"
}

// shellQuote wraps a value for the shell tmux hands the command to. Single
// quotes take everything literally, which is what a JSON blob full of braces
// and double quotes needs; the dance in the middle is the only way to get a
// single quote inside single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
