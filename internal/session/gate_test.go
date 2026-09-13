package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The payloads under test are the ones investigation 0003 captured from a real
// CLI, not payloads written to agree with this package. MUS-D-0114 is the
// finding that made that a rule: a double that agrees with the bug is worse
// than no double.
func payload(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return b
}

func TestOnlyANamedToolInAPromptingModeIsHeld(t *testing.T) {
	for _, tc := range []struct {
		name string
		gate []string
		mode string
		tool string
		want bool
		why  string
	}{
		{"the default set in the default mode", GateDefault, "default", "Bash", true, ""},
		{"case is not the point", GateDefault, "default", "bash", true, ""},
		{"a tool nobody named", GateDefault, "default", "Read", false, "Read is not in the set"},
		{"a mode the CLI never prompts in", GateDefault, "auto", "Bash", false, "MUS-D-0153: no gate Mustur cannot justify"},
		{"accepting edits is not prompting", GateDefault, "acceptEdits", "Edit", false, ""},
		{"bypassing is not prompting", GateDefault, "bypassPermissions", "Bash", false, ""},
		{"a mode this package has never seen", GateDefault, "somethingNew", "Bash", false, "unknown modes are left to the CLI"},
		{"a session that gates nothing", []string{}, "default", "Bash", false, ""},
		{"no tool at all", GateDefault, "default", "", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Gated(tc.gate, tc.mode, tc.tool); got != tc.want {
				t.Errorf("Gated(%v, %q, %q) = %v, want %v. %s", tc.gate, tc.mode, tc.tool, got, tc.want, tc.why)
			}
		})
	}
}

func TestTheCapturedPayloadsAreReadTheWayTheGateNeeds(t *testing.T) {
	ask, ok := AskFromPayload(payload(t, "gate-pretooluse.json"), time.Now())
	if !ok {
		t.Fatal("the Bash payload investigation 0003 captured is not readable as an ask")
	}
	if ask.Tool != "Bash" || ask.Mode != "default" {
		t.Errorf("tool=%q mode=%q, want Bash and default", ask.Tool, ask.Mode)
	}
	if ask.Summary != "touch ran-the-tool" {
		t.Errorf("summary=%q, want the command itself", ask.Summary)
	}
	if !Gated(GateDefault, ask.Mode, ask.Tool) {
		t.Error("the call the CLI drew a dialog for is not held by the default gate")
	}

	// The quiet case: the same event, in the same mode, for a call the CLI
	// allows without asking. MUS-F-0121 is why this is a test rather than an
	// assumption — PreToolUse fires either way and says nothing about which.
	quiet, ok := AskFromPayload(payload(t, "gate-pretooluse-quiet.json"), time.Now())
	if !ok {
		t.Fatal("the Read payload is not readable as an ask")
	}
	if Gated(GateDefault, quiet.Mode, quiet.Tool) {
		t.Errorf("a Read is held by the default gate; PreToolUse fires for it and nobody would have been asked")
	}
}

func TestWhatIsNotAnAsk(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name string
		body string
	}{
		{"a tool call inside a sub-agent", `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_use_id":"t1","agent_id":"a1","permission_mode":"default"}`},
		{"the event after the tool ran", `{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_use_id":"t1","permission_mode":"default"}`},
		{"a call with no identifier of its own", `{"hook_event_name":"PreToolUse","tool_name":"Bash","permission_mode":"default"}`},
		{"something that is not JSON", `not json`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := AskFromPayload([]byte(tc.body), now); ok {
				t.Error("held a call this package has no business holding")
			}
		})
	}
}

