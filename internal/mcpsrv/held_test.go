package mcpsrv

import (
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/export"
)

// A reader's held jot is in its own table (MUS-D-0189), so the mandated call
// and the export — the two ways a record leaves this machine — cannot show it.
// Asserted rather than assumed, because "nothing reads that table" is exactly
// the claim a later listing could quietly stop keeping.
func TestAHeldJotIsNotInTheRouteOrTheExport(t *testing.T) {
	s, ctx := serverWith(t, fixtures()...)
	const line = "a reader's unreviewed line about the share link"
	if _, err := s.store.Hold(ctx, line, "", "acct-1"); err != nil {
		t.Fatal(err)
	}
	got, err := s.answer(ctx, Args{Repository: "Mustur", Task: "held"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "share link") {
		t.Errorf("mustur_route shows a held jot:\n%s", got)
	}
	records, err := s.store.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	files, err := export.Render(records)
	if err != nil {
		t.Fatal(err)
	}
	for path, b := range files {
		if strings.Contains(string(b), "share link") {
			t.Errorf("the export writes a held jot into %s", path)
		}
	}
}
