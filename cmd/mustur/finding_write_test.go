package main

// add and amend refuse a finding whose Status or State its project does not
// declare (MUS-D-0196), and write nothing when they do.

import (
	"context"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/status"
	"github.com/DevOfPie/Mustur/internal/store"
)

func listed() record.Record {
	return mus("unreviewed = open :: not triaged", "open = open :: work remains", "fixed = done :: repaired")
}

func events(t *testing.T, path, id string) int {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h, err := s.History(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return len(h)
}

func stored(t *testing.T, path, id string) record.Record {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAddFindingDefaultsToUnreviewedAndOpen(t *testing.T) {
	path := findingsStore(t, listed())
	out, err := captured(func() error {
		return cmdWrite([]string{"finding", "--db", path, "--title", "t", "--actor", "test"}, "create")
	})
	if err != nil {
		t.Fatal(err)
	}
	r := stored(t, path, strings.TrimSpace(out))
	if status.WordOf(r) != status.Unreviewed || status.StateOf(r) != status.Open {
		t.Errorf("filed %v", r.Data)
	}
}

func TestAddFindingRefusesWhatItsProjectDoesNotDeclare(t *testing.T) {
	path := findingsStore(t, listed())
	for _, c := range []struct {
		data []string
		want string
	}{
		{[]string{"Status=fixed on PR 99, not merged"}, `Status "fixed on PR 99, not merged" is not a word MUS declares`},
		{[]string{"Status=fixed", "State=closed"}, `State "closed" is not open, done or dropped; fixed means done`},
		{[]string{"Status=fixed", "State=open"}, "Status fixed means done, and State says open"},
		{[]string{"State=open"}, "it has no Status word"},
	} {
		args := []string{"finding", "--db", path, "--title", "t", "--actor", "test"}
		for _, d := range c.data {
			args = append(args, "--data", d)
		}
		err := cmdWrite(args, "create")
		if err == nil {
			t.Errorf("%v was written", c.data)
			continue
		}
		msg := err.Error()
		for _, want := range []string{"refused, nothing written", c.want, "--data Status=WORD, which brings the State it means", "--data Note=", "Status is one of: unreviewed (open), open (open), fixed (done)"} {
			if !strings.Contains(msg, want) {
				t.Errorf("%v: message lacks %q:\n%s", c.data, want, msg)
			}
		}
	}
	if n := events(t, path, "MUS-F-0001"); n != 0 {
		t.Errorf("a refused add wrote %d event(s)", n)
	}
}

func TestAmendRefusesProseInStatusAndWritesNothing(t *testing.T) {
	path := findingsStore(t, listed(), aFinding("MUS-F-0001", "open", status.Open))
	err := cmdWrite([]string{"MUS-F-0001", "--db", path, "--actor", "test", "--data", "Status=fixed on PR 99 (24b864e), stacked on PR 97, not merged"}, "amend")
	if err == nil || !strings.Contains(err.Error(), "MUS-F-0001: Status") {
		t.Fatalf("err = %v", err)
	}
	if n := events(t, path, "MUS-F-0001"); n != 1 {
		t.Errorf("%d events, want only the create", n)
	}
	// What it asked for is accepted.
	if err := cmdWrite([]string{"MUS-F-0001", "--db", path, "--actor", "test",
		"--data", "Status=fixed", "--data", "State=done", "--data", "Note=fixed on PR 99, not merged"}, "amend"); err != nil {
		t.Fatal(err)
	}
	r := stored(t, path, "MUS-F-0001")
	if status.WordOf(r) != "fixed" || status.StateOf(r) != status.Done {
		t.Errorf("amended %v", r.Data)
	}
}

// An amend that does not mention Status is still refused if the record it
// leaves behind is wrong: the check is on the result.
func TestAmendChecksTheResultNotTheFlags(t *testing.T) {
	path := findingsStore(t, listed(), aFinding("MUS-F-0001", "fixed", status.Open))
	if err := cmdWrite([]string{"MUS-F-0001", "--db", path, "--actor", "test", "--title", "better"}, "amend"); err == nil ||
		!strings.Contains(err.Error(), "Status fixed means done, and State says open") {
		t.Errorf("err = %v", err)
	}
}

// A word alone brings the State it means, on add and on amend, replacing the
// State the record had; a word and a State that disagree are still refused.
func TestAWordAloneBringsItsState(t *testing.T) {
	path := findingsStore(t, listed(), aFinding("MUS-F-0001", "open", status.Open))
	if err := cmdWrite([]string{"MUS-F-0001", "--db", path, "--actor", "test", "--data", "Status=fixed"}, "amend"); err != nil {
		t.Fatal(err)
	}
	if r := stored(t, path, "MUS-F-0001"); status.WordOf(r) != "fixed" || status.StateOf(r) != status.Done {
		t.Errorf("amended %v", r.Data)
	}
	out, err := captured(func() error {
		return cmdWrite([]string{"finding", "--db", path, "--title", "t", "--actor", "test", "--data", "Status= fixed "}, "create")
	})
	if err != nil {
		t.Fatal(err)
	}
	r := stored(t, path, strings.TrimSpace(out))
	if v, _ := r.Get(status.StatusField); v != "fixed" || status.StateOf(r) != status.Done {
		t.Errorf("added %q (trimmed?) %v", v, r.Data)
	}
}

// A second Status, or one spelled in lower case, is refused rather than filed
// beside the one the checks read.
func TestAMisspeltOrRepeatedStatusIsRefused(t *testing.T) {
	path := findingsStore(t, listed(), aFinding("MUS-F-0001", "open", status.Open))
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"finding", "--title", "t", "--data", "status=garbage"}, `a field is named "status", which is spelled Status`},
		{[]string{"finding", "--title", "t", "--data", "Status=open", "--data", "Status=fixed"}, "Status is given 2 times"},
		{[]string{"MUS-F-0001", "--data", "Status=open", "--data", "Status=fixed", "--data", "Status=open"}, ""},
	} {
		op := "create"
		if strings.HasPrefix(c.args[0], "MUS-") {
			op = "amend"
		}
		err := cmdWrite(append(c.args, "--db", path, "--actor", "test"), op)
		if c.want == "" {
			// Three passed, one already held: merge replaces the one and adds
			// two, so this is the repeat case on amend.
			if err == nil || !strings.Contains(err.Error(), "Status is given") {
				t.Errorf("amend with repeated Status: %v", err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v", c.args, err)
		}
	}
	if n := events(t, path, "MUS-F-0002"); n != 0 {
		t.Errorf("a refused add wrote %d event(s)", n)
	}
	if n := events(t, path, "MUS-F-0001"); n != 1 {
		t.Errorf("a refused amend wrote: %d events", n)
	}
}

// The reserved _IB prefix resolves through the routing record whose Prefix is
// _IB, like any other: its list is checked at write time.
func TestAnIntakeBoxFindingIsCheckedAgainstItsList(t *testing.T) {
	inbox := record.Record{ID: "MUS-P-0002", Kind: "project", Title: "Intake box", At: "2026-09-18",
		Data: []record.Field{{Key: status.PrefixField, Value: "_IB"},
			{Key: status.WordField, Value: "unreviewed = open :: a jot"},
			{Key: status.WordField, Value: "routed = done :: filed onward"}}}
	path := findingsStore(t, listed(), inbox, aFinding("_IB-F-0001", "unreviewed", status.Open))
	if err := cmdWrite([]string{"_IB-F-0001", "--db", path, "--actor", "test", "--data", "Status=fixed"}, "amend"); err == nil ||
		!strings.Contains(err.Error(), `Status "fixed" is not a word _IB declares`) {
		t.Errorf("a MUS word on an _IB jot: %v", err)
	}
	if err := cmdWrite([]string{"_IB-F-0001", "--db", path, "--actor", "test", "--data", "Status=routed"}, "amend"); err != nil {
		t.Fatal(err)
	}
	if r := stored(t, path, "_IB-F-0001"); status.StateOf(r) != status.Done {
		t.Errorf("routed brings done: %v", r.Data)
	}
}

// A project with no list is not locked out of its findings: the write goes
// through with a note naming the record to give a list to, and only a State
// that is not one is refused. _IB is such a prefix until its record has one.
func TestAProjectWithNoListCanStillWrite(t *testing.T) {
	inbox := record.Record{ID: "MUS-P-0002", Kind: "project", Title: "Intake box", At: "2026-09-18",
		Data: []record.Field{{Key: status.PrefixField, Value: "_IB"}}}
	path := findingsStore(t, listed(), inbox, aFinding("_IB-F-0001", "", ""))
	if err := cmdWrite([]string{"_IB-F-0001", "--db", path, "--actor", "test", "--data", "Note=looked at"}, "amend"); err != nil {
		t.Errorf("a note-only amend: %v", err)
	}
	if err := cmdWrite([]string{"_IB-F-0001", "--db", path, "--actor", "test", "--data", "Status=routed"}, "amend"); err != nil {
		t.Errorf("a Status with no list to check it against: %v", err)
	}
	if err := cmdWrite([]string{"_IB-F-0001", "--db", path, "--actor", "test", "--data", "State=closed"}, "amend"); err == nil ||
		!strings.Contains(err.Error(), `State "closed" is not open, done or dropped`) {
		t.Errorf("an invalid State: %v", err)
	}
	if n := events(t, path, "_IB-F-0001"); n != 3 {
		t.Errorf("%d events, want the create and two amends", n)
	}
}

// A store where no project declares a list predates the lists: nothing is
// checked, which is what keeps a fresh seed writable.
func TestNoListMeansNoCheck(t *testing.T) {
	path := findingsStore(t, mus(), aFinding("MUS-F-0001", "", ""))
	if err := cmdWrite([]string{"MUS-F-0001", "--db", path, "--actor", "test", "--data", "Status=whatever it was"}, "amend"); err != nil {
		t.Errorf("err = %v", err)
	}
}

// Other kinds are not findings.
func TestADecisionIsNotChecked(t *testing.T) {
	path := findingsStore(t, listed())
	if err := cmdWrite([]string{"decision", "--db", path, "--title", "t", "--actor", "test", "--data", "Status=accepted"}, "create"); err != nil {
		t.Errorf("err = %v", err)
	}
}
