package main

// `mustur anchors FILE...` — every anchor those documents offer, one per line.
//
// This exists so that `scripts/check-links.sh` does not work anchors out for
// itself. It used to, and the two implementations disagreed in opposite
// directions on the same day: the audit refused a correct anchor by stripping a
// heading's backticks, and the script accepted one that does not exist by
// reading a `#` inside a code fence as a heading. The owner chose one
// implementation, in Go, on MUS-Q-0064.
//
// Output is `path<TAB>anchor`, with the path absolute and cleaned so a caller
// that reached the file by a relative route can still match on it. A file that
// cannot be read is an error rather than an empty list: the caller is a gate,
// and a gate that cannot read a file has not checked it.

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DevOfPie/Mustur/internal/audit"
)

func cmdAnchors(args []string) error {
	fs := flag.NewFlagSet("anchors", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	paths := fs.Args()
	if len(paths) == 0 {
		return fmt.Errorf("anchors needs at least one markdown file")
	}

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	for _, path := range paths {
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		for _, a := range audit.Anchors(string(text)) {
			fmt.Fprintf(out, "%s\t%s\n", abs, a)
		}
	}
	return nil
}
