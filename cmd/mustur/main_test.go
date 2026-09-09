package main

import (
	"flag"
	"github.com/DevOfPie/Mustur/internal/record"
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

// A repeated field is replaced in order, not collapsed.
//
// MUS-F-0094: incoming fields were held one per key, so two --data Option=
// values became the last one and then overwrote every existing Option row with
// it. A question with three options came back with three copies of the third,
// silently, from the command whose whole job is correcting records.
func TestAmendReplacesEachOccurrenceOfARepeatedField(t *testing.T) {
	old := record.Record{ID: "MUS-Q-0001", Kind: "question", Data: []record.Field{
		{Key: "Status", Value: "open"},
		{Key: "Option", Value: "A :: one :: d1"},
		{Key: "Option", Value: "B :: two :: d2"},
		{Key: "Option", Value: "C :: three :: d3"},
		{Key: "Asked by", Value: "whippy"},
	}}

	// Three passed for three held: each replaced where it stood, and the
	// fields around them keep their places.
	in := record.Record{Data: []record.Field{
		{Key: "Option", Value: "A :: Recommended. one :: d1"},
		{Key: "Option", Value: "B :: two :: d2"},
		{Key: "Option", Value: "C :: three :: d3"},
	}}
	got := merge(old, in, map[string]bool{}, nil)
	var opts []string
	for _, f := range got.Data {
		if f.Key == "Option" {
			opts = append(opts, f.Value)
		}
	}
	if len(opts) != 3 {
		t.Fatalf("got %d options, want 3: %v", len(opts), opts)
	}
	if opts[0] != "A :: Recommended. one :: d1" || opts[1] != "B :: two :: d2" || opts[2] != "C :: three :: d3" {
		t.Errorf("options came back as %v", opts)
	}
	if got.Data[0].Key != "Status" || got.Data[len(got.Data)-1].Key != "Asked by" {
		t.Errorf("the fields around them moved: %+v", got.Data)
	}

	// Fewer passed than held: the surplus is what the caller chose not to
	// restate, and it goes rather than lingering as a stale row.
	fewer := record.Record{Data: []record.Field{{Key: "Option", Value: "only :: one :: d"}}}
	got = merge(old, fewer, map[string]bool{}, nil)
	opts = nil
	for _, f := range got.Data {
		if f.Key == "Option" {
			opts = append(opts, f.Value)
		}
	}
	if len(opts) != 1 || opts[0] != "only :: one :: d" {
		t.Errorf("fewer options came back as %v", opts)
	}

	// More passed than held: the extras are appended rather than dropped.
	more := record.Record{Data: []record.Field{
		{Key: "Option", Value: "1 :: a :: d"}, {Key: "Option", Value: "2 :: b :: d"},
		{Key: "Option", Value: "3 :: c :: d"}, {Key: "Option", Value: "4 :: d :: d"},
	}}
	got = merge(old, more, map[string]bool{}, nil)
	opts = nil
	for _, f := range got.Data {
		if f.Key == "Option" {
			opts = append(opts, f.Value)
		}
	}
	if len(opts) != 4 {
		t.Fatalf("got %d options, want 4: %v", len(opts), opts)
	}
	// A field the amendment never mentioned is untouched, which is MUS-D-0134.
	if v, _ := got.Get("Status"); v != "open" {
		t.Errorf("Status = %q, want the value the amendment did not restate", v)
	}
}
