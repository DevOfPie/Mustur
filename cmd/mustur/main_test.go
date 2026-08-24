package main

import (
	"flag"
	"io"
	"testing"
	"time"
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

// A lifecycle verb records when it ran, not what the caller typed.
//
// Every question in this repository up to 2026-08-24 records an answer
// timestamped before the question existed, because the times were typed in by
// hand from a conversation that had already happened. The record is what says
// surfacing preceded the answer.
func TestStampedRefusesAPastTime(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.Local)

	if got, err := stamped("", now); err != nil || got != "2026-08-24 12:00" {
		t.Errorf("an empty --at gave %q, %v; want the clock", got, err)
	}
	if _, err := stamped("2026-08-24 09:00", now); err == nil {
		t.Error("a time three hours in the past was accepted")
	}
	if _, err := stamped("2026-08-23", now); err == nil {
		t.Error("yesterday was accepted")
	}
	// A minute of slack, so a run straddling the boundary is not refused for
	// being a second stale.
	if _, err := stamped("2026-08-24 11:59", now); err != nil {
		t.Errorf("a time inside the slack was refused: %v", err)
	}
	if _, err := stamped("not a time", now); err == nil {
		t.Error("an unparseable --at was accepted")
	}
	// The future is allowed: it is not the failure this guards, and refusing it
	// would break nothing that exists.
	if _, err := stamped("2026-08-24 12:30", now); err != nil {
		t.Errorf("a future time was refused: %v", err)
	}
}
