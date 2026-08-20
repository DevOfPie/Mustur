// Package seed puts what already existed into an empty store.
//
// The records here are the ones this repository already held as prose: the
// milestones in Plan.md, the entries in decisions.md, the lines in queue.md,
// the accepted investigation. Each becomes addressable by identifier and each
// carries a link back to the prose it summarises. Nothing is copied out: two
// copies of a rationale drift, and the file is the one a person edits.
//
// Seeding is a bootstrap and runs once. Once records are written through
// Mustur the seed no longer reproduces the store, which is why it refuses a
// store that already holds anything.
package seed

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"

	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

//go:embed data/*.json
var data embed.FS

// Actor is recorded against every seeded event, so the log distinguishes what
// was imported at bootstrap from what was written since.
const Actor = "seed"

// Records returns every seeded record, sorted, with each one validated.
func Records() ([]record.Record, error) {
	entries, err := fs.ReadDir(data, "data")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var out []record.Record
	seen := map[string]string{}
	for _, name := range names {
		b, err := data.ReadFile(path.Join("data", name))
		if err != nil {
			return nil, err
		}
		var batch []record.Record
		if err := json.Unmarshal(b, &batch); err != nil {
			return nil, fmt.Errorf("seed file %s: %w", name, err)
		}
		for _, r := range batch {
			if err := r.Validate(); err != nil {
				return nil, fmt.Errorf("seed file %s: %w", name, err)
			}
			if first, dup := seen[r.ID]; dup {
				return nil, fmt.Errorf("seed: %s appears in both %s and %s", r.ID, first, name)
			}
			seen[r.ID] = name
			out = append(out, r)
		}
	}
	record.Sort(out)
	return out, nil
}

// Apply writes every seeded record into an empty store. It refuses a store
// that already holds records rather than amending them: a bootstrap that runs
// twice is a bootstrap that has silently become an import.
func Apply(ctx context.Context, s *store.Store) (int, error) {
	n, err := s.Count(ctx)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		return 0, fmt.Errorf("store already holds %d record(s): the seed is a bootstrap and runs once", n)
	}
	records, err := Records()
	if err != nil {
		return 0, err
	}
	for _, r := range records {
		if err := s.Append(ctx, r, "create", Actor); err != nil {
			return 0, err
		}
	}
	return len(records), nil
}
