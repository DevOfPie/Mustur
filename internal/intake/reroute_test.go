package intake

import (
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/status"
)

func refOf(r record.Record, key string) string {
	for _, f := range r.Refs {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
}

// A moved jot's stub is dropped under superseded and cites its replacement
// under the lowercase citation MUS-D-0200 names; the replacement is open and
// unreviewed, as the jot was (MUS-D-0196).
func TestARerouteDropsTheStubAndKeepsTheJotOpen(t *testing.T) {
	s, ctx := openWith(t, withOptOut())
	r, _, err := File(ctx, s, Request{Project: "MUS", Text: "archive the old intake notes", Actor: "pie", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	done, err := Reroute(ctx, s, RerouteRequest{Project: "MUS", ID: r.ID, To: "MUS-P-0003", Actor: "owner", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.Get(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if w, st := status.WordOf(old), status.StateOf(old); w != status.Superseded || st != status.Dropped {
		t.Errorf("stub: Status %q, State %q; want superseded and dropped", w, st)
	}
	if got := refOf(old, SupersededByRef); got != done.Fresh.ID {
		t.Errorf("stub cites %q under %q, want %s", got, SupersededByRef, done.Fresh.ID)
	}
	if got := refOf(old, SupersededBy); got != "" {
		t.Errorf("stub still writes the capitalised citation: %q", got)
	}
	// The data field stays: it carries why, and older code reads it.
	if v, _ := old.Get(SupersededBy); !strings.HasPrefix(v, done.Fresh.ID+" — ") {
		t.Errorf("%s = %q", SupersededBy, v)
	}
	fresh, err := s.Get(ctx, done.Fresh.ID)
	if err != nil {
		t.Fatal(err)
	}
	if w, st := status.WordOf(fresh), status.StateOf(fresh); w != status.Unreviewed || st != status.Open {
		t.Errorf("replacement: Status %q, State %q; want unreviewed and open", w, st)
	}
	// One of each, not the jot's pair plus a second pair.
	n := 0
	for _, f := range fresh.Data {
		if f.Key == status.StateField || f.Key == status.StatusField {
			n++
		}
	}
	if n != 2 {
		t.Errorf("replacement carries %d State/Status fields: %v", n, fresh.Data)
	}
}

// A jot already triaged keeps its triage when it moves.
func TestARerouteCarriesATriagedStatus(t *testing.T) {
	s, ctx := openWith(t, withOptOut())
	r, _, err := File(ctx, s, Request{Project: "MUS", Text: "archive the old intake notes", Actor: "pie", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	status.Set(&r, status.Done, "noted")
	if err := s.Append(ctx, r, "amend", "test"); err != nil {
		t.Fatal(err)
	}
	done, err := Reroute(ctx, s, RerouteRequest{Project: "MUS", ID: r.ID, To: "MUS-P-0003", Actor: "owner", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if w, st := status.WordOf(done.Fresh), status.StateOf(done.Fresh); w != "noted" || st != status.Done {
		t.Errorf("Status %q, State %q; want noted and done carried", w, st)
	}
}

// A jot takes the State its word means where it lands. A word the destination
// does not declare means nothing there, so the jot arrives unreviewed and open
// and the word goes into its Note (review of #109, m2).
func TestARerouteGivesAStatelessJotTheStateItsWordMeans(t *testing.T) {
	rs := withOptOut()
	for i := range rs {
		if rs[i].ID == "MUS-P-0003" {
			rs[i].Data = append(rs[i].Data, record.Field{Key: status.WordField, Value: "noted = done :: nothing to do"})
		}
	}
	for _, c := range []struct{ word, wantWord, wantState string }{
		{"noted", "noted", status.Done},
		{"", status.Unreviewed, status.Open},
		{"undeclared", status.Unreviewed, status.Open},
	} {
		s, ctx := openWith(t, rs)
		r, _, err := File(ctx, s, Request{Project: "MUS", Text: "archive the old intake notes", Actor: "pie", Now: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		var kept []record.Field
		for _, f := range r.Data {
			if f.Key != status.StateField && f.Key != status.StatusField {
				kept = append(kept, f)
			}
		}
		if c.word != "" {
			kept = append(kept, record.Field{Key: status.StatusField, Value: c.word})
		}
		r.Data = kept
		if err := s.Append(ctx, r, "amend", "test"); err != nil {
			t.Fatal(err)
		}
		done, err := Reroute(ctx, s, RerouteRequest{Project: "MUS", ID: r.ID, To: "MUS-P-0003", Actor: "owner", Now: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		if w, st := status.WordOf(done.Fresh), status.StateOf(done.Fresh); w != c.wantWord || st != c.wantState {
			t.Errorf("word %q: Status %q, State %q; want %q and %q", c.word, w, st, c.wantWord, c.wantState)
		}
		note, _ := done.Fresh.Get("Note")
		if c.word == "undeclared" && !strings.Contains(note, "Status was undeclared before it was moved to ARC") {
			t.Errorf("the word it carried is lost: Note %q", note)
		}
		if c.word != "undeclared" && note != "" {
			t.Errorf("word %q: a Note was written: %q", c.word, note)
		}
	}
}

// A stub is written superseded and dropped even where its own project's list
// does not declare superseded, and the reroute is not failed over it: the
// correction matters more than the stub's word, which the gate will name.
func TestAStubIsSupersededWhateverItsListSays(t *testing.T) {
	rs := withOptOut()
	for i := range rs {
		if rs[i].ID == "MUS-P-0002" {
			rs[i].Data = append(rs[i].Data, record.Field{Key: status.WordField, Value: "unreviewed = open :: a jot"})
		}
	}
	s, ctx := openWith(t, rs)
	r, _, err := File(ctx, s, Request{Project: "MUS", Text: "archive the old intake notes", Actor: "pie", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Reroute(ctx, s, RerouteRequest{Project: "MUS", ID: r.ID, To: "MUS-P-0003", Actor: "owner", Now: time.Now()}); err != nil {
		t.Fatal(err)
	}
	old, err := s.Get(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.WordOf(old) != status.Superseded || status.StateOf(old) != status.Dropped {
		t.Errorf("stub: %v", old.Data)
	}
}

// Every finding intake writes passes the check `mustur add` and `amend` refuse
// on (MUS-D-0196): a filed jot, a kept jot, a moved jot and its stub. The web
// intake box, Move and Keep call these same functions, so this is their check
// too.
func TestEveryFindingIntakeWritesIsOneItsProjectDeclares(t *testing.T) {
	words := []string{
		"unreviewed = open :: not triaged",
		"superseded = dropped :: rerouted",
	}
	rs := withOptOut()
	for i := range rs {
		if rs[i].Kind != "project" {
			continue
		}
		if rs[i].ID == "MUS-P-0001" {
			rs[i].Data = append(rs[i].Data, record.Field{Key: PrefixField, Value: "MUS"})
		}
		for _, w := range words {
			rs[i].Data = append(rs[i].Data, record.Field{Key: status.WordField, Value: w})
		}
	}
	s, ctx := openWith(t, rs)
	check := func(what string, id string) {
		t.Helper()
		r, err := s.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		projects, err := s.List(ctx, "project")
		if err != nil {
			t.Fatal(err)
		}
		index, _ := status.Index(projects)
		if !index.Declared() {
			t.Fatal("the fixture declares no list, so nothing was checked")
		}
		prefix := strings.SplitN(id, "-", 2)[0]
		if no := status.Finding(prefix, r, index); no != nil {
			t.Errorf("%s: %v", what, no)
		}
	}

	filed, _, err := File(ctx, s, Request{Project: "MUS", Text: "archive the old intake notes", Actor: "pie", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	check("filed", filed.ID)
	if _, err := Keep(ctx, s, filed.ID, "owner@example.com", time.Now()); err != nil {
		t.Fatal(err)
	}
	check("kept", filed.ID)

	other, _, err := File(ctx, s, Request{Project: "MUS", Text: "archive the older intake notes", Actor: "pie", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	moved, err := Reroute(ctx, s, RerouteRequest{Project: "MUS", ID: other.ID, To: "MUS-P-0003", Actor: "owner", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	check("moved", moved.Fresh.ID)
	check("stub", other.ID)
}

// A stub written with only the lowercase citation is still a stub.
func TestAStubCitingItsReplacementIsNotReroutedAgain(t *testing.T) {
	s, ctx := openWith(t, withOptOut())
	r, _, err := File(ctx, s, Request{Project: "MUS", Text: "archive the old intake notes", Actor: "pie", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	r.Refs = append(r.Refs, record.Field{Key: SupersededByRef, Value: "ARC-F-0009"})
	if err := s.Append(ctx, r, "amend", "test"); err != nil {
		t.Fatal(err)
	}
	_, err = Reroute(ctx, s, RerouteRequest{Project: "MUS", ID: r.ID, To: "MUS-P-0003", Actor: "owner", Now: time.Now()})
	if err == nil || !strings.Contains(err.Error(), "already corrected, by ARC-F-0009") {
		t.Errorf("err = %v", err)
	}
}
