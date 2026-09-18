package web

import (
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

// MUS-D-0197 on the Records surface: the records describing a rename show the
// retired identifiers as text, and every other record links an identifier of
// the same spelling to the record that holds it now.
func TestARetiredIdentifierRendersAsTextWhereTheRenameIsDescribed(t *testing.T) {
	srv := serveRecords(t, "",
		record.Record{ID: "MUS-D-0192", Kind: "decision", Title: "Renamed in place", At: "2026-09-18",
			Refs: []record.Field{{Key: "answers", Value: "MUS-Q-0153"}},
			Data: []record.Field{
				{Key: record.RenamedField, Value: "IDW-F-0001 = _IB-F-0001"},
				{Key: record.RetiredField, Value: "IDW-F-0007 :: named and never issued"},
			}},
		record.Record{ID: "MUS-Q-0153", Kind: "question", Title: "What happens to them?", At: "2026-09-18",
			Refs: []record.Field{{Key: "about", Value: "IDW-F-0001"}},
			Body: "See [the jot](findings.md#idw-f-0001), IDW-F-0007, and [the box](findings.md#_ib-f-0001)."},
		record.Record{ID: "_IB-F-0001", Kind: "finding", Title: "The box's first", At: "2026-09-01"},
		// Idea Warehouse has issued its own IDW-F-0001 since.
		record.Record{ID: "IDW-F-0001", Kind: "finding", Title: "Idea Warehouse's first", At: "2026-09-20"},
		record.Record{ID: "MUS-F-0009", Kind: "finding", Title: "Elsewhere", At: "2026-09-21",
			Body: "See [the idea](findings.md#idw-f-0001) and IDW-F-0001."},
	)

	q, _ := fetch(t, srv, "/records/MUS-Q-0153")
	for _, want := range []string{
		"See the jot, IDW-F-0007, and",
		`<a href="/records/_IB-F-0001">the box</a>`,
		`<span class="k">about</span><span class="v">IDW-F-0001</span>`,
	} {
		if !strings.Contains(q, want) {
			t.Errorf("MUS-Q-0153 does not carry %q", want)
		}
	}
	for _, bad := range []string{`href="/records/IDW-F-0001"`, `href="/records/IDW-F-0007"`,
		`<summary class="badge">IDW-F-0001</summary>`, `<summary class="badge">IDW-F-0007</summary>`,
		"Idea Warehouse&#39;s first"} {
		if strings.Contains(q, bad) {
			t.Errorf("MUS-Q-0153 carries %q", bad)
		}
	}

	d, _ := fetch(t, srv, "/records/MUS-D-0192")
	if strings.Contains(d, `<summary class="badge">IDW-F-0001</summary>`) {
		t.Error("the carrier offers a retired identifier as a citation")
	}
	if !strings.Contains(d, `<summary>answers: MUS-Q-0153`) {
		t.Error("the carrier's citation of the question stopped resolving")
	}

	// The question queue renders bodies the same way.
	open := openQuestion("MUS-Q-0153", "Still open")
	open.Body = "See [the jot](findings.md#idw-f-0001)."
	qsrv, _ := serveQuestions(t, open,
		record.Record{ID: "MUS-D-0192", Kind: "decision", Title: "Renamed in place", At: "2026-09-18",
			Refs: []record.Field{{Key: "answers", Value: "MUS-Q-0153"}},
			Data: []record.Field{{Key: record.RenamedField, Value: "IDW-F-0001 = _IB-F-0001"}}},
		record.Record{ID: "IDW-F-0001", Kind: "finding", Title: "Idea Warehouse's first", At: "2026-09-20"},
	)
	queue := getFrom(t, qsrv, "/questions")
	if !strings.Contains(queue, "See the jot.") || strings.Contains(queue, `href="/records/IDW-F-0001"`) {
		t.Error("the question queue links a retired identifier")
	}

	f, _ := fetch(t, srv, "/records/MUS-F-0009")
	for _, want := range []string{`<a href="/records/IDW-F-0001">the idea</a>`, `<summary class="badge">IDW-F-0001</summary>`} {
		if !strings.Contains(f, want) {
			t.Errorf("a record outside the rename does not carry %q", want)
		}
	}
}
