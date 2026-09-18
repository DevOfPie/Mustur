package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A reroute with no --to is refused before the store is opened, so a mistyped
// command does not create or touch a database to say so (review of PR 108,
// nit 9).
func TestRerouteWithoutToTouchesNoStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never.db")
	err := cmdReroute([]string{"MUS-F-0001", "--db", path})
	if err == nil || !strings.Contains(err.Error(), "needs --to") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the store was opened: %v", err)
	}
}
