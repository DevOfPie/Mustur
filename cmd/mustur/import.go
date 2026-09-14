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
	repair := fs.Bool("repair", false, "re-state imported records read differently now; never one written since the import")
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

	log, err := os.ReadFile(filepath.Join(notes, "decisions.md"))
	if err != nil {
		return err
	}
	tables := map[string]string{}
	for _, name := range []string{"phase-2.md", "phase-3.md"} {
		b, err := os.ReadFile(filepath.Join(notes, "phase-details", name))
		if err != nil {
			return err
		}
		tables["phase-details/"+name] = string(b)
	}
	decisions, err := linkctrl.Decisions(string(log), tables, today)
	if err != nil {
		return err
	}
	sources = append(sources, linkctrl.Source{Path: "docs/build-notes/decisions.md", Records: decisions})

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

	ms := linkctrl.MilestoneSources{Files: map[string]string{}, Phases: map[string]string{}}
	files, err := filepath.Glob(filepath.Join(notes, "phase-details", "*.md"))
	if err != nil {
		return err
	}
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		switch base := filepath.Base(p); {
		case base == "phase-1.md":
			ms.Phase1 = string(b)
			ms.Phases[base] = string(b)
		case strings.HasPrefix(base, "phase-"):
			ms.Phases[base] = string(b)
		default:
			ms.Files[base] = string(b)
		}
	}
	plan, err := os.ReadFile(filepath.Join(*from, "Plan.md"))
	if err != nil {
		return err
	}
	ms.Plan = string(plan)
	// Two passes: the first finds what is cited and defined nowhere, the second
	// stubs it in its place (MUS-D-0169).
	milestones, renumber, err := linkctrl.Milestones(ms, nil, today)
	if err != nil {
		return err
	}
	cited := linkctrl.Unplaced(append(sources[:len(sources):len(sources)], linkctrl.Source{Records: milestones}), renumber)
	milestones, renumber, err = linkctrl.Milestones(ms, cited, today)
	if err != nil {
		return err
	}
	fmt.Printf("stubbed %d milestone(s) cited and defined nowhere\n", len(cited))
	sources = append(sources, linkctrl.Source{Path: "docs/build-notes/phase-details/", Records: milestones})

	rewritten, unresolved := linkctrl.Rewrite(sources, renumber)
	var left []string
	leftTotal := 0
	for tok, n := range unresolved {
		left = append(left, fmt.Sprintf("%s×%d", tok, n))
		leftTotal += n
	}
	sort.Strings(left)
	fmt.Printf("renumbered %d milestones; rewrote %d references; left %d as written: %s\n",
		len(renumber), rewritten, leftTotal, strings.Join(left, " "))
	fmt.Printf("%d citation(s) became refs\n", linkctrl.Cite(sources))

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
	if *apply && *repair {
		return fmt.Errorf("--apply and --repair are two different runs: the first writes an empty import, the second corrects one")
	}
	if !*apply && !*repair {
		fmt.Println("counted, nothing written: pass --apply to write, or --repair to correct an import")
		return nil
	}
	s, ctx, err := openStore(*db)
	if err != nil {
		return err
	}
	defer s.Close()
	if *repair {
		amended, skipped, missing, err := linkctrl.Repair(ctx, s, sources)
		fmt.Printf("amended %d record(s)\n", len(amended))
		if len(amended) > 0 {
			fmt.Printf("  %s\n", strings.Join(amended, " "))
		}
		if len(skipped) > 0 {
			fmt.Printf("left %d written since the import: %s\n", len(skipped), strings.Join(skipped, " "))
		}
		if len(missing) > 0 {
			fmt.Printf("%d not in the store: %s\n", len(missing), strings.Join(missing, " "))
		}
		return err
	}
	n, err := linkctrl.Apply(ctx, s, sources)
	if err != nil {
		return err
	}
	fmt.Printf("imported %d record(s) into %s\n", n, *db)
	return nil
}
