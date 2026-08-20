// Command mustur holds a project's records and routing, and serves them to a
// session that is told to ask.
//
// Nothing here talks to an agent process. This binary is the store, the
// export, and the one tool call the injection kit mandates; sessions arrive at
// a later milestone.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/audit"
	"github.com/DevOfPie/Mustur/internal/export"
	"github.com/DevOfPie/Mustur/internal/mcpsrv"
	"github.com/DevOfPie/Mustur/internal/seed"
	"github.com/DevOfPie/Mustur/internal/store"
	"github.com/DevOfPie/Mustur/internal/verify"
)

const usage = `mustur — records and routing for one project

  mustur seed     [--db PATH]                 put what already exists into an empty store
  mustur export   [--db PATH] [--out DIR]     render the store as markdown
  mustur verify   [--db PATH] [--records DIR] check the exported tree against itself, and against the store
  mustur serve    [--db PATH] [--addr HOST]   serve the one tool call over MCP
  mustur list     [--db PATH] [--kind KIND]   every record, by identifier
  mustur get ID   [--db PATH]                 one record in full (either order)
  mustur rebuild  [--db PATH]                 re-derive the materialized latest from the log
  mustur audit    [--root DIR] [--catalog DIR] check this tree against the modules it adopts
  mustur version

The store defaults to $MUSTUR_DB, then to $XDG_DATA_HOME/mustur/mustur.db,
then to ~/.local/share/mustur/mustur.db.

The audit's module catalog defaults to $MUSTUR_STRUCGU, then to a StrucGu
checkout beside the audited tree. --format markdown renders the record form.
--gate exits non-zero on findings, and is off by default, deliberately.
`

// version is the binary's own version.
const version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "mustur: %v\n", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	if len(argv) == 0 {
		fmt.Print(usage)
		return nil
	}
	cmd, args := argv[0], argv[1:]
	switch cmd {
	case "seed":
		return cmdSeed(args)
	case "export":
		return cmdExport(args)
	case "verify":
		return cmdVerify(args)
	case "serve":
		return cmdServe(args)
	case "list":
		return cmdList(args)
	case "get":
		return cmdGet(args)
	case "rebuild":
		return cmdRebuild(args)
	case "audit":
		return cmdAudit(args)
	case "version", "--version", "-version":
		fmt.Println(version)
		return nil
	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil
	default:
		fmt.Print(usage)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// defaultCatalog is where the modules are read from. StrucGu is a separate
// repository and nothing vendors it here: a pinned copy of somebody else's
// specification goes stale silently.
//
// A catalog holding a version other than the one pinned is not refused — the
// checker evaluates the module as it reads it, and the mismatch comes back as
// a notice pointing at that module's changelog. An earlier draft of this
// comment said the opposite, describing a refusal that two commits earlier had
// been removed for being the wrong reading of the rule.
func defaultCatalog(root string) string {
	if p := os.Getenv("MUSTUR_STRUCGU"); p != "" {
		return p
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "../StrucGu"
	}
	return filepath.Join(filepath.Dir(abs), "StrucGu")
}

func cmdAudit(args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	root := fs.String("root", ".", "the tree to audit; it holds the adoption record")
	catalog := fs.String("catalog", "", "a StrucGu checkout holding the modules")
	format := fs.String("format", "text", "text or markdown")
	gate := fs.Bool("gate", false, "exit non-zero when there are findings")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *catalog == "" {
		*catalog = defaultCatalog(*root)
	}
	cat, err := audit.LoadCatalog(*catalog)
	if err != nil {
		return fmt.Errorf("%w\nPoint --catalog at a StrucGu checkout, or set MUSTUR_STRUCGU", err)
	}
	report, err := audit.Run(*root, cat, time.Now())
	if err != nil {
		return err
	}
	switch *format {
	case "text":
		err = report.Text(os.Stdout)
	case "markdown":
		err = report.Markdown(os.Stdout)
	default:
		return fmt.Errorf("format %q is not text or markdown", *format)
	}
	if err != nil {
		return err
	}
	// Exit zero when the audit ran, non-zero only when it could not. Findings
	// are output, not failure: a check that fails on day one in a repository
	// with required status checks is made non-required within the hour, and a
	// dead gate is worse than no gate because it looks like coverage. A
	// consumer who wants to gate asks for it.
	if *gate && report.Findings() > 0 {
		return fmt.Errorf("%d finding(s), and --gate was asked for", report.Findings())
	}
	return nil
}

// defaultDB is where the store lives when nothing says otherwise. It is not in
// the repository: the database is the record, and a binary file in git is a
// record nobody can review.
func defaultDB() string {
	if p := os.Getenv("MUSTUR_DB"); p != "" {
		return p
	}
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "mustur.db"
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "mustur", "mustur.db")
}

