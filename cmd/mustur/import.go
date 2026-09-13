package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/linkctrl"
)

// cmdImport reads a project's records out of its tree. It counts by default
// and writes only when told to: an import writes hundreds of records under
// numbers that are never reused, so the count is read before anything lands
// (MUS-D-0164).
func cmdImport(args []string) error {
	if len(args) == 0 || args[0] != "linkctrl" {
		return fmt.Errorf("import needs a source: linkctrl")
	}
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	db := dbFlag(fs)
	from := fs.String("from", "", "the LinkCtrl checkout to read")
	apply := fs.Bool("apply", false, "write the records; without it, only count them")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *from == "" {
		return fmt.Errorf("import linkctrl needs --from")
	}
	today := time.Now().UTC().Format("2006-01-02")
	notes := filepath.Join(*from, "docs", "build-notes")

	var sources []linkctrl.Source
	f, err := os.Open(filepath.Join(notes, "deferred-findings.md"))
	if err != nil {
		return err
	}
	findings, err := linkctrl.Findings(f, today)
	f.Close()
	if err != nil {
		return err
	}
	sources = append(sources, linkctrl.Source{Path: "docs/build-notes/deferred-findings.md", Records: findings})

	b, err := os.ReadFile(filepath.Join(notes, "upcoming-decisions.md"))
	if err != nil {
		return err
	}
	questions, err := linkctrl.Questions(string(b), today)
	if err != nil {
		return err
	}
	sources = append(sources, linkctrl.Source{Path: "docs/build-notes/upcoming-decisions.md", Records: questions})

	adrs, err := filepath.Glob(filepath.Join(*from, "docs", "adr", "*.md"))
	if err != nil {
		return err
	}
	sort.Strings(adrs)
	inv := linkctrl.Source{Path: "docs/adr/"}
	for _, p := range adrs {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rec, err := linkctrl.Investigation(filepath.Base(p), string(b), today)
		if err != nil {
			return err
		}
		inv.Records = append(inv.Records, rec)
	}
	sources = append(sources, inv)

	total := 0
	for _, src := range sources {
		byKind := map[string]int{}
		for _, r := range src.Records {
			byKind[r.Kind]++
		}
		var parts []string
		for k, n := range byKind {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
		sort.Strings(parts)
		fmt.Printf("%-42s %s\n", src.Path, strings.Join(parts, ", "))
		total += len(src.Records)
	}
	fmt.Printf("%-42s %d\n", "total", total)
	if !*apply {
		fmt.Println("counted, nothing written: pass --apply to write")
		return nil
	}
	s, ctx, err := openStore(*db)
	if err != nil {
		return err
	}
	defer s.Close()
	n, err := linkctrl.Apply(ctx, s, sources)
	if err != nil {
		return err
	}
	fmt.Printf("imported %d record(s) into %s\n", n, *db)
	return nil
}
