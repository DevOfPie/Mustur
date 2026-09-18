package ident

import "testing"

func TestParseCanonical(t *testing.T) {
	id, err := Parse("MUS-D-0001")
	if err != nil {
		t.Fatalf("MUS-D-0001: %v", err)
	}
	if id.Project != "MUS" || id.Role != Decision || id.Serial != 1 {
		t.Fatalf("parsed %+v", id)
	}
	if got := id.String(); got != "MUS-D-0001" {
		t.Fatalf("round trip gave %q", got)
	}
}

func TestParseRejects(t *testing.T) {
	// Each of these is a shape the export or a citation could otherwise carry
	// without anything noticing.
	for _, s := range []string{
		"", "MUS-D-1", "MUS-D-00001", "MU-D-0001", "mus-d-0001",
		"MUS-Z-0001", "MUS-D-0000", "MUS_D_0001", "MUS-D-0001 ",
	} {
		if _, err := Parse(s); err == nil {
			t.Errorf("%q parsed and should not have", s)
		}
	}
}

func TestLessOrdersByRoleThenSerial(t *testing.T) {
	must := func(s string) ID {
		id, err := Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	// Milestones sort before decisions because Roles says so, not because M
	// sorts before D as a letter.
	if !Less(must("MUS-M-0009"), must("MUS-D-0001")) {
		t.Error("milestone should sort before decision")
	}
	if !Less(must("MUS-D-0002"), must("MUS-D-0010")) {
		t.Error("serials should sort numerically, not as strings")
	}
	if Less(must("MUS-D-0001"), must("MUS-D-0001")) {
		t.Error("an identifier is not less than itself")
	}
}

func TestCited(t *testing.T) {
	got := Cited("Discharges MUS-M-0002, decided by MUS-D-0022 and MUS-D-0022 again; not AN-ID.")
	want := []string{"MUS-M-0002", "MUS-D-0022"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// The intake box is a list Mustur keeps for itself, and its prefix is the
// reserved form so no project can ever be given it.
func TestReservedPrefix(t *testing.T) {
	id, err := Parse("_IB-F-0001")
	if err != nil {
		t.Fatalf("_IB-F-0001: %v", err)
	}
	if id.Project != "_IB" || id.Role != Finding || id.Serial != 1 {
		t.Fatalf("parsed %+v", id)
	}
	if got := id.String(); got != "_IB-F-0001" {
		t.Fatalf("round trip gave %q", got)
	}
	if !Valid("_IB-F-0001") {
		t.Error("_IB-F-0001 should be valid")
	}
	if !ValidProject("_IB") || !ValidProject("MUS") {
		t.Error("_IB and MUS are both prefixes")
	}
	// Only the two shapes: not a reserved three, not a lower-case pair, not a
	// bare underscore, not an underscore in the middle.
	for _, s := range []string{"_IBX", "_ib", "_I", "__B", "I_B", "_", "_1B"} {
		if ValidProject(s) {
			t.Errorf("%q is a prefix and should not be", s)
		}
	}
	for _, s := range []string{"_IBX-F-0001", "_ib-f-0001", "__B-F-0001", "_IB-F-0000", "_IB-Z-0001"} {
		if Valid(s) {
			t.Errorf("%q is valid and should not be", s)
		}
	}
}

func TestReservedPrefixSortsLastAndDeterministically(t *testing.T) {
	must := func(s string) ID {
		id, err := Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	if !Less(must("ZZZ-F-0009"), must("_AA-F-0001")) {
		t.Error("a reserved prefix should sort after every project prefix")
	}
	if Less(must("_AA-F-0001"), must("ZZZ-F-0009")) {
		t.Error("Less is not antisymmetric across the two forms")
	}
	if !Less(must("_IB-F-0002"), must("_IB-F-0010")) {
		t.Error("serials within a reserved prefix should sort numerically")
	}
}

// Admitting the underscore as an identifier character must not lose a
// three-letter identifier glued to a word by one, which was found before.
func TestCitedReadsReservedIdentifiers(t *testing.T) {
	got := Cited("Parked in _IB-F-0003, see (_IB-F-0003) and MUS-D-0001; glued FOO_MUS-D-0002 and X_IB-F-0004.")
	want := []string{"_IB-F-0003", "MUS-D-0001", "MUS-D-0002", "_IB-F-0004"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
