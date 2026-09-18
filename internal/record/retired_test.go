package record

import (
	"sort"
	"strings"
	"testing"
)

func TestTheRetirementFieldsParseOneWay(t *testing.T) {
	for _, c := range []struct{ in, old, new string }{
		{"IDW-F-0001 = _IB-F-0001", "IDW-F-0001", "_IB-F-0001"},
		{"IDW-F-0001=_IB-F-0001", "IDW-F-0001", "_IB-F-0001"},
		{"  IDW-F-0006   =   _IB-F-0006 ", "IDW-F-0006", "_IB-F-0006"},
	} {
		old, new, err := ParseRenamed(c.in)
		if err != nil || old != c.old || new != c.new {
			t.Errorf("ParseRenamed(%q) = %q, %q, %v", c.in, old, new, err)
		}
	}
	for _, bad := range []string{"IDW-F-0001 -> _IB-F-0001", "IDW-F-0001 = ", "IDW-F-1 = _IB-F-0001",
		"IDW-F-0001 = _IB-F-0001 = X", "IDW-F-0001 = IDW-F-0001", "idw-f-0001 = _ib-f-0001"} {
		if _, _, err := ParseRenamed(bad); err == nil {
			t.Errorf("ParseRenamed(%q) accepted", bad)
		}
	}
	id, why, err := ParseRetired("IDW-F-0007 :: named in MUS-Q-0153 and never issued")
	if err != nil || id != "IDW-F-0007" || why != "named in MUS-Q-0153 and never issued" {
		t.Errorf("ParseRetired = %q, %q, %v", id, why, err)
	}
	for _, bad := range []string{"IDW-F-0007", "IDW-F-0007 ::", "IDW-F-0007 ::   ", "IDW-F-7 :: why", ":: why"} {
		if _, _, err := ParseRetired(bad); err == nil {
			t.Errorf("ParseRetired(%q) accepted", bad)
		}
	}
}

// The carrier and what it cites show retired identifiers as plain text; what
// the Renamed fields point at does not, and nothing else does.
func TestRetirementsReachTheCarrierAndWhatItCites(t *testing.T) {
	carrier := Record{ID: "MUS-D-0192", Kind: "decision", Title: "Renamed", At: "2026-09-18",
		Refs: []Field{{Key: "answers", Value: "MUS-Q-0153"}},
		Data: []Field{
			{Key: RenamedField, Value: "IDW-F-0001 = _IB-F-0001"},
			{Key: RenamedField, Value: "IDW-F-0004 = _IB-F-0004"},
			{Key: RetiredField, Value: "IDW-F-0007 :: never issued"},
			{Key: RenamedField, Value: "not a pair"},
		}}
	ids, problems := RetiredBy(carrier)
	if strings.Join(ids, ",") != "IDW-F-0001,IDW-F-0004,IDW-F-0007" {
		t.Errorf("retired %v", ids)
	}
	if len(problems) != 1 || !strings.Contains(problems[0].Error(), "not a pair") {
		t.Errorf("problems %v", problems)
	}
	ret := Retire([]Record{carrier, {ID: "MUS-Q-0153", Kind: "question", Title: "q", At: "2026-09-18"}})
	var all []string
	for id := range ret.IDs {
		all = append(all, id)
	}
	sort.Strings(all)
	if strings.Join(all, ",") != "IDW-F-0001,IDW-F-0004,IDW-F-0007" {
		t.Errorf("IDs %v", all)
	}
	for _, where := range []string{"MUS-D-0192", "MUS-Q-0153"} {
		if p := ret.PlainIn(where); !p["IDW-F-0001"] || !p["IDW-F-0007"] {
			t.Errorf("%s shows %v as plain", where, p)
		}
	}
	for _, where := range []string{"_IB-F-0001", "IDW-F-0001", "MUS-D-0001"} {
		if p := ret.PlainIn(where); p != nil {
			t.Errorf("%s shows %v as plain, and should show nothing", where, p)
		}
	}
}
