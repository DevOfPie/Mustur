package intake

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

func routing() []record.Record {
	return []record.Record{
		{ID: "MUS-R-0001", Kind: "repository", Title: "DevOfPie/Mustur", At: "2026-08-20"},
		{ID: "MUS-H-0001", Kind: "machine", Title: "whippy-vm", At: "2026-08-20"},
		{ID: "MUS-P-0002", Kind: "project", Title: "Idea inbox", At: "2026-08-20",
			Data: []record.Field{{Key: DefaultField, Value: DefaultValue}}},
		{ID: "MUS-D-0001", Kind: "decision", Title: "Mustur", At: "2026-08-20"},
	}
}

func TestAnObviousHintRoutes(t *testing.T) {
	// "Mustur" is the short half of DevOfPie/Mustur. The owner types the short
	// name and should not have to know the long one.
	got := Route("the mustur audit should report waivers on the phone surface", routing())
	if got.ID != "MUS-R-0001" {
		t.Fatalf("routed to %s (%s)", got.ID, got.Why)
	}
	if !strings.Contains(got.Why, "names") {
		t.Errorf("the reason does not say what it saw: %q", got.Why)
	}
}

func TestNoHintFallsBackToTheDeclaredDefault(t *testing.T) {
	got := Route("something about billing that has no home yet", routing())
	if got.ID != "MUS-P-0002" {
		t.Fatalf("routed to %s (%s)", got.ID, got.Why)
	}
	if !strings.Contains(got.Why, "no destination is obvious") {
		t.Errorf("reason: %q", got.Why)
	}
}

// Two names is not an obvious hint, it is an ambiguous one, and choosing
// between them is the decision this surface refuses to ask for.
func TestTwoHintsIsNotAHint(t *testing.T) {
	got := Route("move mustur off whippy-vm one day", routing())
	if got.ID != "MUS-P-0002" {
		t.Fatalf("routed to %s (%s)", got.ID, got.Why)
	}
	if !strings.Contains(got.Why, "more than one") {
		t.Errorf("the reason does not say it was ambiguous: %q", got.Why)
	}
	if !strings.Contains(got.Why, "whippy-vm") || !strings.Contains(got.Why, "Mustur") {
		t.Errorf("the reason does not name what it saw: %q", got.Why)
	}
}

// Only routing records are destinations. A decision that happens to be titled
// "Mustur" is not somewhere a jot can go.
func TestOnlyRoutingRecordsAreDestinations(t *testing.T) {
	got := Route("Mustur", []record.Record{
		{ID: "MUS-D-0001", Kind: "decision", Title: "Mustur", At: "2026-08-20"},
	})
	if got.ID != "" {
		t.Fatalf("routed to a %s record: %s", "decision", got.ID)
	}
	if !strings.Contains(got.Why, "no default") {
		t.Errorf("reason: %q", got.Why)
	}
}

// Without word boundaries a name matches inside a longer word and routes a jot
// about something else entirely.
func TestNamesMatchOnWordBoundaries(t *testing.T) {
	if got := Route("blockchain mustursomething", routing()); got.ID != "MUS-P-0002" {
		t.Errorf("a substring match routed a jot: %s", got.ID)
	}
	if got := Route("Mustur.", routing()); got.ID != "MUS-R-0001" {
		t.Errorf("punctuation after a name defeated the match: %s (%s)", got.ID, got.Why)
	}
}

func TestAliasesAreDeclaredOnTheRecord(t *testing.T) {
	rs := routing()
	rs[0].Data = []record.Field{{Key: "Aliases", Value: "the router, MUS"}}
	if got := Route("the router needs a phone surface", rs); got.ID != "MUS-R-0001" {
		t.Fatalf("an alias did not route: %s (%s)", got.ID, got.Why)
	}
}

func TestTitleIsDerivedRatherThanAsked(t *testing.T) {
	cases := []struct{ in, want string }{
		{"queue.md needs an evidence column", "queue.md needs an evidence column"},
		{"first line\nsecond line", "first line"},
		{"The audit should waive this. And then some more prose follows.", "The audit should waive this"},
		{"  spaced   out   words  ", "spaced out words"},
	}
	for _, c := range cases {
		if got := Title(c.in); got != c.want {
			t.Errorf("Title(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	long := Title(strings.Repeat("word ", 60))
	if len([]rune(long)) > 100 {
		t.Errorf("a long jot produced a %d-character title", len([]rune(long)))
	}
	if !strings.HasSuffix(long, "…") {
		t.Errorf("a truncated title does not say it was truncated: %q", long)
	}
}

func TestFileWritesAFindingCarryingItsRouting(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, r := range routing() {
		if r.Kind == "decision" {
			continue
		}
		if err := s.Append(ctx, r, "create", "test"); err != nil {
			t.Fatal(err)
		}
	}

	r, to, err := File(ctx, s, "MUS", "the mustur composer eats drafts on a dropped connection", "pie",
		time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "finding" || r.ID != "MUS-F-0001" {
		t.Fatalf("filed %s as %s", r.ID, r.Kind)
	}
	if to.ID != "MUS-R-0001" {
		t.Errorf("routed to %s", to.ID)
	}
	if r.At != "2026-08-20" {
		t.Errorf("date %q: the caller's clock was not used", r.At)
	}
	// The guess is recorded as a guess, with what it saw.
	routed, _ := r.Get("Routing")
	if !strings.Contains(routed, "names") {
		t.Errorf("the record does not say why it went there: %q", routed)
	}
	if filed, _ := r.Get("Filed by"); filed != "pie" {
		t.Errorf("filed by %q", filed)
	}
	// An empty evidence cell rather than an absent one: the findings-queue
	// module asks for the column, and an empty cell says "nothing recorded"
	// while a filled one would say the opposite.
	if evidence, ok := r.Get("Evidence"); !ok || evidence != "" {
		t.Errorf("evidence = %q, present %v", evidence, ok)
	}
	back, err := s.Get(ctx, r.ID)
	if err != nil || back.Title != r.Title {
		t.Errorf("the record did not survive the round trip: %v", err)
	}
}

func TestFilingNothingIsRefused(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, _, err := File(ctx, s, "MUS", "   \n  ", "pie", time.Now()); err == nil {
		t.Fatal("an empty jot was filed")
	}
}
