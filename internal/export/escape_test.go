package export

import (
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/record"
)

// A reserved identifier's underscore is escaped where the export writes free
// text, and nowhere in code (review of #107, nit 8).
func TestAReservedIdentifierIsEscapedOutsideCode(t *testing.T) {
	for in, want := range map[string]string{
		"a _IB-F-0001_ b":                  `a \_IB-F-0001_ b`,
		"already \\_IB-F-0001":             `already \_IB-F-0001`,
		"`_IB-F-0001` and _IB-F-0002":      "`_IB-F-0001` and \\_IB-F-0002",
		"``a ` _IB-F-0001`` _IB-F-0003":    "``a ` _IB-F-0001`` \\_IB-F-0003",
		"```\n_IB-F-0001\n```\n_IB-F-0001": "```\n_IB-F-0001\n```\n\\_IB-F-0001",
		"snake_case and MUS-D-0001":        "snake_case and MUS-D-0001",
	} {
		if got := escapeReserved(in); got != want {
			t.Errorf("escapeReserved(%q) = %q, want %q", in, got, want)
		}
	}

	files, err := Render([]record.Record{
		{ID: "_IB-F-0001", Kind: "finding", Title: "about _IB-F-0002", At: "2026-09-01",
			Body: "see _IB-F-0002_", Data: []record.Field{{Key: "Evidence", Value: "_IB-F-0002"}}},
		{ID: "_IB-F-0002", Kind: "finding", Title: "other", At: "2026-09-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	f := string(files["findings.md"])
	for _, want := range []string{"## _IB-F-0001\n", `**about \_IB-F-0002**`, `see \_IB-F-0002_`, `| Evidence | \_IB-F-0002 |`, "[_IB-F-0001](#_ib-f-0001)"} {
		if !strings.Contains(f, want) {
			t.Errorf("findings.md does not carry %q", want)
		}
	}
}

// A link with a title, or with brackets in its text, is still a link to a
// retired identifier (review of #107, nit 9).
func TestUnlinkRetiredReadsTitlesAndNestedBrackets(t *testing.T) {
	plain := map[string]bool{"IDW-F-0001": true}
	for in, want := range map[string]string{
		`[the jot](findings.md#idw-f-0001 "old name")`: "the jot",
		`[the [old] jot](findings.md#idw-f-0001)`:      "the [old] jot",
		`[x](<findings.md#idw-f-0001>)`:                "x",
		`[x](findings.md#idw-f-0002 'other')`:          `[x](findings.md#idw-f-0002 'other')`,
		`[x](https://example.com/#idw-f-0001)`:         `[x](https://example.com/#idw-f-0001)`,
	} {
		if got := unlinkRetired(in, plain); got != want {
			t.Errorf("unlinkRetired(%q) = %q, want %q", in, got, want)
		}
	}
}
