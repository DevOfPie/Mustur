package linkctrl

import (
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

func TestCiteMakesRefsForWhatTheImportHolds(t *testing.T) {
	d14 := record.Record{ID: "LNK-D-0014", Kind: "decision", Title: "fourteen", At: "2026-08-01"}
	m5 := record.Record{ID: "LNK-M-0005", Kind: "milestone", Title: "five", At: "2026-08-01"}
	f := record.Record{
		ID: "LNK-F-0001", Kind: "finding", Title: "Breaks D14", At: "2026-08-01",
		Body: "Also D14, F1 itself, D999, W30, LNK-M-0005 and `D14` in code.",
		Data: []record.Field{{Key: "Closed by", Value: "F2"}, {Key: "LinkCtrl", Value: "F1"}},
	}
	f2 := record.Record{ID: "LNK-F-0002", Kind: "finding", Title: "two", At: "2026-08-01"}
	sources := []Source{{Records: []record.Record{d14, m5, f, f2}}}
	if n := Cite(sources); n != 3 {
		t.Fatalf("added %d refs, want D14, M5 and F2 once each", n)
	}
	got := sources[0].Records[2].Refs
	want := []string{"LNK-D-0014", "LNK-M-0005", "LNK-F-0002"}
	if len(got) != len(want) {
		t.Fatalf("refs %v", got)
	}
	for i, id := range want {
		if got[i].Key != "cites" || got[i].Value != id {
			t.Fatalf("ref %d is %v, want cites %s", i, got[i], id)
		}
	}
	if Cite(sources) != 0 {
		t.Fatal("a second pass added refs again")
	}
}
