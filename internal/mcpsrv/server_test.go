package mcpsrv

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

func serverWith(t *testing.T, records ...record.Record) (*Server, context.Context) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, r := range records {
		if err := s.Append(ctx, r, "create", "test"); err != nil {
			t.Fatal(err)
		}
	}
	return &Server{store: s}, ctx
}

func fixtures() []record.Record {
	return []record.Record{
		{ID: "MUS-R-0001", Kind: "repository", Title: "DevOfPie/Mustur", At: "2026-08-19",
			Data: []record.Field{{Key: "Contract", Value: "workflow.md"}}},
		{ID: "MUS-R-0002", Kind: "repository", Title: "DevOfPie/hoard", At: "2026-09-13"},
		{ID: "MUS-R-0003", Kind: "repository", Title: "DevOfPie/LinkCtrl", At: "2026-09-13"},
		{ID: "MUS-P-0001", Kind: "project", Title: "Mustur", At: "2026-08-19",
			Data: []record.Field{{Key: "Prefix", Value: "MUS"}, {Key: "Repositories", Value: "MUS-R-0001"}}},
		// A project with no repository, as the idea inbox is: nothing resolves to it.
		{ID: "MUS-P-0002", Kind: "project", Title: "Idea inbox", At: "2026-08-22",
			Data: []record.Field{{Key: "Prefix", Value: "IDW"}}},
		{ID: "MUS-P-0003", Kind: "project", Title: "Hoard", At: "2026-09-13",
			Data: []record.Field{{Key: "Prefix", Value: "HRD"}, {Key: "Repositories", Value: "MUS-R-0002"}}},
		{ID: "MUS-P-0004", Kind: "project", Title: "LinkCtrl", At: "2026-09-13",
			Data: []record.Field{{Key: "Repositories", Value: "MUS-R-0003"}, {Key: "Prefix", Value: "LNK"}}},
		{ID: "MUS-D-0001", Kind: "decision", Title: "Inject, never offer", At: "2026-08-19"},
		{ID: "LNK-D-0001", Kind: "decision", Title: "Links are short", At: "2026-09-13"},
		{ID: "IDW-F-0001", Kind: "finding", Title: "A jot", At: "2026-09-13"},
	}
}

// The no-identifier call lists the named repository's project and no other
// (MUS-F-0149, on the owner's answer to MUS-Q-0139), under whichever spelling
// of the name a session passes.
func TestIndexIsScopedToTheRepositorysProject(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	cases := []struct {
		repo     string
		want     string
		unwanted []string
	}{
		{"Mustur", "MUS-D-0001", []string{"LNK-D-0001", "IDW-F-0001"}},
		{"DevOfPie/Mustur", "MUS-D-0001", []string{"LNK-D-0001", "IDW-F-0001"}},
		{"~/repos/DevOfPie/Mustur/", "MUS-D-0001", []string{"LNK-D-0001"}},
		{"devofpie/linkctrl", "LNK-D-0001", []string{"MUS-D-0001", "IDW-F-0001"}},
		{"LinkCtrl", "LNK-D-0001", []string{"MUS-D-0001"}},
		{"https://github.com/DevOfPie/LinkCtrl.git", "LNK-D-0001", []string{"MUS-D-0001"}},
		{"git@github.com:DevOfPie/LinkCtrl.git", "LNK-D-0001", []string{"MUS-D-0001"}},
		{"git@github.com:DevOfPie/Mustur", "MUS-D-0001", []string{"LNK-D-0001"}},
		{"/home/whippy/repos/DevOfPie/Mustur/.claude/worktrees/agent-x", "MUS-D-0001", []string{"LNK-D-0001", "IDW-F-0001"}},
		{"/home/whippy/repos/DevOfPie/Mustur/.claude/worktrees/x/sub", "MUS-D-0001", []string{"LNK-D-0001"}},
		// Nothing about the name ".claude" is known: any directory below the checkout reaches it.
		{`C:\src\DevOfPie\LinkCtrl\wt\feature`, "LNK-D-0001", []string{"MUS-D-0001"}},
		{"/home/whippy/repos/Mustur", "MUS-D-0001", []string{"LNK-D-0001"}},
		{"Hoard", "This project holds no records yet", []string{"MUS-D-0001", "LNK-D-0001"}},
	}
	for _, c := range cases {
		got, err := s.answer(ctx, Args{Repository: c.repo})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, c.want) || !strings.Contains(got, "## Routing") {
			t.Errorf("%s: missing %q or the routing:\n%s", c.repo, c.want, got)
		}
		for _, u := range c.unwanted {
			if strings.Contains(got, "- "+u+" ") {
				t.Errorf("%s: the index lists another project's %s:\n%s", c.repo, u, got)
			}
		}
	}
}

