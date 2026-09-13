package linkctrl

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/store"
)

func TestApplyRunsOnce(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
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
