package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/export"
	"github.com/DevOfPie/Mustur/internal/record"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDanglingCitationIsReported(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "decisions.md", "## MUS-D-0001\n\nDischarges: MUS-M-0404\n")
	problems, defined, err := Tree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if defined != 1 {
		t.Errorf("defined %d identifiers, want 1", defined)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "MUS-M-0404") {
		t.Fatalf("problems: %v", problems)
	}
}

func TestDuplicateDefinitionIsReported(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "decisions.md", "## MUS-D-0001\n")
	write(t, dir, "findings.md", "## MUS-D-0001\n")
	problems, _, err := Tree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "already defined") {
		t.Fatalf("problems: %v", problems)
	}
}

func TestAgainstStoreCatchesAHandEdit(t *testing.T) {
	dir := t.TempDir()
	records := []record.Record{{ID: "MUS-D-0001", Kind: "decision", Title: "First", At: "2026-08-19"}}
	if err := export.Write(dir, records); err != nil {
		t.Fatal(err)
	}
	if problems, err := AgainstStore(dir, records); err != nil || len(problems) != 0 {
		t.Fatalf("a fresh export disagreed with its own store: %v %v", problems, err)
	}

	path := filepath.Join(dir, "decisions.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(b), "First", "Edited by hand", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	problems, err := AgainstStore(dir, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "decisions.md") {
		t.Fatalf("a hand-edited export was not reported: %v", problems)
	}
}
