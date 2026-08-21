package seed

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/intake"
	"github.com/DevOfPie/Mustur/internal/store"
)

func TestRecordsAreValidAndUnique(t *testing.T) {
	records, err := Records()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 {
		t.Fatal("the seed is empty")
	}
	seen := map[string]bool{}
	for _, r := range records {
		if err := r.Validate(); err != nil {
			t.Errorf("%v", err)
		}
		if seen[r.ID] {
			t.Errorf("%s appears twice", r.ID)
		}
		seen[r.ID] = true
	}
}

// Every seeded record either stands alone or cites records that are also
// seeded. A citation to nothing is a dangling identifier the moment it is
// exported.
func TestSeedCitesOnlyWhatItSeeds(t *testing.T) {
	records, err := Records()
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, r := range records {
		known[r.ID] = true
	}
	for _, r := range records {
		for _, cited := range r.Cites() {
			if !known[cited] {
				t.Errorf("%s cites %s, which the seed does not hold", r.ID, cited)
			}
		}
	}
}

// Milestone 2c's done-when ends "defaults to the idea inbox where it is not",
// and the fallback is a record rather than a constant, so the clause holds only
// if the seed ships one. Every other test of the default supplies it as a
// fixture, which is how the suite stayed green while a fresh clone routed an
// unroutable jot to nowhere.
func TestSeedDeclaresAnIntakeDefault(t *testing.T) {
	records, err := Records()
	if err != nil {
		t.Fatal(err)
	}
	var defaults []string
	for _, r := range records {
		if v, ok := r.Get(intake.DefaultField); ok && strings.EqualFold(strings.TrimSpace(v), intake.DefaultValue) {
			defaults = append(defaults, r.ID)
		}
	}
	if len(defaults) != 1 {
		t.Fatalf("the seed declares %d intake defaults (%v); it must declare exactly one", len(defaults), defaults)
	}

	// The record existing is not the clause. Routing has to reach it.
	got := intake.Route("some entirely unrelated thought about gardening", records)
	if got.ID != defaults[0] {
		t.Errorf("an unroutable jot went to %q (%s), not to the declared default %s", got.Name, got.ID, defaults[0])
	}
}

func TestApplyOnceThenRefuses(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	n, err := Apply(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("seeded nothing")
	}
	_, err = Apply(ctx, s)
	if err == nil || !strings.Contains(err.Error(), "runs once") {
		t.Fatalf("second seed gave %v", err)
	}
}
