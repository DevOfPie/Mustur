package intake

// Two presses at once. The review of PR 108 reproduced a double press on Move
// filing the jot twice, 30 of 30: both presses passed the not-yet-corrected
// check before either wrote. These run the same two-goroutine scenario.

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// both runs f twice at once and waits for both.
func both(f func(i int)) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			f(i)
		}()
	}
	close(start)
	wg.Wait()
}

// Two Moves at once file one record between them. The loser is told the jot
// was already corrected, and by the record the winner filed.
func TestTwoMovesAtOnceFileOne(t *testing.T) {
	s, ctx := openWith(t, withOptOut())
	for round := range 20 {
		r, _, err := File(ctx, s, Request{Project: "MUS", Text: fmt.Sprintf("archive round %d", round), Actor: "pie", Now: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		results := make([]Rerouted, 2)
		errs := make([]error, 2)
		both(func(i int) {
			results[i], errs[i] = Reroute(ctx, s, RerouteRequest{Project: "MUS", ID: r.ID, To: "MUS-P-0003", Actor: "owner", Now: time.Now()})
		})

		all, err := s.List(ctx, "finding")
		if err != nil {
			t.Fatal(err)
		}
		var filed []string
		for _, f := range all {
			for _, ref := range f.Refs {
				if ref.Key == "Corrects" && ref.Value == r.ID {
					filed = append(filed, f.ID)
				}
			}
		}
		if len(filed) != 1 {
			t.Fatalf("round %d: two presses filed %v", round, filed)
		}
		won := 0
		for i := range 2 {
			var already *AlreadyCorrected
			switch {
			case errs[i] == nil:
				won++
				if results[i].Fresh.ID != filed[0] {
					t.Errorf("round %d: the winner says it filed %s, the store holds %s", round, results[i].Fresh.ID, filed[0])
				}
			case errors.As(errs[i], &already):
				if already.By != filed[0] {
					t.Errorf("round %d: the loser was pointed at %s, not %s", round, already.By, filed[0])
				}
			default:
				t.Errorf("round %d: %v", round, errs[i])
			}
		}
		if won != 1 {
			t.Errorf("round %d: %d presses say they moved it", round, won)
		}
	}
}

// Two Keeps at once write one Kept between them.
func TestTwoKeepsAtOnceWriteOne(t *testing.T) {
	s, ctx := openWith(t, withOptOut())
	for round := range 20 {
		r, _, err := File(ctx, s, Request{Project: "MUS", Text: fmt.Sprintf("archive keep round %d", round), Actor: "pie", Now: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		errs := make([]error, 2)
		both(func(i int) {
			_, errs[i] = Keep(ctx, s, r.ID, "owner", time.Now())
		})
		got, err := s.Get(ctx, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, f := range got.Data {
			if f.Key == KeptField {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("round %d: %d Kept fields", round, n)
		}
		for i := range 2 {
			if errs[i] != nil && !errors.Is(errs[i], ErrAlreadyKept) {
				t.Errorf("round %d: %v", round, errs[i])
			}
		}
	}
}
