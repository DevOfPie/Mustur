package main

// Renaming records in place, which identifiers otherwise never are.
//
// This command exists for one exception and is not a general rename facility.
// MUS-D-0192 records the owner's answer to MUS-Q-0153: the intake box's six
// jots, filed as IDW-F-0001 to IDW-F-0006 before the reserved prefix existed,
// are rewritten as _IB-F-0001 to _IB-F-0006, so that IDW belongs to Idea
// Warehouse alone. Everything else keeps its identifier, and a jot filed in the
// wrong place is still corrected with `reroute`, which renames nothing.
//
// It counts by default and writes only with --apply, the way `import linkctrl`
// does: what it would change is read before anything lands. The work is
// store.Rename, which says what it touches and why.

import (
	"flag"
	"fmt"
	"strings"

	"github.com/DevOfPie/Mustur/internal/store"
)

func cmdRename(args []string) error {
	fs := flag.NewFlagSet("rename", flag.ContinueOnError)
	db := dbFlag(fs)
	keep := fs.String("keep", "", "records whose text is left as written, comma-separated: the ones recording the rename")
	apply := fs.Bool("apply", false, "write the rename; without it, only list what would change")
	acceptUnmatched := fs.Bool("accept-unmatched", false, "apply although rows spell an old identifier inside something longer; read the dry run first")
	var pairs []string
	rest := args
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		rest = fs.Args()
		if len(rest) > 0 {
			pairs = append(pairs, rest[0])
			rest = rest[1:]
		}
	}
	if len(pairs) == 0 {
		return fmt.Errorf("rename needs OLD=NEW, one or more (MUS-D-0192)")
	}
	var renames []store.Renaming
	for _, p := range pairs {
		old, new, ok := strings.Cut(p, "=")
		if !ok {
			return fmt.Errorf("%q is not OLD=NEW", p)
		}
		renames = append(renames, store.Renaming{Old: strings.TrimSpace(old), New: strings.TrimSpace(new)})
	}

	s, ctx, err := openStore(*db)
	if err != nil {
		return err
	}
	defer s.Close()

	report, err := s.Rename(ctx, renames, store.RenameOptions{
		Keep: strings.Split(*keep, ","), Apply: *apply, AcceptUnmatched: *acceptUnmatched,
	})
	if err != nil && len(report.Unmatched) > 0 {
		printUnmatched(report)
	}
	if err != nil {
		return err
	}
	for _, r := range renames {
		fmt.Printf("rename %s -> %s\n", r.Old, r.New)
	}
	byTable := map[string][]store.RenameChange{}
	for _, c := range report.Changes {
		byTable[c.Table] = append(byTable[c.Table], c)
	}
	absent := map[string]bool{}
	for _, t := range report.Absent {
		absent[t] = true
	}
	for _, t := range []string{"record_event", "attachment", "held_jot", "scratch"} {
		cs := byTable[t]
		switch {
		case absent[t]:
			fmt.Printf("%s: not in this store\n", t)
		case t == "record_event":
			fmt.Printf("%s: %d event(s) across %d record(s)\n", t, len(cs), report.Records())
		default:
			fmt.Printf("%s: %d row(s)\n", t, len(cs))
		}
		for _, c := range cs {
			printChange(c)
		}
	}
	printUnmatched(report)
	fmt.Printf("kept as written: %d event(s)\n", len(report.Kept))
	for _, c := range report.Kept {
		printChange(c)
	}
	if !report.Applied {
		fmt.Printf("record_latest: would be re-derived from the log, %d record(s)\n", report.Latest)
		fmt.Println("dry run: nothing written. --apply writes all of it in one transaction.")
		return nil
	}
	fmt.Printf("record_latest: re-derived from the log, %d record(s)\n", report.Latest)
	fmt.Println("applied.")
	return nil
}

func printUnmatched(report store.RenameReport) {
	fmt.Printf("unmatched, left as written: %d row(s)\n", len(report.Unmatched))
	for _, c := range report.Unmatched {
		if c.Table == "record_event" {
			printChange(c)
		} else {
			printChange(store.RenameChange{Row: c.Row, Record: c.Table, Where: c.Where})
		}
	}
}

func printChange(c store.RenameChange) {
	who := c.Record
	if who == "" {
		who = "-"
	}
	fmt.Printf("  %-8s %-12s %s\n", c.Row, who, strings.Join(c.Where, ", "))
}