func TestAPressReachesTheHeldCall(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	if err := RaiseAsk(dir, "P", Ask{ID: "t1", Tool: "Bash", Summary: "rm -rf /", At: now}); err != nil {
		t.Fatalf("raise: %v", err)
	}

	waiting, err := Asks(dir, "P", now)
	if err != nil || len(waiting) != 1 || waiting[0].Summary != "rm -rf /" {
		t.Fatalf("Asks = %v, %v; want the one call that is waiting", waiting, err)
	}

	if err := AnswerAsk(dir, "P", "t1", Answer{Decision: "allow", At: now}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, ok := AwaitAnswer(ctx, dir, "P", "t1")
	if !ok || got.Decision != "allow" {
		t.Fatalf("AwaitAnswer = %+v, %v; want the press", got, ok)
	}

	DropAsk(dir, "P", "t1")
	if waiting, _ := Asks(dir, "P", now); len(waiting) != 0 {
		t.Errorf("a finished call is still being offered: %v", waiting)
	}
}

// The property that makes the whole thing safe to ship: nobody presses, and the
// hook returns nothing at all rather than a decision of its own. What happens
// next is the CLI's, and investigation 0003 measured it drawing its own dialog.
func TestNobodyPressesAndNothingIsDecided(t *testing.T) {
	dir := t.TempDir()
	if err := RaiseAsk(dir, "P", Ask{ID: "t1", Tool: "Bash", At: time.Now()}); err != nil {
		t.Fatalf("raise: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if got, ok := AwaitAnswer(ctx, dir, "P", "t1"); ok {
		t.Fatalf("AwaitAnswer invented %+v when nobody pressed", got)
	}
}

func TestAnAnswerNobodyIsWaitingForIsRefused(t *testing.T) {
	dir := t.TempDir()
	if err := AnswerAsk(dir, "P", "gone", Answer{Decision: "allow"}); err == nil {
		t.Error("answered a call nothing is holding")
	}
	if err := RaiseAsk(dir, "P", Ask{ID: "t1", Tool: "Bash", At: time.Now()}); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if err := AnswerAsk(dir, "P", "t1", Answer{Decision: "maybe"}); err == nil {
		t.Error("a decision that is neither allow nor deny was accepted")
	}
}

// A hook the CLI killed at its timeout leaves its file behind, and a button for
// it would answer a process that is gone — the dialog is on the pane by then.
func TestACallOlderThanTheTimeoutIsGoneRatherThanStale(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	if err := RaiseAsk(dir, "P", Ask{ID: "old", Tool: "Bash", At: now.Add(-GateTimeout - time.Minute)}); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if err := RaiseAsk(dir, "P", Ask{ID: "new", Tool: "Bash", At: now}); err != nil {
		t.Fatalf("raise: %v", err)
	}
	waiting, err := Asks(dir, "P", now)
	if err != nil {
		t.Fatalf("Asks: %v", err)
	}
	if len(waiting) != 1 || waiting[0].ID != "new" {
		t.Fatalf("Asks = %v; want only the call still being held", waiting)
	}
	if _, err := os.Stat(filepath.Join(AskDir(dir, "P"), "old.json")); !os.IsNotExist(err) {
		t.Error("the expired call is still on disk")
	}
}

// The shape the CLI reads back, key for key, as investigation 0003 returned it
// and watched the tool run.
func TestTheDecisionIsTheShapeTheCLIHonours(t *testing.T) {
	var got struct {
		Out struct {
			Event  string `json:"hookEventName"`
			Dec    string `json:"permissionDecision"`
			Reason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(Decision(Answer{Decision: "deny", Reason: "not this one"})), &got); err != nil {
		t.Fatalf("the decision is not JSON: %v", err)
	}
	if got.Out.Event != "PreToolUse" || got.Out.Dec != "deny" || got.Out.Reason != "not this one" {
		t.Errorf("decision = %+v; want the event, the decision and the reason the agent is told", got.Out)
	}
}

// An identifier arrives from outside this process. A file is named after it.
func TestAnIdentifierCannotNameAFileElsewhere(t *testing.T) {
	dir := t.TempDir()
	ask, ok := AskFromPayload([]byte(`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_use_id":"../../escaped","permission_mode":"default"}`), time.Now())
	if !ok {
		t.Fatal("payload not read")
	}
	if strings.Contains(ask.ID, "/") || strings.Contains(ask.ID, "..") {
		t.Fatalf("id = %q, which names a path", ask.ID)
	}
	if err := RaiseAsk(dir, "P", ask); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if _, err := os.Stat(filepath.Join(AskDir(dir, "P"), ask.ID+".json")); err != nil {
		t.Errorf("the file is not where the ask says it is: %v", err)
	}
}

func TestTheGateRidesInOnTheCommandLine(t *testing.T) {
	settings, err := HookSettings("/usr/bin/mustur", "/state", "P", "", []string{"Bash", "Edit"})
	if err != nil {
		t.Fatalf("HookSettings: %v", err)
	}
	if !strings.Contains(settings, "--gate 'Bash,Edit'") {
		t.Errorf("the gate is not on the hook's command line: %s", settings)
	}
	if !strings.Contains(settings, `"timeout":300`) {
		t.Errorf("the held call has no timeout of its own, so it waits at the CLI's default: %s", settings)
	}

	// A session that gates nothing still gets its sub-agent rows.
	settings, err = HookSettings("/usr/bin/mustur", "/state", "P", "", nil)
	if err != nil {
		t.Fatalf("HookSettings: %v", err)
	}
	if strings.Contains(settings, "--gate") {
		t.Errorf("a session with no gate carries one anyway: %s", settings)
	}
	if !strings.Contains(settings, "SubagentStart") {
		t.Errorf("turning the gate off turned the rows off with it: %s", settings)
	}
}

// A press that arrives after the CLI gave up on the hook.
//
// The file is still there — the hook was killed, so nothing removed it — and
// statting it was the whole of the check that shipped for one commit. The
// dialog is on the pane by then and this is not the way to answer it.
func TestAPressAfterTheTimeoutIsRefused(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	if err := RaiseAsk(dir, "P", Ask{ID: "t1", Tool: "Bash", At: now.Add(-GateTimeout - time.Minute)}); err != nil {
		t.Fatalf("raise: %v", err)
	}
	err := AnswerAsk(dir, "P", "t1", Answer{Decision: "allow", At: now})
	if err == nil {
		t.Fatal("a call that waited out its timeout took an answer anyway")
	}
	if !strings.Contains(err.Error(), "waited out its timeout") {
		t.Errorf("the refusal does not say what happened: %v", err)
	}
	if _, err := os.Stat(filepath.Join(AskDir(dir, "P"), "t1.answer")); err == nil {
		t.Error("an answer was written for a call nobody is holding")
	}
}

// Both directions carry a reason, because only one allow object was ever
// measured and it had one.
func TestAnAllowIsTheShapeThatWasMeasured(t *testing.T) {
	var got struct {
		Out struct {
			Dec    string `json:"permissionDecision"`
			Reason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(Decision(Answer{Decision: "allow"})), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if got.Out.Dec != "allow" || got.Out.Reason == "" {
		t.Errorf("allow = %+v; want the shape investigation 0003 validated, which carried a reason", got.Out)
	}
}
