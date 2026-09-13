package linkctrl

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/store"
)

func openStore(t *testing.T) (*store.Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, ctx
}

func TestApplyRunsOnce(t *testing.T) {
	s, ctx := openStore(t)
	findings, err := Findings(strings.NewReader(findingsFixture), "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	src := []Source{{Path: "deferred-findings.md", Records: findings}}
	if n, err := Apply(ctx, s, src); err != nil || n != 2 {
		t.Fatalf("first import: %d, %v", n, err)
	}
	if _, err := Apply(ctx, s, src); err == nil || !strings.Contains(err.Error(), "runs once") {
		t.Fatalf("second import gave %v", err)
	}
}

// A failure partway leaves nothing, so the retry is not refused over a
// partial import.
func TestApplyIsAllOrNothing(t *testing.T) {
	s, ctx := openStore(t)
	findings, err := Findings(strings.NewReader(findingsFixture), "2026-09-13")
	if err != nil {
		t.Fatal(err)
	}
	broken := append(findings, findings[0])
	if _, err := Apply(ctx, s, []Source{{Records: broken}}); err == nil {
		t.Fatal("a batch holding one identifier twice was accepted")
	}
	if n, err := s.Count(ctx); err != nil || n != 0 {
		t.Fatalf("a failed import left %d record(s), %v", n, err)
	}
	if _, err := Apply(ctx, s, []Source{{Records: findings}}); err != nil {
		t.Fatalf("the retry was refused: %v", err)
	}
}
