// Command mustur holds a project's records and routing, and serves them to a
// session that is told to ask.
//
// Nothing here talks to an agent process. This binary is the store, the
// export, and the one tool call the injection kit mandates; sessions arrive at
// a later milestone.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/audit"
	"github.com/DevOfPie/Mustur/internal/export"
	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/mcpsrv"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/session"
	"github.com/DevOfPie/Mustur/internal/seed"
	"github.com/DevOfPie/Mustur/internal/store"
	"github.com/DevOfPie/Mustur/internal/verify"
	"github.com/DevOfPie/Mustur/internal/web"
)

const usage = `mustur — records and routing for one project

  mustur seed     [--db PATH]                 put what already exists into an empty store
  mustur export   [--db PATH] [--out DIR]     render the store as markdown
  mustur verify   [--db PATH] [--records DIR] check the exported tree against itself, and against the store
  mustur serve    [--db PATH] [--addr HOST]   serve the one tool call over MCP
  mustur list     [--db PATH] [--kind KIND]   every record, by identifier
  mustur get ID   [--db PATH]                 one record in full (either order)
  mustur rebuild  [--db PATH]                 re-derive the materialized latest from the log
  mustur add KIND --title T [...]             write one record into the store
  mustur amend ID --title T [...]             correct one, without losing what it said
  mustur ask      --title T [--blocks W]      raise a question the owner has to answer
                  [--option "L :: line :: detail"]  an answer they can pick, repeatable
                  [--needed]                  the work cannot proceed without the answer
  mustur surfaced ID                          record that it reached a prompt
  mustur answer   ID --answer A               record what the owner said, or --withdraw
  mustur questions [--all] [--gate]           open questions; --gate exits non-zero on buried ones
  mustur session  start P --dir D --cmd C     start a session Mustur owns, inside tmux
                  list | stop P               there is no send: see cmd/mustur/sessions.go
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
	case "add":
		return cmdWrite(args, "create")
	case "amend":
		return cmdWrite(args, "amend")
	case "ask":
		return cmdAsk(args)
	case "surfaced":
		return cmdSurfaced(args)
	case "answer":
		return cmdAnswer(args)
	case "questions":
		return cmdQuestions(args)
	case "session":
		return cmdSession(args)
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

// fields collects repeated k=v flags in the order they were given. Order is
// the author's and the export renders it, so a flag package that sorted them
// would decide how a record reads.
type fields []record.Field

func (f *fields) String() string { return fmt.Sprint(*f) }

func (f *fields) Set(v string) error {
	key, value, ok := strings.Cut(v, "=")
	if !ok || strings.TrimSpace(key) == "" {
		return fmt.Errorf("%q is not key=value", v)
	}
	*f = append(*f, record.Field{Key: key, Value: value})
	return nil
}

// cmdWrite is `add` and `amend`. One function, because the difference between
// them is one word passed to the store — and the store is what refuses a create
// over an existing identifier or an amendment of one that is not there. A
// second code path would be a second place for that rule to be wrong.
func cmdWrite(args []string, op string) error {
	fs := flag.NewFlagSet(op, flag.ContinueOnError)
	db := dbFlag(fs)
	title := fs.String("title", "", "the record's claim, in one line")
	body := fs.String("body", "", "the prose")
	at := fs.String("at", "", "the date the record's content was true (default today)")
	project := fs.String("project", "MUS", "identifier prefix, for a store holding more than one project")
	actor := fs.String("actor", defaultActor(), "who is writing this")
	var data, refs fields
	fs.Var(&data, "data", "a k=value field, repeatable and rendered in order")
	fs.Var(&refs, "ref", "a k=IDENTIFIER citation, repeatable")
	complaint := "add needs one kind: " + strings.Join(kindNames(), ", ")
	if op == "amend" {
		complaint = "amend needs one identifier"
	}
	positional, err := parseWithPositional(fs, args, complaint)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*title) == "" {
		return fmt.Errorf("%s needs a --title: a record with no claim is a row, not a record", op)
	}
	if *at == "" {
		*at = time.Now().Format("2006-01-02")
	}

	s, ctx, err := openStore(*db)
	if err != nil {
		return err
	}
	defer s.Close()

	r := record.Record{Title: *title, Body: *body, At: *at, Data: data, Refs: refs}
	switch op {
	case "create":
		role, ok := ident.RoleFor(positional)
		if !ok {
			return fmt.Errorf("%q is not a record kind: %s", positional, strings.Join(kindNames(), ", "))
		}
		r.Kind = positional
		// Allocation and insertion in one act. Two calls let two writers claim
		// the same serial, and the loser's record was told it was filed.
		written, err := s.Create(ctx, r, *project, role, *actor)
		if err != nil {
			return err
		}
		fmt.Println(written.ID)
		return nil
	case "amend":
		existing, err := s.Get(ctx, positional)
		if err != nil {
			return err
		}
		r.ID, r.Kind = existing.ID, existing.Kind
		// An amendment states the record afresh. Carrying the old fields
		// forward silently would make `amend --title` quietly keep data the
		// writer never saw, and the log holds the earlier version anyway.
	}
	if err := s.Append(ctx, r, op, *actor); err != nil {
		return err
	}
	fmt.Println(r.ID)
	return nil
}

func kindNames() []string {
	var out []string
	for _, role := range ident.Roles {
		out = append(out, role.Name())
	}
	return out
}

// defaultActor names who wrote a record. The log distinguishes what the
// bootstrap imported from what has been written since, so an unattributed
// record would erase the one thing that distinction is for.
func defaultActor() string {
	if who := os.Getenv("MUSTUR_ACTOR"); who != "" {
		return who
	}
	if who := os.Getenv("USER"); who != "" {
		return who
	}
	return "unknown"
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
	// Loopback only. The server is unauthenticated, so whatever publishes it
	// carries the identity: on mustur.devofpie.com that is Cloudflare Access,
	// and it covers /mcp as well as the intake box, both being on this mux.
	addr := fs.String("addr", "127.0.0.1:7777", "address to listen on; loopback only until identity is in front of it")
	project := fs.String("project", "MUS", "identifier prefix for records this server writes")
	// Without this the surface writes the store and nothing else, and the file
	// the findings role is mapped at falls behind every jot filed from a phone.
	exportTo := fs.String("export", "", "render the store into this directory after each filing; empty means do not")
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
	intake := &web.Intake{Store: s, Project: *project, Actor: defaultActor(), ExportTo: *exportTo}
	questions := &web.Questions{
		Store: s, Project: *project, Actor: defaultActor(), ExportTo: *exportTo,
		// An answer typed from a phone is carried into the session that raised
		// it, if that session is still alive and Mustur started it.
		Sessions: &session.Adapter{},
	}
	// Registered on the outer mux, ahead of the intake box's catch-all, so the
	// queue is reachable at a hostname whose "/" belongs to intake.
	questions.Routes(mux)
	mux.Handle("/", intake.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "ok %d record(s)\n", n)
	})
	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	fmt.Printf("mustur %s serving %d record(s) from %s\n  tool call  http://%s/mcp\n  intake     http://%s/intake\n  decisions  http://%s/questions\n",
		version, n, *db, *addr, *addr, *addr)
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
	id, err := parseWithPositional(fs, args, "get needs exactly one identifier")
	if err != nil {
		return err
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

// parseWithPositional parses flags around a single positional argument, in
// either order.
//
// Go's flag package stops at the first non-flag argument, so `get ID --db P`
// reads the identifier and then leaves the flag unread — silently, which is
// the part that makes it a bug rather than an inconvenience. Parsing a second
// time over what the first parse left behind makes both orders mean the same
// thing. It cannot mistake a flag's value for the positional, because the
// first parse is the one that consumes flag values.
func parseWithPositional(fs *flag.FlagSet, args []string, complaint string) (string, error) {
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	switch {
	case fs.NArg() == 1:
		return fs.Arg(0), nil
	case fs.NArg() > 1:
		positional := fs.Arg(0)
		if err := fs.Parse(fs.Args()[1:]); err != nil {
			return "", err
		}
		if fs.NArg() != 0 {
			return "", errors.New(complaint)
		}
		return positional, nil
	}
	return "", errors.New(complaint)
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
