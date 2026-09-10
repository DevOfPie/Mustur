package session

// The gate: a tool call held in front of the owner instead of a dialog drawn on
// a screen nobody is looking at.
//
// Milestone 8, and it is smaller than it sounds because of what cannot be done.
// `PreToolUse` is the only hook whose decision the CLI honours (MUS-F-0120); it
// fires on every tool call rather than on the ones a person would be asked
// about (MUS-F-0121); and the CLI waits it out before running the permission
// flow at all, so nothing tells Mustur a dialog was coming while it can still
// answer (MUS-F-0122). Mustur therefore decides for itself which calls to hold,
// with no way to ask the CLI what it would have done.
//
// MUS-D-0153 is that rule and this file is it: hold when the tool is named and
// the session is in a mode where the CLI prompts at all, never return allow
// without a press, and let an unanswered hook time out into the dialog the CLI
// would have drawn. The fallback is today's behaviour, which is what makes this
// safe to ship rather than something that has to be right.
//
// The rendezvous is two files rather than a port. The hook is a process the CLI
// starts and this package keeps alive while a call is held; the surface is a
// long-lived one holding a socket; they share a directory under the owner's own
// state and nothing else. No credential is needed for a file, and a hook that
// had to authenticate to a server would fail in exactly the case the gate
// exists for.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GateDefault is the set of tools held in front of the owner when nothing says
// otherwise.
//
// Small on purpose. Every name here is a call that changes something outside
// the conversation, and every name left out is one the CLI would usually let
// through — a gate on Read would ask the owner about a file the agent was going
// to be allowed to read anyway, which is the cost MUS-D-0153 names and the
// reason the set is a flag rather than a constant.
var GateDefault = []string{"Bash", "Edit", "Write", "NotebookEdit"}

// GateTimeout bounds how long a held call waits. It is the CLI's own hook
// timeout, so what happens at the end of it is the CLI's decision and not
// Mustur's: it draws the dialog it would have drawn.
//
// Five minutes, and the owner chose it on MUS-Q-0096 (MUS-D-0155). It shipped
// for one commit as a number nobody had chosen, under a comment calling it a
// number the owner feels — which is the argument for asking rather than for
// writing that sentence.
//
// The question does not leave Mustur when this expires. The dialog the CLI
// draws is on the pane, the pane is parsed, and the same pop-up offers it as a
// keypress: the gate hands the question over rather than dropping it.
const GateTimeout = 300 * time.Second

// AskPoll is how often a held hook looks for its answer. A press should feel
// immediate and the process is doing nothing else, so this is short; it costs
// one stat per interval on a file in the owner's own state directory.
const AskPoll = 150 * time.Millisecond

// SummaryMax bounds what a row shows of a tool's input. The whole input is kept
// in the file the surface reads, so a long command is truncated on the way to a
// button and not on the way to disk.
const SummaryMax = 400

// promptingModes are the permission modes in which the CLI asks a person
// anything at all.
//
// A mode this does not know is left alone rather than gated. Mustur adding a
// gate it cannot justify is the failure MUS-D-0153 exists to avoid, and the
// modes that skip the permission system entirely — auto, acceptEdits,
// bypassPermissions — are exactly the ones where a held call would be a stall
// the owner never asked for.
var promptingModes = map[string]bool{"default": true, "plan": true, "manual": true}

// Ask is one tool call waiting on the owner.
type Ask struct {
	ID      string    `json:"id"`
	Tool    string    `json:"tool"`
	Summary string    `json:"summary"`
	Input   string    `json:"input"`
	Mode    string    `json:"mode"`
	At      time.Time `json:"at"`
}

// Answer is what the owner pressed. There is no third value: a call is allowed
// because somebody allowed it, denied because somebody denied it, and otherwise
// there is no answer at all and the hook times out.
type Answer struct {
	Decision string    `json:"decision"`
	Reason   string    `json:"reason,omitempty"`
	At       time.Time `json:"at"`
}

// Gated says whether a call is held.
//
// Both clauses or neither, which is MUS-D-0153's rule stated once so that
// nothing else in this package has to restate it.
func Gated(tools []string, mode, tool string) bool {
	if tool == "" || !promptingModes[mode] {
		return false
	}
	for _, t := range tools {
		if strings.EqualFold(strings.TrimSpace(t), tool) {
			return true
		}
	}
	return false
}

