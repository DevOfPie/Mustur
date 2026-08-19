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
