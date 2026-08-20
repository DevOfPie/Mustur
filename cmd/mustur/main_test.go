package main

import (
	"flag"
	"io"
	"testing"
)

// The bug this exists for is silent: Go's flag package stops at the first
// non-flag argument, so a flag written after the positional is left unread and
// the command runs against the wrong store saying nothing.
func TestPositionalParsesInEitherOrder(t *testing.T) {
	for _, args := range [][]string{
		{"MUS-D-0001", "--db", "/tmp/x.db"},
		{"--db", "/tmp/x.db", "MUS-D-0001"},
		{"MUS-D-0001"},
	} {
		fs := flag.NewFlagSet("get", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		db := fs.String("db", "default", "")
		got, err := parseWithPositional(fs, args, "needs one identifier")
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if got != "MUS-D-0001" {
			t.Errorf("%v: positional = %q", args, got)
		}
		if len(args) > 1 && *db != "/tmp/x.db" {
			t.Errorf("%v: the flag was not read, db = %q", args, *db)
		}
	}
}

func TestPositionalRefusesTheWrongCount(t *testing.T) {
	for _, args := range [][]string{{}, {"one", "two"}} {
		fs := flag.NewFlagSet("get", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		fs.String("db", "default", "")
		if _, err := parseWithPositional(fs, args, "needs one identifier"); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
}

// Fields are ordered because the order is the author's and the export renders
// it. A map would have decided how every record reads.
func TestFieldsKeepTheirOrder(t *testing.T) {
	var f fields
	for _, v := range []string{"Depends on=nothing", "Risks=some", "Done means=it is done"} {
		if err := f.Set(v); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"Depends on", "Risks", "Done means"}
	for i, key := range want {
		if f[i].Key != key {
			t.Fatalf("field %d is %q, want %q", i, f[i].Key, key)
		}
	}
	if f[2].Value != "it is done" {
		t.Errorf("value = %q", f[2].Value)
	}
}

func TestFieldsRefuseWhatIsNotAPair(t *testing.T) {
	var f fields
	for _, v := range []string{"nokey", "=novalue", " =blank"} {
		if err := f.Set(v); err == nil {
			t.Errorf("%q was accepted as a field", v)
		}
	}
}
