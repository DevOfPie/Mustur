package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func heldStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, ctx
}

// A held jot is not a record: holding one writes nothing to the log, and the
// listing every reading path starts from does not see it.
func TestAHeldJotIsNotARecord(t *testing.T) {
	s, ctx := heldStore(t)
	if _, err := s.Hold(ctx, "the share link opens the desktop layout", "", "acct-1"); err != nil {
		t.Fatal(err)
	}
	n, err := s.Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("holding a jot wrote %d record(s)", n)
	}
	all, err := s.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range all {
		if strings.Contains(r.Body, "share link") {
			t.Errorf("a held jot is in the record listing as %s", r.ID)
		}
	}
	held, err := s.HeldJots(ctx, "acct-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 1 || !strings.HasPrefix(held[0].ID, "held-") {
		t.Fatalf("the reader's own list: %+v", held)
	}
}

func TestAReaderSeesOnlyTheirOwnHeldJots(t *testing.T) {
	s, ctx := heldStore(t)
	for _, who := range []string{"acct-1", "acct-2"} {
		if _, err := s.Hold(ctx, "from "+who, "", who); err != nil {
			t.Fatal(err)
		}
	}
	mine, _ := s.HeldJots(ctx, "acct-1")
	if len(mine) != 1 || mine[0].Text != "from acct-1" {
		t.Errorf("acct-1 sees %+v", mine)
	}
	all, _ := s.HeldJots(ctx, "")
	if len(all) != 2 {
		t.Errorf("everybody's list has %d", len(all))
	}
}

// A phone resending the same POST holds one jot, not three.
func TestTheSameSendTwiceIsHeldOnce(t *testing.T) {
	s, ctx := heldStore(t)
	a, _ := s.Hold(ctx, "a line", "", "acct-1")
	b, _ := s.Hold(ctx, "  a line ", "", "acct-1")
	if a.ID != b.ID {
		t.Errorf("a retry held a second jot: %s and %s", a.ID, b.ID)
	}
	later := time.Now().Add(2 * HoldRetry)
	s.now = func() time.Time { return later }
	c, _ := s.Hold(ctx, "a line", "", "acct-1")
	if c.ID == a.ID {
		t.Error("the same line sent after the retry window was taken as the first one")
	}
}

func TestApproveHandsTheJotOverOnce(t *testing.T) {
	s, ctx := heldStore(t)
	h, _ := s.Hold(ctx, "a line", "MUS-P-0001", "acct-1")
	var mu sync.Mutex
	filed := 0
	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = s.Approve(ctx, h.ID, func(got Held) error {
				mu.Lock()
				defer mu.Unlock()
				if got.Text != "a line" || got.To != "MUS-P-0001" || got.AccountID != "acct-1" {
					t.Errorf("file was handed %+v", got)
				}
				filed++
				return nil
			})
		}(i)
	}
	wg.Wait()
	if filed != 1 {
		t.Errorf("five approvals filed %d times", filed)
	}
	gone := 0
	for _, err := range errs {
		if errors.Is(err, ErrHeldGone) {
			gone++
		}
	}
	if gone != 4 {
		t.Errorf("%d of the four late approvals were told it was gone: %v", gone, errs)
	}
	if left, _ := s.HeldJots(ctx, ""); len(left) != 0 {
		t.Errorf("an approved jot is still held: %+v", left)
	}
}

// A filing that fails leaves the jot waiting rather than lost.
func TestAFailedApprovalPutsTheJotBack(t *testing.T) {
	s, ctx := heldStore(t)
	h, _ := s.Hold(ctx, "a line", "", "acct-1")
	boom := errors.New("no such destination")
	if err := s.Approve(ctx, h.ID, func(Held) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("Approve returned %v", err)
	}
	back, err := s.HeldJot(ctx, h.ID)
	if err != nil {
		t.Fatalf("the jot did not come back: %v", err)
	}
	if back.Text != h.Text || !back.Created.Equal(h.Created.Truncate(time.Second)) {
		t.Errorf("it came back changed: %+v, was %+v", back, h)
	}
}

func TestDiscardLeavesNothing(t *testing.T) {
	s, ctx := heldStore(t)
	h, _ := s.Hold(ctx, "a line", "", "acct-1")
	if err := s.Discard(ctx, h.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Discard(ctx, h.ID); !errors.Is(err, ErrHeldGone) {
		t.Errorf("a second discard: %v", err)
	}
	var rows int
	if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM held_jot`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Count(ctx); rows != 0 || n != 0 {
		t.Errorf("discard left %d held row(s) and %d record(s)", rows, n)
	}
}

// A store created before the table existed gains it on open, which is how the
// live store will meet this change.
func TestAnOlderStoreGainsTheHeldTable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(ctx, `DROP TABLE held_jot`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	_ = raw.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'held_jot'`).Scan(&n)
	raw.Close()
	if n != 0 {
		t.Fatal("the table was not dropped, so this tests nothing")
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Hold(ctx, "a line", "", "acct-1"); err != nil {
		t.Errorf("an older store could not hold a jot after opening: %v", err)
	}
}
