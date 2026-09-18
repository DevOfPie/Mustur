package session

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func record(t *testing.T, dir, project string, at time.Time, payload map[string]any) {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	RecordHookEvent(dir, project, b, at)
}

// The whole of it: a sub-agent starts, does something, and finishes.
func TestASubagentIsSeenStartedWorkingAndFinished(t *testing.T) {
	dir := t.TempDir()
	t0 := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

	record(t, dir, "Mustur", t0, map[string]any{
		"hook_event_name": "PreToolUse", "tool_name": "Agent",
		"tool_input": map[string]any{"description": "Contract reviewer", "subagent_type": "general-purpose"},
	})
	record(t, dir, "Mustur", t0.Add(time.Second), map[string]any{
		"hook_event_name": "SubagentStart", "agent_id": "a1", "agent_type": "general-purpose",
	})
	record(t, dir, "Mustur", t0.Add(2*time.Second), map[string]any{
		"hook_event_name": "PreToolUse", "agent_id": "a1", "tool_name": "Grep",
	})

	rows, err := Subagents(dir, "Mustur")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("%d rows, want 1", len(rows))
	}
	if got := rows[0].Task; got != "Contract reviewer" {
		t.Errorf("task %q, want the description the parent launched it with", got)
	}
	if rows[0].Doing != "Grep" {
		t.Errorf("doing %q, want Grep", rows[0].Doing)
	}
	if !rows[0].Running() {
		t.Error("a sub-agent that has not stopped is not running")
	}

	record(t, dir, "Mustur", t0.Add(90*time.Second), map[string]any{
		"hook_event_name": "SubagentStop", "agent_id": "a1",
		"last_assistant_message": "Three findings.",
	})
	rows, err = Subagents(dir, "Mustur")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Running() {
		t.Error("still running after SubagentStop")
	}
	if rows[0].Said != "Three findings." {
		t.Errorf("said %q, want the final message", rows[0].Said)
	}
	if rows[0].Doing != "" {
		t.Errorf("a finished sub-agent still claims to be doing %q", rows[0].Doing)
	}
	if got := rows[0].For(t0.Add(5 * time.Minute)); got != 89*time.Second {
		t.Errorf("ran for %v, want the time between its own start and stop", got)
	}
}

// Three of a kind at once. The identifier is the CLI's; the task is not, and
// pairing them is the one place this package infers anything.
func TestThreeAtOnceKeepTheirOwnTasks(t *testing.T) {
	dir := t.TempDir()
	t0 := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	for i, task := range []string{"Done-when", "Shipped-claims", "Contract"} {
		record(t, dir, "P", t0.Add(time.Duration(i)*time.Millisecond), map[string]any{
			"hook_event_name": "PreToolUse", "tool_name": "Agent",
			"tool_input": map[string]any{"description": task, "subagent_type": "general-purpose"},
		})
		record(t, dir, "P", t0.Add(time.Duration(i)*time.Millisecond+time.Millisecond/2), map[string]any{
			"hook_event_name": "SubagentStart", "agent_id": "a" + task, "agent_type": "general-purpose",
		})
	}
	rows, err := Subagents(dir, "P")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("%d rows, want 3", len(rows))
	}
	for _, want := range []string{"Done-when", "Shipped-claims", "Contract"} {
		var found bool
		for _, r := range rows {
			if r.ID == "a"+want && r.Task == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s did not keep its own task: %+v", want, rows)
		}
	}
}

// The failure this package refuses to make. A start with no launch to pair does
// not borrow the nearest one, because a row labelled with another sub-agent's
// job reads as a fact.
func TestAnUnpairedSubagentShowsNoTaskRatherThanTheWrongOne(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	record(t, dir, "P", now, map[string]any{
		"hook_event_name": "PreToolUse", "tool_name": "Agent",
		"tool_input": map[string]any{"description": "Explore the tree", "subagent_type": "Explore"},
	})
	record(t, dir, "P", now.Add(time.Second), map[string]any{
		"hook_event_name": "SubagentStart", "agent_id": "a1", "agent_type": "general-purpose",
	})
	rows, err := Subagents(dir, "P")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("%d rows, want 1", len(rows))
	}
	if rows[0].Task != "" {
		t.Errorf("task %q; a general-purpose start took an Explore launch's description", rows[0].Task)
	}
}

