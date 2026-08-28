package intake

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/seed"
	"github.com/DevOfPie/Mustur/internal/store"
)

// routing is the registry as the store actually holds it, not a convenient
// subset of it. An earlier version of this fixture modelled the second
// "Mustur"-titled record as a decision, which the router filters out — so the
// one record that makes the real registry ambiguous was the one record the
// test left out, and the suite was green while no jot naming this repository
// could route to it.
func routing() []record.Record {
	return []record.Record{
		{ID: "MUS-R-0001", Kind: "repository", Title: "DevOfPie/Mustur", At: "2026-08-20"},
		{ID: "MUS-H-0001", Kind: "machine", Title: "whippy-vm", At: "2026-08-20"},
		{ID: "MUS-P-0001", Kind: "project", Title: "Mustur", At: "2026-08-20",
			Data: []record.Field{{Key: "Repositories", Value: "MUS-R-0001"}, {Key: "Machines", Value: "MUS-H-0001"}}},
		{ID: "MUS-P-0002", Kind: "project", Title: "Idea inbox", At: "2026-08-20",
			Data: []record.Field{
				{Key: DefaultField, Value: DefaultValue},
				{Key: PrefixField, Value: "IDW"},
			}},
		{ID: "MUS-D-0001", Kind: "decision", Title: "Mustur", At: "2026-08-20"},
	}
}