func openStore(path string) (*store.Store, context.Context, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}
	ctx := context.Background()
	s, err := store.Open(ctx, path)
	return s, ctx, err
}

func dbFlag(fs *flag.FlagSet) *string {
	return fs.String("db", defaultDB(), "path to the store")
}

func cmdSeed(args []string) error {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	db := dbFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, ctx, err := openStore(*db)
	if err != nil {
		return err
	}
	defer s.Close()
	n, err := seed.Apply(ctx, s)
	if err != nil {
		return err
	}
	fmt.Printf("seeded %d record(s) into %s\n", n, *db)
	return nil
}

func cmdExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	db := dbFlag(fs)
	out := fs.String("out", "records", "directory to render into")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, ctx, err := openStore(*db)
	if err != nil {
		return err
	}
	defer s.Close()
	records, err := s.List(ctx, "")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	if err := export.Write(*out, records); err != nil {
		return err
	}
	fmt.Printf("exported %d record(s) to %s\n", len(records), *out)
	return nil
}

func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	db := fs.String("db", "", "compare the tree against this store as well (optional)")
	dir := fs.String("records", "records", "the exported tree to check")
	if err := fs.Parse(args); err != nil {
		return err
	}
	problems, checked, err := verify.Tree(*dir)
	if err != nil {
		return err
	}
	if *db != "" {
		s, ctx, err := openStore(*db)
		if err != nil {
			return err
		}
		defer s.Close()
		records, err := s.List(ctx, "")
		if err != nil {
			return err
		}
		drift, err := verify.AgainstStore(*dir, records)
		if err != nil {
			return err
		}
		problems = append(problems, drift...)
	}
	for _, p := range problems {
		fmt.Printf("  FAIL  %s\n", p)
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d problem(s) in %s", len(problems), *dir)
	}
	fmt.Printf("  ok    %d identifier(s) in %s, every citation resolves\n", checked, *dir)
	return nil
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	db := dbFlag(fs)
	// Loopback only. The server is unauthenticated, which is sound while
	// nothing but this machine can reach it and stops being sound the day the
	// ingress rule exists.
	addr := fs.String("addr", "127.0.0.1:7777", "address to listen on; loopback only until identity is in front of it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := loopbackOnly(*addr); err != nil {
		return err
	}
	s, ctx, err := openStore(*db)
	if err != nil {
		return err
	}
	defer s.Close()
	n, err := s.Count(ctx)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpsrv.Handler(s))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "ok %d record(s)\n", n)
	})
	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	fmt.Printf("mustur %s serving %d record(s) from %s on http://%s/mcp\n", version, n, *db, *addr)
	return srv.ListenAndServe()
}

// loopbackOnly refuses an address that is not on the loopback interface. The
// check is a guard rail, not security: it exists so that binding to the world
// is a deliberate change to this file rather than a flag someone passes once.
func loopbackOnly(addr string) error {
	host := addr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		host = addr[:i]
	}
	switch host {
	case "127.0.0.1", "localhost", "[::1]", "::1":
		return nil
	}
	return fmt.Errorf("address %q is not loopback: nothing authenticates this server yet", addr)
}

func cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	db := dbFlag(fs)
	kind := fs.String("kind", "", "limit to one kind")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, ctx, err := openStore(*db)
	if err != nil {
		return err
	}
	defer s.Close()
	records, err := s.List(ctx, *kind)
	if err != nil {
		return err
	}
	for _, r := range records {
		fmt.Printf("%s  %-14s %s\n", r.ID, r.Kind, r.Title)
	}
	return nil
}

func cmdGet(args []string) error {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	db := dbFlag(fs)
	// Go's flag package stops at the first non-flag argument, so `get ID --db P`
	// parses the identifier and then leaves the flag unread. Parsing twice makes
	// both orders mean the same thing, which is what anyone typing it expects.
	// It cannot mistake a flag's value for the identifier, because the first
	// parse is the one that consumes flag values.
	if err := fs.Parse(args); err != nil {
		return err
	}
	id := ""
	switch {
	case fs.NArg() == 1:
		id = fs.Arg(0)
	case fs.NArg() > 1:
		id = fs.Arg(0)
		if err := fs.Parse(fs.Args()[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return fmt.Errorf("get needs exactly one identifier")
		}
	default:
		return fmt.Errorf("get needs exactly one identifier")
	}
	s, ctx, err := openStore(*db)
	if err != nil {
		return err
	}
	defer s.Close()
	r, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	fmt.Print(export.One(r))
	return nil
}

func cmdRebuild(args []string) error {
	fs := flag.NewFlagSet("rebuild", flag.ContinueOnError)
	db := dbFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, ctx, err := openStore(*db)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.Rebuild(ctx); err != nil {
		return err
	}
	n, err := s.Count(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("rebuilt %d record(s) from the log\n", n)
	return nil
}