// A name nothing resolves must not fall back to the whole store: that is the
// flood MUS-F-0149 records. It says what is registered and how to reach records.
func TestUnknownRepositoryListsNoIndex(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	got, err := s.answer(ctx, Args{Repository: "Nope"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Routing", "no repository named \"Nope\"", "DevOfPie/Mustur, DevOfPie/hoard, DevOfPie/LinkCtrl", "`id`", "`kind`"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	for _, u := range []string{"- MUS-D-0001", "- LNK-D-0001"} {
		if strings.Contains(got, u) {
			t.Errorf("an unknown repository got an index line %q:\n%s", u, got)
		}
	}
}

// A path below a checkout that no registered repository answers to is as
// unknown as a mistyped name, however many segments it walks.
func TestUnknownPathListsNoIndex(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	for _, name := range []string{
		"/home/whippy/repos/DevOfPie/TradeShop/.claude/worktrees/agent-x",
		"git@github.com:DevOfPie/TradeShop.git",
	} {
		got, err := s.answer(ctx, Args{Repository: name})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, "no repository named") || !strings.Contains(got, "DevOfPie/Mustur, DevOfPie/hoard, DevOfPie/LinkCtrl") {
			t.Errorf("%s: not answered as unknown:\n%s", name, got)
		}
		if strings.Contains(got, "- MUS-D-0001") || strings.Contains(got, "- LNK-D-0001") {
			t.Errorf("%s: an unknown path got an index:\n%s", name, got)
		}
	}
}

// A bare name matching two repositories is answered as an unknown one is:
// routing, every registered repository, and no index (MUS-D-0187, on the
// owner's answer to MUS-Q-0147). Naming the pair that matched is extra.
func TestAmbiguousBareNameIsNotGuessed(t *testing.T) {
	recs := append(fixtures(), record.Record{ID: "MUS-R-0004", Kind: "repository", Title: "rleeon/hoard", At: "2026-09-13"})
	s, ctx := serverWith(t, recs...)
	for _, name := range []string{"hoard", "/home/whippy/src/hoard/.claude/worktrees/agent-x"} {
		got, err := s.answer(ctx, Args{Repository: name})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"## Routing", "more than one repository",
			"The registered repositories are: DevOfPie/Mustur, DevOfPie/hoard, DevOfPie/LinkCtrl, rleeon/hoard.", "owner/name"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: missing %q:\n%s", name, want, got)
			}
		}
		if strings.Contains(got, "## Records of") {
			t.Errorf("%s: an ambiguous name got an index:\n%s", name, got)
		}
	}
	for _, name := range []string{"DevOfPie/hoard", "/home/whippy/repos/DevOfPie/hoard/.claude/worktrees/agent-x", "git@github.com:DevOfPie/hoard.git"} {
		got, err := s.answer(ctx, Args{Repository: name})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, "Records of Hoard (HRD)") {
			t.Errorf("%s: owner/name did not resolve:\n%s", name, got)
		}
	}
}