// A session's own tool calls are not sub-agents, and a log that recorded them
// would be one line per tool call in the session for nothing.
func TestTheMainConversationIsNotRecorded(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	record(t, dir, "P", now, map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Bash"})
	record(t, dir, "P", now, map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Edit"})
	if _, err := os.Stat(SubagentLog(dir, "P")); !os.IsNotExist(err) {
		b, _ := os.ReadFile(SubagentLog(dir, "P"))
		t.Errorf("the main conversation was logged: %s", b)
	}
}

// A hook that fails is a hook interfering with the agent it was watching.
func TestTheHookSurvivesAnythingItIsGiven(t *testing.T) {
	dir := t.TempDir()
	for _, payload := range []string{"", "not json", "[]", `{"hook_event_name":123}`, "null"} {
		RecordHookEvent(dir, "P", []byte(payload), time.Now())
	}
	RecordHookEvent(filepath.Join(dir, "no", "such", "\x00"), "P", []byte(`{"hook_event_name":"SubagentStart"}`), time.Now())
	if _, err := Subagents(dir, "P"); err != nil {
		t.Fatalf("reading after garbage: %v", err)
	}
}

// Payloads the CLI actually emitted, captured from a run of three sub-agents.
// A parser tested only against payloads its author wrote is a parser tested
// against its author's beliefs.
func TestRealPayloadsFromTheCLI(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "hook-payloads.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	at := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		RecordHookEvent(dir, "P", []byte(line), at)
		at = at.Add(time.Second)
	}
	rows, err := Subagents(dir, "P")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("%d rows from the captured run, want 3", len(rows))
	}
	for _, r := range rows {
		if r.Running() {
			t.Errorf("%s still running; every sub-agent in the capture stopped", r.ID)
		}
		if r.Task == "" {
			t.Errorf("%s has no task, and every launch in the capture carried a description", r.ID)
		}
		if r.Said == "" {
			t.Errorf("%s said nothing, and every stop in the capture carried a final message", r.ID)
		}
	}
}

// The log holds a projection, not the payload. The prompt a sub-agent was given
// can be the largest thing in the session and none of it is needed to draw a
// row.
func TestThePromptIsNotKept(t *testing.T) {
	dir := t.TempDir()
	record(t, dir, "P", time.Now(), map[string]any{
		"hook_event_name": "PreToolUse", "tool_name": "Agent",
		"tool_input": map[string]any{
			"description": "Review it", "subagent_type": "general-purpose",
			"prompt": "SECRET-PROMPT-BODY",
		},
		"transcript_path": "/home/owner/.claude/projects/x/y.jsonl",
	})
	b, err := os.ReadFile(SubagentLog(dir, "P"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "SECRET-PROMPT-BODY") {
		t.Error("the sub-agent's prompt was written to the log")
	}
	if strings.Contains(string(b), "transcript") {
		t.Error("the session's transcript path was written to the log")
	}
}

func TestTheHookIsOnlyAddedToACommandItRecognises(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		want bool
	}{
		{"claude", true},
		{"claude --model opus", true},
		{"/usr/local/bin/claude", true},
		{"codex", false},
		{"my-agent --claude", false},
		{"", false},
	} {
		got := withHook(tc.cmd, "/usr/bin/mustur", "/state", "P", "", nil) != tc.cmd
		if got != tc.want {
			t.Errorf("withHook(%q) changed=%v, want %v", tc.cmd, got, tc.want)
		}
	}
}