// ParseGate reads a comma-separated tool list. An empty string is no gate at
// all, which is how a session opts out of this entirely.
func ParseGate(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// AskDir is where one project's held calls live.
func AskDir(dir, project string) string {
	return filepath.Join(dir, "asks", project)
}

// ForgetAsks drops a project's held calls.
//
// Called when a session starts, for the reason ForgetSubagents is: a call held
// by a process that no longer exists is a button that answers nobody, and a
// surface offering one is worse than a surface offering none.
func ForgetAsks(dir, project string) {
	if dir == "" || project == "" {
		return
	}
	_ = os.RemoveAll(AskDir(dir, project))
}

// RaiseAsk records a call as waiting.
//
// Written to a temporary name and renamed, because the surface lists this
// directory on a ticker and a half-written file read at the wrong moment is a
// row with no tool in it.
func RaiseAsk(dir, project string, a Ask) error {
	if dir == "" || project == "" || a.ID == "" {
		return fmt.Errorf("gate: an ask needs a directory, a project and an id")
	}
	d := AskDir(dir, project)
	if err := os.MkdirAll(d, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	tmp := filepath.Join(d, "."+a.ID+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(d, a.ID+".json"))
}

// Asks lists what is waiting, oldest first.
//
// Anything older than the hook timeout is gone rather than stale: the process
// that raised it has been killed by the CLI, its dialog has been drawn on the
// pane, and a button here would answer a call nobody is holding. Expiry deletes
// the pair, so the directory does not grow across a long session.
func Asks(dir, project string, now time.Time) ([]Ask, error) {
	d := AskDir(dir, project)
	entries, err := os.ReadDir(d)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Ask
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasPrefix(name, ".") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(d, name))
		if err != nil {
			continue
		}
		var a Ask
		if err := json.Unmarshal(b, &a); err != nil || a.ID == "" {
			continue
		}
		if now.Sub(a.At) > GateTimeout {
			_ = os.Remove(filepath.Join(d, name))
			_ = os.Remove(answerPath(dir, project, a.ID))
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

// AskStamp is a cheap fingerprint of what is waiting, so a ticker can tell that
// nothing has changed without parsing anything. The same trick SubagentStamp
// plays, and for the same reason: the common case is that nothing happened.
func AskStamp(dir, project string) string {
	entries, err := os.ReadDir(AskDir(dir, project))
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if n := e.Name(); strings.HasSuffix(n, ".json") && !strings.HasPrefix(n, ".") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// AnswerAsk records what the owner pressed.
//
// It refuses an identifier nothing is waiting on. A button pressed twice, or
// pressed after the hook has timed out and the CLI has drawn its own dialog,
// gets a refusal the surface can say out loud rather than a file that sits
// there being nobody's answer.
func AnswerAsk(dir, project, id string, ans Answer) error {
	if ans.Decision != "allow" && ans.Decision != "deny" {
		return fmt.Errorf("gate: a decision is allow or deny, not %q", ans.Decision)
	}
	// Not just "is there a file". A hook the CLI killed at its timeout leaves
	// one behind, and the dialog is on the pane by then — so a press that
	// arrives late has to be refused rather than written into a file no process
	// will ever read. Statting alone shipped, briefly, and said a call had been
	// answered when nothing was listening.
	b, err := os.ReadFile(filepath.Join(AskDir(dir, project), id+".json"))
	if err != nil {
		return fmt.Errorf("gate: nothing is waiting on %s", id)
	}
	var held Ask
	if err := json.Unmarshal(b, &held); err != nil {
		return fmt.Errorf("gate: nothing is waiting on %s", id)
	}
	if ans.At.IsZero() {
		ans.At = time.Now()
	}
	if ans.At.Sub(held.At) > GateTimeout {
		return fmt.Errorf("gate: %s waited out its timeout, so the session is drawing its own dialog", id)
	}
	b, err = json.Marshal(ans)
	if err != nil {
		return err
	}
	d := AskDir(dir, project)
	tmp := filepath.Join(d, "."+id+".answer.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, answerPath(dir, project, id))
}

// AwaitAnswer blocks until the owner presses or the context ends.
//
// The context is what the CLI's own timeout becomes: when it fires, this
// returns nothing at all, the hook prints nothing, and the CLI does what it
// would have done unhooked. That is the whole of the fallback and it is why
// nothing here needs to be right.
func AwaitAnswer(ctx context.Context, dir, project, id string) (Answer, bool) {
	path := answerPath(dir, project, id)
	t := time.NewTicker(AskPoll)
	defer t.Stop()
	for {
		if b, err := os.ReadFile(path); err == nil {
			var a Answer
			if err := json.Unmarshal(b, &a); err == nil && (a.Decision == "allow" || a.Decision == "deny") {
				return a, true
			}
		}
		select {
		case <-ctx.Done():
			return Answer{}, false
		case <-t.C:
		}
	}
}

// DropAsk removes a held call and its answer. The hook calls it on the way out,
// so the ordinary case leaves nothing behind; expiry in Asks is for the case
// where the hook was killed rather than allowed to finish.
func DropAsk(dir, project, id string) {
	_ = os.Remove(filepath.Join(AskDir(dir, project), id+".json"))
	_ = os.Remove(answerPath(dir, project, id))
}

// Decision renders what the CLI reads back on stdout.
//
// The shape is the CLI's, measured rather than remembered: investigation 0003
// returned exactly this object and watched the tool run and the dialog not be
// drawn. A reason travels with a denial because the agent is told it verbatim,
// which is the difference between a refusal it can work around and one it
// cannot see.
func Decision(a Answer) string {
	// Both directions carry a reason. The only allow object investigation 0003
	// validated had a non-empty one, and shipping a variation on the single
	// shape that was measured is how a hook comes to be ignored in a way
	// nothing here would notice.
	if a.Reason == "" {
		a.Reason = "Allowed from Mustur's session view"
	}
	out := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       a.Decision,
			"permissionDecisionReason": a.Reason,
		},
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(b)
}

// AskFromPayload builds what the surface shows from what the CLI sent.
//
// The identifier is the CLI's own tool_use_id where there is one. Nothing here
// invents one when there is not: two calls sharing an identifier would be two
// buttons answering each other, and a call with no identifier is one this
// package declines to hold rather than one it guesses at.
func AskFromPayload(payload []byte, now time.Time) (Ask, bool) {
	var p struct {
		Event   string          `json:"hook_event_name"`
		Tool    string          `json:"tool_name"`
		Mode    string          `json:"permission_mode"`
		UseID   string          `json:"tool_use_id"`
		AgentID string          `json:"agent_id"`
		Input   json.RawMessage `json:"tool_input"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return Ask{}, false
	}
	if p.Event != "PreToolUse" || p.UseID == "" {
		return Ask{}, false
	}
	// A sub-agent's own tool call. The owner is answering for the session, and
	// a sub-agent is a call inside it — holding those too would put a button in
	// front of work the owner cannot see the shape of. Left to the CLI.
	if p.AgentID != "" {
		return Ask{}, false
	}
	return Ask{
		ID:      safeID(p.UseID),
		Tool:    p.Tool,
		Summary: summarise(p.Input),
		Input:   string(p.Input),
		Mode:    p.Mode,
		At:      now,
	}, true
}

// summarise is the one line a button shows. The command, where there is one,
// because that is what the owner is deciding about; otherwise the input as it
// came, clipped.
func summarise(input json.RawMessage) string {
	var fields struct {
		Command     string `json:"command"`
		FilePath    string `json:"file_path"`
		Description string `json:"description"`
		URL         string `json:"url"`
	}
	_ = json.Unmarshal(input, &fields)
	for _, s := range []string{fields.Command, fields.FilePath, fields.URL, fields.Description} {
		if t := strings.TrimSpace(s); t != "" {
			return clip(t, SummaryMax)
		}
	}
	return clip(strings.TrimSpace(string(input)), SummaryMax)
}

// safeID keeps an identifier that came from outside this process from naming a
// file outside the directory it belongs in.
func safeID(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return clip(b.String(), 128)
}

func answerPath(dir, project, id string) string {
	return filepath.Join(AskDir(dir, project), id+".answer")
}

// gate is the tool set a session starts with. Nil means the default; an empty
// non-nil slice means a session that gates nothing, and the difference is the
// whole reason this is not just a package variable.
func (a *Adapter) gate() []string {
	if a.Gate == nil {
		return GateDefault
	}
	return a.Gate
}
