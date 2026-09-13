package session

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// remembered stands in for the store: what Start wrote down and what Stop
// deleted, without a database.
type remembered struct {
	rows   map[string][2]string // project -> {dir, cmd}
	forgot []string
	err    error
}

func (r *remembered) RememberSession(_ context.Context, project, dir, cmd string) error {
	if r.err != nil {
		return r.err
	}
	if r.rows == nil {
		r.rows = map[string][2]string{}
	}
	r.rows[project] = [2]string{dir, cmd}
	return nil
}

func (r *remembered) ForgetSession(_ context.Context, project string) error {
	if r.err != nil {
		return r.err
	}
	delete(r.rows, project)
	r.forgot = append(r.forgot, project)
	return nil
}

func TestStartWritesDownWhatItLaunched(t *testing.T) {
	f := &fake{started: owned("mustur/Mustur", 1, false)}
	r := &remembered{}
	a := &Adapter{Run: f, Stat: func(string) error { return nil }, Remember: r}

	if _, err := a.Start(context.Background(), "Mustur", "/some/checkout", "claude"); err != nil {
		t.Fatal(err)
	}
	got, ok := r.rows["Mustur"]
	if !ok {
		t.Fatal("a started session was not written down, so a reboot loses it")
	}
	if got[0] != "/some/checkout" || got[1] != "claude" {
		t.Errorf("wrote down %v, want the directory and command as given", got)
	}
}

// The command recorded is the one the caller gave, without the hook Start
// appends. A stale hook baked into a stored command line would outlive the
// binary that answers it.
func TestWhatIsWrittenDownCarriesNoHook(t *testing.T) {
	f := &fake{started: owned("mustur/Mustur", 1, false)}
	r := &remembered{}
	a := &Adapter{
		Run: f, Stat: func(string) error { return nil },
		Remember: r, HookDir: "/state", Exe: "/usr/bin/mustur",
	}

	if _, err := a.Start(context.Background(), "Mustur", "/checkout", "claude"); err != nil {
		t.Fatal(err)
	}
	if !f.ran("--settings") {
		t.Fatal("the hook was not installed, so this proves nothing")
	}
	if got := r.rows["Mustur"][1]; got != "claude" {
		t.Errorf("wrote down %q, want the command without the hook", got)
	}
}

// A session that died on startup is not something to offer back.
func TestACommandThatDiedIsNotWrittenDown(t *testing.T) {
	f := &fake{started: owned("mustur/Mustur", 1, false), vanished: true}
	r := &remembered{}
	a := &Adapter{Run: f, Stat: func(string) error { return nil }, Remember: r}

	if _, err := a.Start(context.Background(), "Mustur", "/checkout", "true"); err == nil {
		t.Fatal("a session that does not exist was reported as started")
	}
	if len(r.rows) != 0 {
		t.Errorf("a session that never ran was written down: %v", r.rows)
	}
}

// The session is running. Failing to write it down loses the offer to start it
// again, and saying the start failed would be worse than the thing that failed.
func TestAStoreThatWillNotWriteDoesNotFailTheStart(t *testing.T) {
	f := &fake{started: owned("mustur/Mustur", 1, false)}
	r := &remembered{err: errors.New("disk is full")}
	a := &Adapter{Run: f, Stat: func(string) error { return nil }, Remember: r}

	if _, err := a.Start(context.Background(), "Mustur", "/checkout", "claude"); err != nil {
		t.Fatalf("a started session was reported as failed because it could not be written down: %v", err)
	}
}

func TestStopForgets(t *testing.T) {
	f := &fake{out: listing(owned("mustur/Mustur", 1, false))}
	r := &remembered{rows: map[string][2]string{"Mustur": {"/checkout", "claude"}}}
	a := &Adapter{Run: f, Remember: r}

	if err := a.Stop(context.Background(), "Mustur"); err != nil {
		t.Fatal(err)
	}
	if len(r.forgot) != 1 || r.forgot[0] != "Mustur" {
		t.Errorf("forgot %v, want the session that was stopped", r.forgot)
	}
}

// A session that was never stopped stays written down. That is the whole
// difference between ending one and losing one.
func TestAKillThatFailedDoesNotForget(t *testing.T) {
	f := &fake{
		out:    map[string]string{"list-sessions": owned("mustur/Mustur", 1, false)},
		errFor: map[string]error{"kill-session": errors.New("no")},
	}
	r := &remembered{rows: map[string][2]string{"Mustur": {"/checkout", "claude"}}}
	a := &Adapter{Run: f, Remember: r}

	if err := a.Stop(context.Background(), "Mustur"); err == nil {
		t.Fatal("a kill that failed was reported as a stop")
	}
	if len(r.forgot) != 0 {
		t.Errorf("a session that is still running was forgotten: %v", r.forgot)
	}
}

// Nothing is written down without somewhere to write it. The answer path and
// the delivery path build an adapter with no store and must not need one.
func TestNoRemembererIsNotAFailure(t *testing.T) {
	f := &fake{started: owned("mustur/Mustur", 1, false)}
	a := &Adapter{Run: f, Stat: func(string) error { return nil }}
	if _, err := a.Start(context.Background(), "Mustur", "/checkout", "claude"); err != nil {
		t.Fatal(err)
	}
}

func TestResume(t *testing.T) {
	for _, tc := range []struct {
		name, cmd, cli, want string
	}{
		{"the ordinary case", "claude", "abc-123", "claude --resume abc-123"},
		{"flags are kept", "claude --model x", "abc", "claude --model x --resume abc"},
		{
			// A restored session's stored command carries last time's flag.
			// Two of them resume the wrong thing or nothing at all.
			"an existing resume is replaced",
			"claude --resume old-id", "new-id", "claude --resume new-id",
		},
		{"a continue is dropped too", "claude --continue", "abc", "claude --resume abc"},
		{"no conversation, no flag", "claude --resume old", "", "claude"},
		{
			// The flag belongs to one vendor. Appending it to something else
			// produces a session that will not start.
			"another CLI is left alone", "some-other-agent", "abc", "some-other-agent",
		},
		{
			// Splitting on whitespace would re-word this. A session without
			// its conversation is a disappointment; one with its arguments
			// rearranged is a defect.
			"a quoted command is left alone", `claude --model "opus 5"`, "abc", `claude --model "opus 5"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Resume(tc.cmd, tc.cli); got != tc.want {
				t.Errorf("Resume(%q, %q) = %q, want %q", tc.cmd, tc.cli, got, tc.want)
			}
		})
	}
}

// The conversation identifier is offered in a hook payload and nowhere else, so
// the hook has to ask for it.
func TestTheHookAsksForTheConversationIdentifier(t *testing.T) {
	settings, err := HookSettings("/usr/bin/mustur", "/state", "Mustur", "/var/mustur.db", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(settings, `"SessionStart"`) {
		t.Errorf("no SessionStart hook, so nothing ever learns what the conversation is called: %s", settings)
	}
	if !strings.Contains(settings, "session cli-started") {
		t.Errorf("the hook does not call back: %s", settings)
	}
	// A server told to use another store must not have its hooks write to the
	// machine's usual one.
	if !strings.Contains(settings, "--db '/var/mustur.db'") {
		t.Errorf("the store path did not reach the hook: %s", settings)
	}
}

func TestTheHookLeavesTheStorePathOutWhenThereIsNone(t *testing.T) {
	settings, err := HookSettings("/usr/bin/mustur", "/state", "Mustur", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(settings, "--db") {
		t.Errorf("an empty path was passed as a flag rather than left off: %s", settings)
	}
}