// The settings blob is full of braces and double quotes and goes through the
// shell tmux hands the command to. Quoting it wrong is not a subtle bug — the
// session does not start — but it is a bug a fake runner would never show, so
// the round trip runs through a real shell.
func TestTheHookSurvivesTheShell(t *testing.T) {
	cmd := withHook("claude", "/usr/bin/mustur", "/state dir", "P", "", nil)
	if cmd == "claude" {
		t.Fatal("no hook was added")
	}
	// Replace the program with something that prints its arguments, keeping the
	// quoting exactly as Start would hand it over.
	printer := strings.Replace(cmd, "claude", "printf '%s\\n'", 1)
	out, err := exec.Command("sh", "-c", printer).Output()
	if err != nil {
		t.Fatalf("the shell rejected the command line: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 2 || lines[0] != "--settings" {
		t.Fatalf("the shell split the command line into %q", lines)
	}
	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct{ Command string } `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &settings); err != nil {
		t.Fatalf("the settings did not survive the shell as JSON: %v\n%s", err, lines[1])
	}
	for _, event := range []string{"SubagentStart", "SubagentStop", "PreToolUse"} {
		got := settings.Hooks[event]
		if len(got) != 1 || len(got[0].Hooks) != 1 {
			t.Fatalf("%s has %d hooks", event, len(got))
		}
		if !strings.Contains(got[0].Hooks[0].Command, "'/state dir'") {
			t.Errorf("%s lost the quoting on a directory with a space: %q", event, got[0].Hooks[0].Command)
		}
	}
}

// A sub-agent that started early in a long log still has a row.
//
// The fold used to read the last 256KB, and one day of Hoard_Work put 213 of
// 260 starts outside it, gone from the drawer with nothing saying so
// (MUS-F-0158). This one starts before a megabyte of other events.
func TestAnEarlySubagentSurvivesALongLog(t *testing.T) {
	dir := t.TempDir()
	path := SubagentLog(dir, "P")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString(`{"kind":"start","id":"early","type":"general-purpose","at":"2026-08-22T12:00:00Z"}` + "\n")
	b.WriteString(strings.Repeat(`{"kind":"noise","at":"2026-08-22T12:00:01Z","said":"`+strings.Repeat("x", 900)+`"}`+"\n", 1200))
	b.WriteString(`{"kind":"stop","id":"early","said":"done","at":"2026-08-22T13:00:00Z"}` + "\n")
	b.WriteString(`{"kind":"start","id":"late","type":"general-purpose","at":"2026-08-22T13:00:01Z"}` + "\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	rows, err := Subagents(dir, "P")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows %+v, want both the early one and the late one", rows)
	}
	if early := rowByID(rows, "early"); early.Running() || early.Said != "done" {
		t.Errorf("early row %+v, want it ended by the stop a megabyte later", early)
	}
}

// Between calls only what was appended is read, and what is read is folded
// onto what was already there — including a stop for a start read last time.
func TestTheFoldCarriesAcrossCalls(t *testing.T) {
	dir := t.TempDir()
	t0 := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	record(t, dir, "P", t0, map[string]any{
		"hook_event_name": "SubagentStart", "agent_id": "a1", "agent_type": "general-purpose",
	})
	if rows, _ := Subagents(dir, "P"); len(rows) != 1 || !rows[0].Running() {
		t.Fatalf("rows %+v, want one running", rows)
	}
	record(t, dir, "P", t0.Add(time.Minute), map[string]any{
		"hook_event_name": "SubagentStop", "agent_id": "a1", "last_assistant_message": "ok",
	})
	rows, err := Subagents(dir, "P")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Running() || rows[0].Said != "ok" {
		t.Fatalf("rows %+v, want the row read last call ended by the stop appended since", rows)
	}
}

// Half a line is not read as a line and then skipped: it is left until the
// rest of it arrives.
func TestAHalfWrittenLineWaitsForTheRest(t *testing.T) {
	dir := t.TempDir()
	path := SubagentLog(dir, "P")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	line := `{"kind":"start","id":"a1","type":"general-purpose","at":"2026-08-22T12:00:00Z"}` + "\n"
	if err := os.WriteFile(path, []byte(line[:30]), 0o600); err != nil {
		t.Fatal(err)
	}
	if rows, _ := Subagents(dir, "P"); len(rows) != 0 {
		t.Fatalf("rows %+v from half a line", rows)
	}
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	if rows, _ := Subagents(dir, "P"); len(rows) != 1 {
		t.Fatalf("rows %+v, want the line once it was whole", rows)
	}
}

// A log forgotten and written again is a new log, even when it is as long as
// the old one by the time it is next read.
func TestAForgottenLogIsFoldedAfresh(t *testing.T) {
	dir := t.TempDir()
	t0 := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	record(t, dir, "P", t0, map[string]any{
		"hook_event_name": "SubagentStart", "agent_id": "old", "agent_type": "general-purpose",
	})
	if rows, _ := Subagents(dir, "P"); len(rows) != 1 {
		t.Fatalf("%d rows before forgetting, want 1", len(rows))
	}
	ForgetSubagents(dir, "P")
	for i, id := range []string{"new1", "new2"} {
		record(t, dir, "P", t0.Add(time.Duration(i+1)*time.Hour), map[string]any{
			"hook_event_name": "SubagentStart", "agent_id": id, "agent_type": "general-purpose",
		})
	}
	rows, err := Subagents(dir, "P")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rowByID(rows, "old").ID != "" {
		t.Fatalf("rows %+v, want only the new log's two", rows)
	}
}

// A stop for a sub-agent that never started makes no row.
//
// Not hypothetical: an end-to-end run against the real CLI produced two of
// them, carrying text that was never in the session — the CLI runs work of its
// own that reports a stop without a start this hook ever saw. A fold that
// created a row from a stop would have shown the owner two sub-agents that did
// not exist, alongside the two that did.
func TestAStopWithNoStartMakesNoRow(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	record(t, dir, "P", now, map[string]any{
		"hook_event_name": "SubagentStart", "agent_id": "real", "agent_type": "general-purpose",
	})
	record(t, dir, "P", now.Add(time.Second), map[string]any{
		"hook_event_name": "SubagentStop", "agent_id": "real", "last_assistant_message": "ALPHA",
	})
	record(t, dir, "P", now.Add(2*time.Second), map[string]any{
		"hook_event_name": "SubagentStop", "agent_id": "never-started",
		"last_assistant_message": "did both agents finish?",
	})

	rows, err := Subagents(dir, "P")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("%d rows, want only the sub-agent that started: %+v", len(rows), rows)
	}
	if rows[0].ID != "real" {
		t.Errorf("row is %q, want the one that started", rows[0].ID)
	}
}

// A sub-agent belongs to the session that spawned it.
//
// Before this, the log outlived the session: stop, start again, and the new
// session's page showed the old one's rows — one still pilled running, ageing
// forever, for a process dead before the page existed. That is the condition
// the investigation's own rule called disqualifying.
func TestStartingASessionForgetsTheLastOnesSubagents(t *testing.T) {
	realTmux(t)
	dir := t.TempDir()
	a := &Adapter{HookDir: dir}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

	record(t, dir, "zzForget", now, map[string]any{
		"hook_event_name": "SubagentStart", "agent_id": "old", "agent_type": "general-purpose",
	})
	if rows, _ := Subagents(dir, "zzForget"); len(rows) != 1 {
		t.Fatalf("%d rows before the restart, want 1", len(rows))
	}

	start(t, a, "zzForget", "sh -c 'sleep 30'")

	rows, err := Subagents(dir, "zzForget")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("the new session shows the old session's rows: %+v", rows)
	}
}

// A launch that never produced a sub-agent must not label the next one.
//
// A reviewer reproduced a row reading "DENIED call, never ran" — an Agent call
// denied permission left its description in the queue, and the next sub-agent
// of that type took it. The owner chose to bound the pairing (MUS-Q-0026).
func TestAStaleLaunchDoesNotLabelALaterSubagent(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

	record(t, dir, "P", now, map[string]any{
		"hook_event_name": "PreToolUse", "tool_name": "Agent",
		"tool_input": map[string]any{"description": "DENIED call, never ran", "subagent_type": "general-purpose"},
	})
	// Well past the window, and a real sub-agent starts.
	record(t, dir, "P", now.Add(LaunchWindow+time.Second), map[string]any{
		"hook_event_name": "SubagentStart", "agent_id": "a1", "agent_type": "general-purpose",
	})

	rows, err := Subagents(dir, "P")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("%d rows, want 1", len(rows))
	}
	if rows[0].Task != "" {
		t.Errorf("task %q — a stale launch labelled a later sub-agent", rows[0].Task)
	}
}

// And the bound must not cost a correct pairing. The slowest launch-to-start
// pair measured in docs/investigations/0002-harness/captured is 5.985s, so a
// window that expired inside that would strip the label off rows that are
// right — which is the same failure arriving by the other door.
func TestTheWindowIsWiderThanTheSlowestMeasuredSpawn(t *testing.T) {
	const slowestMeasured = 5985 * time.Millisecond
	if LaunchWindow <= slowestMeasured {
		t.Fatalf("LaunchWindow is %v, not wider than the %v actually measured", LaunchWindow, slowestMeasured)
	}

	dir := t.TempDir()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	record(t, dir, "P", now, map[string]any{
		"hook_event_name": "PreToolUse", "tool_name": "Agent",
		"tool_input": map[string]any{"description": "Slow to spawn", "subagent_type": "general-purpose"},
	})
	record(t, dir, "P", now.Add(slowestMeasured), map[string]any{
		"hook_event_name": "SubagentStart", "agent_id": "a1", "agent_type": "general-purpose",
	})
	rows, _ := Subagents(dir, "P")
	if len(rows) != 1 || rows[0].Task != "Slow to spawn" {
		t.Errorf("a pairing as slow as the slowest measured lost its task: %+v", rows)
	}
}

// A sub-agent between tool calls is not still in the last one.
//
// The first version hooked only the start of a tool call, so a row showed the
// last tool forever — including after the process died — while a comment on the
// surface claimed the opposite. PostToolUse is the other half.
func TestASubagentLeavesAToolWhenTheToolEnds(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	record(t, dir, "P", now, map[string]any{
		"hook_event_name": "SubagentStart", "agent_id": "a1", "agent_type": "general-purpose",
	})
	record(t, dir, "P", now.Add(time.Second), map[string]any{
		"hook_event_name": "PreToolUse", "agent_id": "a1", "tool_name": "Bash",
	})
	if rows, _ := Subagents(dir, "P"); rows[0].Doing != "Bash" {
		t.Fatalf("doing %q during the call, want Bash", rows[0].Doing)
	}

	record(t, dir, "P", now.Add(2*time.Second), map[string]any{
		"hook_event_name": "PostToolUse", "agent_id": "a1", "tool_name": "Bash",
	})
	rows, _ := Subagents(dir, "P")
	if rows[0].Doing != "" {
		t.Errorf("doing %q after the call ended, want nothing", rows[0].Doing)
	}
}

// The parent's PostToolUse for its own Agent call carries no agent_id either,
// so without checking the event name it landed as a second launch — every
// description in the queue twice. Verified against the real CLI before it
// shipped; kept here so it cannot come back.
func TestTheParentsAgentPostToolUseIsNotASecondLaunch(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	for _, ev := range []string{"PreToolUse", "PostToolUse"} {
		record(t, dir, "P", now, map[string]any{
			"hook_event_name": ev, "tool_name": "Agent",
			"tool_input": map[string]any{"description": "Only once", "subagent_type": "general-purpose"},
		})
	}
	record(t, dir, "P", now.Add(time.Second), map[string]any{
		"hook_event_name": "SubagentStart", "agent_id": "a1", "agent_type": "general-purpose",
	})
	record(t, dir, "P", now.Add(2*time.Second), map[string]any{
		"hook_event_name": "SubagentStart", "agent_id": "a2", "agent_type": "general-purpose",
	})

	rows, _ := Subagents(dir, "P")
	if len(rows) != 2 {
		t.Fatalf("%d rows, want 2", len(rows))
	}
	// Looked up by identifier: both are running, so the drawer's order puts the
	// second first.
	if r := rowByID(rows, "a1"); r.Task != "Only once" {
		t.Errorf("first row task %q", r.Task)
	}
	if r := rowByID(rows, "a2"); r.Task != "" {
		t.Errorf("second row took a duplicate of the first row's task: %q", r.Task)
	}
}

func rowByID(rows []Subagent, id string) Subagent {
	for _, r := range rows {
		if r.ID == id {
			return r
		}
	}
	return Subagent{}
}

// capturedLine is one payload from testdata/hook-payloads.jsonl, decoded so a
// test can graft a field onto it.
func capturedLine(t *testing.T, n int) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "hook-payloads.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var p map[string]any
	if err := json.Unmarshal([]byte(lines[n-1]), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// A sub-agent that launches one of its own still says what it asked for.
//
// The launching call inside a sub-agent carries agent_id, and the switch used to
// test for agent_id before it tested for a launch, so the call was logged as the
// parent reaching for the Agent tool and its description was dropped. The start
// it caused paired with nothing and the row read "general-purpose" — 257 of 260
// starts in one session's log (MUS-F-0156).
//
// No nested launch was captured. The payload here is composed from captured
// ones: line 1's PreToolUse for Agent, with the agent_id and agent_type that
// line 8 shows a sub-agent's own tool call carrying. The start is line 6's shape
// under a new identifier.
func TestANestedLaunchTitlesTheSubagentItStarts(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	rec := func(p map[string]any) {
		record(t, dir, "P", at, p)
		at = at.Add(time.Second)
	}
	parent := capturedLine(t, 4)["agent_id"].(string)

	rec(capturedLine(t, 1)) // the main conversation launches "Task ONE"
	rec(capturedLine(t, 4)) // and it starts

	nested := capturedLine(t, 1)
	nested["agent_id"], nested["agent_type"] = parent, "general-purpose"
	nested["tool_input"].(map[string]any)["description"] = "Nested task"
	rec(nested)

	child := capturedLine(t, 6)
	child["agent_id"] = "nested-child"
	rec(child)

	rows, err := Subagents(dir, "P")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("%d rows, want the parent and its child: %+v", len(rows), rows)
	}
	if r := rowByID(rows, "nested-child"); r.Task != "Nested task" {
		t.Errorf("child task %q, want the description its parent launched it with", r.Task)
	}
	p := rowByID(rows, parent)
	if p.Task != "Task ONE" {
		t.Errorf("parent task %q, want Task ONE", p.Task)
	}
	if p.Doing != "Agent" {
		t.Errorf("parent doing %q while its launch is in flight, want Agent", p.Doing)
	}

	post := capturedLine(t, 1)
	post["hook_event_name"], post["agent_id"] = "PostToolUse", parent
	rec(post)
	rows, _ = Subagents(dir, "P")
	if p := rowByID(rows, parent); p.Doing != "" {
		t.Errorf("parent doing %q after its launch returned, want nothing", p.Doing)
	}
	if r := rowByID(rows, "nested-child"); r.Task != "Nested task" {
		t.Errorf("the parent's PostToolUse relabelled the child: %q", r.Task)
	}
}

// What is running is what the drawer is opened to see, so it comes first, and
// within running and within ended the newest leads (MUS-F-0156). Ended rows are
// all still here: what happens to them is the owner's question, not this sort's.
func TestRunningRowsComeFirstAndNewestLeads(t *testing.T) {
	dir := t.TempDir()
	t0 := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	for i, id := range []string{"run-old", "end-old", "run-new", "end-new"} {
		record(t, dir, "P", t0.Add(time.Duration(i)*time.Second), map[string]any{
			"hook_event_name": "SubagentStart", "agent_id": id, "agent_type": "general-purpose",
		})
	}
	for _, id := range []string{"end-new", "end-old"} {
		record(t, dir, "P", t0.Add(time.Minute), map[string]any{
			"hook_event_name": "SubagentStop", "agent_id": id, "last_assistant_message": "done",
		})
	}
	rows, err := Subagents(dir, "P")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range rows {
		got = append(got, r.ID)
	}
	want := []string{"run-new", "run-old", "end-new", "end-old"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("order %v, want %v", got, want)
	}
}

// The poll that drives live rows skips a parse when the log has not moved, so
// the stamp has to move whenever it has.
func TestTheStampMovesWithTheLog(t *testing.T) {
	dir := t.TempDir()
	if got := SubagentStamp(dir, "P"); got != "" {
		t.Errorf("stamp %q for a log that does not exist", got)
	}
	record(t, dir, "P", time.Now(), map[string]any{
		"hook_event_name": "SubagentStart", "agent_id": "a1", "agent_type": "general-purpose",
	})
	first := SubagentStamp(dir, "P")
	if first == "" {
		t.Fatal("no stamp after the first event")
	}
	record(t, dir, "P", time.Now(), map[string]any{
		"hook_event_name": "SubagentStop", "agent_id": "a1",
	})
	if SubagentStamp(dir, "P") == first {
		t.Error("the stamp did not move when the log did")
	}
}

// Multi-line reaches the session whole.
//
// What this catches and what it does not, because the difference matters. It
// runs against a real pane and asserts the bytes arrive, in order, with the
// newlines intact — so a paste that silently dropped or reordered lines fails
// here. It does **not** discriminate between the paste path and plain
// keystrokes: the pane holds `cat`, which is happy with newlines either way,
// and this test passes with the paste path reverted.
//
// The mechanism is justified elsewhere. That a bracketed paste reaches an
// agent's composer as one message rather than as one prompt per line was
// measured against the real CLI and is written up in
// records/work-units/MUS-W-0019.md; that Send actually uses it is asserted by
// TestThePasteBufferIsDroppedAfterDelivery, on the fake runner, where the
// command line is visible.
func TestAMultiLineMessageArrivesInOnePiece(t *testing.T) {
	realTmux(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "got.txt")
	a := &Adapter{}

	// `cat > file` ends on EOF, not on a newline, so every line it receives
	// lands in the file and a premature Enter cannot hide by looking like a
	// finished message.
	if _, err := a.Start(context.Background(), "zzMultiline", dir, "sh -c 'cat > "+out+"'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Stop(context.Background(), "zzMultiline") })

	msg := "ALPHA\nBRAVO\nCHARLIE"
	if err := a.Send(context.Background(), "zzMultiline", msg); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(15 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(out)
		if err == nil {
			got = string(b)
			if strings.Contains(got, "CHARLIE") {
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	for _, want := range []string{"ALPHA", "BRAVO", "CHARLIE"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q never arrived; the pane received %q", want, got)
		}
	}
	// Order preserved, and on separate lines: a paste that arrived as one run-on
	// line would satisfy the check above and still be wrong.
	if i, j := strings.Index(got, "ALPHA"), strings.Index(got, "CHARLIE"); i < 0 || j < 0 || i > j {
		t.Errorf("lines arrived out of order: %q", got)
	}
	if !strings.Contains(got, "ALPHA\nBRAVO") && !strings.Contains(got, "ALPHA\r\nBRAVO") {
		t.Errorf("the newline between lines did not survive: %q", got)
	}
}

// A single-line message still goes as keystrokes, which is the path the answer
// delivery from milestone 4a uses and the one with shipped behaviour behind it.
func TestASingleLineStillGoesAsKeystrokes(t *testing.T) {
	f := &fake{}
	a := &Adapter{Run: f}
	f.out = listing(owned("mustur/One", 1, false))
	if err := a.Send(context.Background(), "One", "just one line"); err != nil {
		t.Fatal(err)
	}
	if !f.ran("send-keys -t mustur/One -l just one line") {
		t.Errorf("a single line did not go as keystrokes: %v", f.calls)
	}
	if f.ran("paste-buffer") {
		t.Errorf("a single line went through the paste path: %v", f.calls)
	}
}

// And the paste path cleans up after itself: a draft left in tmux's buffer
// stack is the owner's prose readable by anything on the machine.
func TestThePasteBufferIsDroppedAfterDelivery(t *testing.T) {
	f := &fake{}
	a := &Adapter{Run: f}
	f.out = listing(owned("mustur/Two", 1, false))
	if err := a.Send(context.Background(), "Two", "line one\nline two"); err != nil {
		t.Fatal(err)
	}
	var pasted string
	for _, c := range f.calls {
		joined := strings.Join(c, " ")
		if strings.Contains(joined, "paste-buffer") {
			pasted = joined
		}
	}
	if pasted == "" {
		t.Fatalf("multi-line did not use the paste path: %v", f.calls)
	}
	if !strings.Contains(pasted, "-d") {
		t.Errorf("the paste buffer is not dropped: %q", pasted)
	}
	if !strings.Contains(pasted, "-p") {
		t.Errorf("the paste is not bracketed, so the receiving program has to guess: %q", pasted)
	}
}