// An empty project has no identifier above to call again with.
func TestEmptyProjectOffersNoIdentifierAbove(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	got, err := s.answer(ctx, Args{Repository: "Hoard"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "This project holds no records yet.") || strings.Contains(got, "any identifier above") {
		t.Errorf("empty project reply:\n%s", got)
	}
}

// The answer's other half: another project's records are reached with an
// identifier or a kind, from any repository.
func TestKindAndIdentifierReachEveryProject(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	got, err := s.answer(ctx, Args{Repository: "Mustur", Kind: "decision"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "LNK-D-0001") || !strings.Contains(got, "MUS-D-0001") {
		t.Errorf("a kind did not list every project:\n%s", got)
	}
	got, err = s.answer(ctx, Args{Repository: "Mustur", ID: "LNK-D-0001"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Links are short") {
		t.Errorf("an identifier in another project was not returned:\n%s", got)
	}
}

func TestIndexCarriesRoutingAndRecords(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	got, err := s.answer(ctx, Args{Repository: "Mustur", Task: "milestone 2"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Mustur: Mustur", "Session: milestone 2", "## Routing", "DevOfPie/Mustur", "workflow.md", "MUS-D-0001 — Inject, never offer"} {
		if !strings.Contains(got, want) {
			t.Errorf("the index is missing %q:\n%s", want, got)
		}
	}
}

func TestIdentifierReturnsOneRecord(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	got, err := s.answer(ctx, Args{Repository: "Mustur", ID: "MUS-D-0001"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "## MUS-D-0001") || !strings.Contains(got, "Inject, never offer") {
		t.Errorf("record not returned:\n%s", got)
	}
	if strings.Contains(got, "## Routing") {
		t.Errorf("an identifier call returned the whole index:\n%s", got)
	}
}

// An empty result reads as "nothing to say about it", which is a different
// claim from "no such record".
func TestUnknownIdentifierSaysSo(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	got, err := s.answer(ctx, Args{Repository: "Mustur", ID: "MUS-D-9999"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "holds no record MUS-D-9999") {
		t.Errorf("unknown identifier gave:\n%s", got)
	}
}

func TestMalformedIdentifierSaysWhatOneLooksLike(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	got, err := s.answer(ctx, Args{Repository: "Mustur", ID: "decision 1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "PROJECT-ROLE-SERIAL") {
		t.Errorf("malformed identifier gave:\n%s", got)
	}
}

func TestUnknownKindListsTheKinds(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	got, err := s.answer(ctx, Args{Repository: "Mustur", Kind: "tickets"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "no record kind") || !strings.Contains(got, "investigation") {
		t.Errorf("unknown kind gave:\n%s", got)
	}
}

// The argument the disproof scored is required, not optional.
func TestRepositoryIsRequired(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	if _, err := s.answer(ctx, Args{}); err == nil {
		t.Fatal("a call with no repository was answered")
	}
}

// The kind list in Args.Kind's jsonschema tag is a compile-time string, so it
// cannot be built from ident.KindNames the way the runtime list now is. This is
// what stops the two drifting: the tag omitted `question` for exactly as long
// as it took one role letter to be added, while the tool went on describing its
// own reply as "an index of every record". It is scoped to one project now
// (MUS-F-0149), but within that project every kind is still listed.
func TestIndexCarriesEveryKindIncludingQuestions(t *testing.T) {
	recs := append(fixtures(), record.Record{
		ID: "MUS-Q-0001", Kind: "question", Title: "Own the session, or attach?", At: "2026-08-21",
		Data: []record.Field{{Key: "Status", Value: "open"}},
	})
	s, ctx := serverWith(t, recs...)
	got, err := s.answer(ctx, Args{Repository: "Mustur", Task: "milestone 3"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "MUS-Q-0001") {
		t.Errorf("the index omits questions:\n%s", got)
	}
}

// A finding's line ends with its Status word (MUS-D-0196); no other kind's
// does, a question's Status included.
func TestAFindingLineCarriesItsStatusWord(t *testing.T) {
	recs := append(fixtures(),
		record.Record{ID: "MUS-F-0001", Kind: "finding", Title: "The drawer is too wide", At: "2026-08-21",
			Data: []record.Field{{Key: "Status", Value: "in-review"}, {Key: "State", Value: "open"}}},
		record.Record{ID: "MUS-F-0002", Kind: "finding", Title: "No status yet", At: "2026-08-21"},
		record.Record{ID: "MUS-Q-0001", Kind: "question", Title: "Own the session?", At: "2026-08-21",
			Data: []record.Field{{Key: "Status", Value: "open"}}},
	)
	s, ctx := serverWith(t, recs...)
	got, err := s.answer(ctx, Args{Repository: "Mustur"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"- MUS-F-0001 — The drawer is too wide · in-review\n",
		"- MUS-F-0002 — No status yet\n",
		"- MUS-Q-0001 — Own the session?\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the index is missing %q:\n%s", want, got)
		}
	}
}

func TestSchemaListsEveryKind(t *testing.T) {
	field, ok := reflect.TypeOf(Args{}).FieldByName("Kind")
	if !ok {
		t.Fatal("Args has no Kind field")
	}
	tag := field.Tag.Get("jsonschema")
	for _, kind := range ident.KindNames() {
		if !strings.Contains(tag, kind) {
			t.Errorf("the kind schema does not mention %q:\n%s", kind, tag)
		}
	}
}