// The jot a reader would expect to work, against the registry as it is: the
// project and the repository inside it both answer to "Mustur", and the
// narrower one wins. Without this the only reachable destination in the whole
// registry was the machine.
func TestNamingThisRepositoryRoutesToIt(t *testing.T) {
	for _, jot := range []string{
		"Mustur should log slow queries",
		"DevOfPie/Mustur should log slow queries",
		"the mustur audit should report waivers",
	} {
		got := Route(jot, routing())
		if got.ID != "MUS-R-0001" {
			t.Errorf("%q routed to %s (%s)", jot, got.ID, got.Why)
		}
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

	r, to, err := File(ctx, s, Request{
		Project: "MUS",
		Text:    "the mustur composer eats drafts on a dropped connection",
		Actor:   "pie",
		Now:     time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
	})
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
	if _, _, err := File(ctx, s, Request{Project: "MUS", Text: "   \n  ", Actor: "pie", Now: time.Now()}); err == nil {
		t.Fatal("an empty jot was filed")
	}
}

// A destination the filer picked is never overruled by the guess. The point of
// offering the choice is that somebody knows something the text does not say.
func TestAnExplicitDestinationBeatsTheGuess(t *testing.T) {
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
	// The text names the repository; the filer says the machine.
	rec, to, err := File(ctx, s, Request{
		Project: "MUS", Text: "mustur needs more disk", Actor: "pie",
		To: "MUS-H-0001", Now: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if to.ID != "MUS-H-0001" {
		t.Fatalf("routed to %s (%s)", to.ID, to.Why)
	}
	if why, _ := rec.Get("Routing"); why != "chosen by the filer" {
		t.Errorf("the record does not say the destination was chosen: %q", why)
	}
}

// An identifier that is not a destination is refused rather than quietly
// falling back to the guess: the filer said something, and filing it somewhere
// else while reporting success is the failure worth avoiding.
func TestAnUnknownDestinationIsRefused(t *testing.T) {
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
	_, _, err = File(ctx, s, Request{Project: "MUS", Text: "anything", Actor: "pie", To: "MUS-R-9999", Now: time.Now()})
	if err == nil {
		t.Fatal("an unknown destination was accepted")
	}
	n, _ := s.Count(ctx)
	if n != len(routing())-1 {
		t.Errorf("a record was written for a refused destination: %d", n)
	}
}

// Where a jot routes decides what it is called.
//
// The owner filed a jot that routed correctly to the idea inbox and still read
// as mis-routed, because it was called MUS-F-0025 — indistinguishable at a
// glance from a record about Mustur itself. The prefix now comes from the
// destination (MUS-Q-0030), and the idea inbox's is IDW (MUS-Q-0031).
func TestAJotToTheIdeaInboxIsFiledUnderItsOwnPrefix(t *testing.T) {
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

	r, to, err := File(ctx, s, Request{
		Project: "MUS",
		Text:    "a thought with no obvious home",
		Actor:   "pie",
		Now:     time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if to.ID != "MUS-P-0002" {
		t.Fatalf("routed to %s, want the idea inbox", to.ID)
	}
	if r.ID != "IDW-F-0001" {
		t.Errorf("filed as %s; a jot in the idea inbox is not a Mustur record and should not be called one", r.ID)
	}

	// And one that does name Mustur still is one, from the same store, so the
	// two serials are seen not to share a counter.
	r2, to2, err := File(ctx, s, Request{
		Project: "MUS",
		Text:    "DevOfPie/Mustur intake needs a press state on the file button",
		Actor:   "pie",
		Now:     time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if to2.ID != "MUS-R-0001" {
		t.Fatalf("routed to %s, want DevOfPie/Mustur", to2.ID)
	}
	if r2.ID != "MUS-F-0001" {
		t.Errorf("filed as %s, want MUS-F-0001 — the prefixes number separately", r2.ID)
	}
}

// A routing record whose prefix is malformed files under the store's prefix
// rather than under something the identifier scheme cannot parse. Wrong in a
// way somebody can see beats wrong in a way that breaks parsing.
func TestAMalformedPrefixIsIgnoredRatherThanUsed(t *testing.T) {
	for _, bad := range []string{"IDWX", "id", "I2W", "", "   ", "ID-"} {
		r := record.Record{
			ID: "MUS-P-0009", Kind: "project", Title: "Somewhere", At: "2026-08-22",
			Data: []record.Field{
				{Key: DefaultField, Value: DefaultValue},
				{Key: PrefixField, Value: bad},
			},
		}
		if got := Route("nothing obvious here", []record.Record{r}).Prefix; got != "" {
			t.Errorf("prefix %q accepted from %q", got, bad)
		}
	}
	good := record.Record{
		ID: "MUS-P-0009", Kind: "project", Title: "Somewhere", At: "2026-08-22",
		Data: []record.Field{
			{Key: DefaultField, Value: DefaultValue},
			{Key: PrefixField, Value: " idw "},
		},
	}
	if got := Route("nothing obvious here", []record.Record{good}).Prefix; got != "IDW" {
		t.Errorf("prefix %q, want IDW trimmed and upper-cased", got)
	}
}

// The seed is what a fresh clone gets, and a prefix that exists only in this
// machine's store is a prefix the next clone does not have. The idea inbox
// having no Prefix once already shipped as a defect of exactly this shape.
func TestTheSeededIdeaInboxCarriesItsPrefix(t *testing.T) {
	records, err := seed.Records()
	if err != nil {
		t.Fatal(err)
	}
	var inbox *record.Record
	for i, r := range records {
		if r.ID == "MUS-P-0002" {
			inbox = &records[i]
		}
	}
	if inbox == nil {
		t.Fatal("the seed has no idea inbox, so a fresh clone routes nothing to it")
	}
	if got, _ := inbox.Get(PrefixField); got != "IDW" {
		t.Errorf("the seeded idea inbox's prefix is %q, want IDW", got)
	}
	if got := Route("nothing obvious", records).Prefix; got != "IDW" {
		t.Errorf("routing against the seed gives prefix %q, want IDW", got)
	}
}

// A destination chosen by hand carries its own prefix, exactly as one arrived
// at by the guess does.
//
// It did not. The prefix was read only inside Route, so a jot the router sent
// to the idea inbox was IDW-F-0001 while the same jot sent there deliberately
// was filed under the store's prefix — the identifier depended on how the
// choice was made rather than on where the record went. Found by the composer,
// which only ever chooses explicitly.
func TestAnExplicitDestinationCarriesItsPrefix(t *testing.T) {
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

	// Text that names Mustur, sent deliberately to the inbox instead: the guess
	// would have gone the other way, so this can only pass on the chosen path.
	r, to, err := File(ctx, s, Request{
		Project: "MUS",
		Text:    "DevOfPie/Mustur could use a second look at this",
		Actor:   "pie",
		To:      "MUS-P-0002",
		Now:     time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if to.ID != "MUS-P-0002" {
		t.Fatalf("routed to %s; the explicit choice was overruled", to.ID)
	}
	if !strings.HasPrefix(r.ID, "IDW-F-") {
		t.Errorf("filed as %s, want the idea inbox's prefix", r.ID)
	}
}

// A correction is not a browser repeating itself.
//
// The duplicate window exists for a phone on a flaky connection sending the
// same POST three times. A reroute re-files a record's own body on purpose, so
// it matches its own original exactly — and was handed that original back, and
// told it was already routed there. False, and unfixable for the first minute
// after filing, which is the minute somebody notices the routing was wrong
// (MUS-F-0056).
func TestADeliberateFilingIsNotTakenForADuplicate(t *testing.T) {
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

	now := time.Now()
	const text = "the same words, filed twice on purpose"
	first, _, err := File(ctx, s, Request{Project: "MUS", Text: text, Actor: "pie", Now: now})
	if err != nil {
		t.Fatal(err)
	}

	// The window still protects a retry, which is the half a fix could break.
	again, _, err := File(ctx, s, Request{Project: "MUS", Text: text, Actor: "pie", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID {
		t.Errorf("a retry inside the window became a second record: %s then %s", first.ID, again.ID)
	}

	// And a filing that says it means it gets its own record.
	meant, _, err := File(ctx, s, Request{
		Project: "MUS", Text: text, Actor: "pie", Now: now, Deliberate: true})
	if err != nil {
		t.Fatal(err)
	}
	if meant.ID == first.ID {
		t.Errorf("a deliberate filing was handed back the original, %s", meant.ID)
	}
}
