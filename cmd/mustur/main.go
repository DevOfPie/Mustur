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
  mustur version

The store defaults to $MUSTUR_DB, then to $XDG_DATA_HOME/mustur/mustur.db,
then to ~/.local/share/mustur/mustur.db.
`

// version is the binary's own version. Milestone 2's shape, nothing served to
// a session yet.
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
