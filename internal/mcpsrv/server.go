// Package mcpsrv serves Mustur's records and routing over MCP.
//
// One tool, `mustur_route`, on a server named `mustur`. Both names, and the
// two arguments the tool started with, are the ones the milestone 1 disproof
// scored, so the clause a repository commits is the clause that was measured
// rather than a reworded descendant of it.
package mcpsrv

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/DevOfPie/Mustur/internal/export"
	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/status"
	"github.com/DevOfPie/Mustur/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is reported to clients as the server's implementation version.
const Version = "0.1.0"

// Args are the tool's arguments. `repository` and `task` are the pair the
// disproof measured; `id` and `kind` narrow what comes back and are optional,
// so a call written against the stub still works.
type Args struct {
	Repository string `json:"repository" jsonschema:"repository name as understood from the checkout"`
	Task       string `json:"task,omitempty" jsonschema:"one line on what this session is about"`
	ID         string `json:"id,omitempty" jsonschema:"return one record by identifier, for example MUS-D-0001"`
	// The kind list here is a struct tag and cannot be built at run time, so
	// TestSchemaListsEveryKind asserts it against ident.KindNames rather than
	// leaving it to go stale the next time a role letter is added.
	Kind string `json:"kind,omitempty" jsonschema:"list one kind across every project instead of this repository's index: phase, milestone, work-unit, question, decision, finding, investigation, repository, machine, project"`
}

// Server answers tool calls out of a store.
type Server struct {
	store *store.Store
}

// New builds the MCP server.
func New(s *store.Store) *mcp.Server {
	srv := &Server{store: s}
	server := mcp.NewServer(&mcp.Implementation{Name: "mustur", Version: Version}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name: "mustur_route",
		Description: "Where this repository's records and routing live, and what they say. " +
			"Call with no identifier for the routing and an index of the records of the project " +
			"holding this repository; with a kind for every record of that kind in any project; " +
			"with an identifier for that record in full, whichever project holds it.",
	}, srv.route)
	return server
}

// Handler serves the MCP server over HTTP. Each request gets the same server;
// the store behind it is shared and read-only on this path.
func Handler(s *store.Store) http.Handler {
	server := New(s)
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
}

func (s *Server) route(ctx context.Context, _ *mcp.CallToolRequest, args Args) (*mcp.CallToolResult, any, error) {
	text, err := s.answer(ctx, args)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

// answer is the whole of what the tool returns, as markdown. Split out so it
// can be tested without a transport.
func (s *Server) answer(ctx context.Context, args Args) (string, error) {
	if strings.TrimSpace(args.Repository) == "" {
		return "", fmt.Errorf("mustur_route needs the repository name as understood from the checkout")
	}
	if args.ID != "" {
		return s.one(ctx, args)
	}
	return s.index(ctx, args)
}

func (s *Server) one(ctx context.Context, args Args) (string, error) {
	if !ident.Valid(args.ID) {
		return fmt.Sprintf("%q is not an identifier. They are shaped PROJECT-ROLE-SERIAL, for example MUS-D-0001.\n", args.ID), nil
	}
	r, err := s.store.Get(ctx, args.ID)
	if err != nil {
		// A missing record is an answer, not a failure: an empty result reads
		// as "nothing to say about it", which is a different claim.
		return fmt.Sprintf("Mustur holds no record %s. Call again without an identifier for this repository's index, or with a kind for that kind in every project.\n", args.ID), nil
	}
	return export.One(r), nil
}

func (s *Server) index(ctx context.Context, args Args) (string, error) {
	all, err := s.store.List(ctx, "")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Mustur: %s\n\n", args.Repository)
	if task := strings.TrimSpace(args.Task); task != "" {
		fmt.Fprintf(&b, "Session: %s\n\n", task)
	}

	if args.Kind == "" {
		b.WriteString("## Routing\n\n")
		routing := filter(all, "repository", "machine", "project")
		if len(routing) == 0 {
			b.WriteString("Mustur holds no routing yet.\n")
		}
		for _, r := range routing {
			// Under `## Routing`, not beside it.
			b.WriteString(export.OneUnder(r, 3))
			b.WriteString("\n")
		}
	}

	kinds := ident.KindNames()
	if args.Kind != "" {
		if _, ok := ident.RoleFor(args.Kind); !ok {
			return fmt.Sprintf("Mustur has no record kind %q. The kinds are: %s.\n", args.Kind, strings.Join(kinds, ", ")), nil
		}
		kinds = []string{args.Kind}
		// A kind reaches every project: the owner's answer on MUS-Q-0139 names
		// an identifier or a kind as the way to another project's records.
		b.WriteString("## Records\n\n")
	} else {
		// No kind: the index is the named repository's project, and nothing
		// else (MUS-F-0149, MUS-D-0187). A name that resolves to no project,
		// or a bare name matching two repositories, returns the routing and
		// the registered repositories and no index, on the owner's answer to
		// MUS-Q-0147 — never the whole store, which is the flood this exists
		// to stop.
		project, why := projectFor(all, args.Repository)
		if project.ID == "" {
			b.WriteString("## Records\n\n")
			b.WriteString(why)
			b.WriteString("\n\nCall mustur_route again with `id` set to an identifier for that record in full, " +
				"or with `kind` for every record of that kind in any project.\n")
			return b.String(), nil
		}
		prefix, _ := project.Get("Prefix")
		prefix = strings.TrimSpace(prefix)
		all = ofPrefix(all, prefix)
		fmt.Fprintf(&b, "## Records of %s (%s)\n\n", strings.TrimSpace(project.Title), prefix)
		b.WriteString("Only this project's records are listed. Another project's are reached with `id`, " +
			"or with `kind`, which lists that kind across every project.\n\n")
	}

	total := 0
	for _, kind := range kinds {
		rs := filter(all, kind)
		if len(rs) == 0 {
			continue
		}
		total += len(rs)
		fmt.Fprintf(&b, "### %s (%d)\n\n", kind, len(rs))
		for _, r := range rs {
			fmt.Fprintf(&b, "- %s — %s", r.ID, strings.TrimSpace(r.Title))
			// A finding says whether work remains without a second call
			// (MUS-D-0196): its Status word, which its project's list maps to
			// a State.
			if w := status.WordOf(r); r.Kind == "finding" && w != "" {
				fmt.Fprintf(&b, " · %s", w)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	// With nothing listed there is no identifier above to call again with.
	switch {
	case total > 0:
		b.WriteString("Call mustur_route again with `id` set to any identifier above for that record in full.\n")
	case args.Kind != "":
		b.WriteString("Mustur holds no records of that kind.\n")
	default:
		b.WriteString("This project holds no records yet.\n")
	}
	return b.String(), nil
}

// projectFor finds the project holding the repository a session named, or says
// why there is none.
//
// Sessions pass what they understood from the checkout, which has been seen as
// "Mustur", "DevOfPie/Mustur", "Hoard" for a record titled "DevOfPie/hoard",
// a checkout path, an agent worktree inside one, and a remote URL in either
// the https or the scp form. So the match ignores case and a trailing ".git"
// or slash, splits on "/", "\" and ":", and walks the segments from the end:
// at each one it tries the owner/name pair ending there, then the segment as
// a bare name, and stops at the first that answers to a registered
// repository. That reaches the checkout from anywhere below it without
// knowing what the directory holding worktrees is called.
//
// A bare name answering to two repositories is not guessed between, and is
// answered the way a name answering to none is: the registered repositories
// and no index (MUS-D-0187, on the owner's answer to MUS-Q-0147).
func projectFor(all []record.Record, named string) (record.Record, string) {
	var repos []record.Record
	for _, r := range all {
		if r.Kind == "repository" {
			repos = append(repos, r)
		}
	}
	sort.SliceStable(repos, func(i, j int) bool { return repos[i].ID < repos[j].ID })
	var names []string
	for _, r := range repos {
		names = append(names, strings.TrimSpace(r.Title))
	}
	registered := "none"
	if len(names) > 0 {
		registered = strings.Join(names, ", ")
	}

	matched := matchRepo(repos, named)
	switch len(matched) {
	case 0:
		return record.Record{}, fmt.Sprintf("Mustur holds no repository named %q, so no project's index is listed. "+
			"The registered repositories are: %s.", named, registered)
	case 1:
	default:
		var ids []string
		for _, r := range matched {
			ids = append(ids, fmt.Sprintf("%s (%s)", strings.TrimSpace(r.Title), r.ID))
		}
		return record.Record{}, fmt.Sprintf("%q names more than one repository (%s), so no project's index is listed. "+
			"The registered repositories are: %s. Call again with the owner/name.",
			named, strings.Join(ids, ", "), registered)
	}
	repo := matched[0]
	for _, r := range all {
		if r.Kind != "project" {
			continue
		}
		held, _ := r.Get("Repositories")
		prefix, _ := r.Get("Prefix")
		if strings.TrimSpace(prefix) == "" {
			continue
		}
		for _, id := range ident.Cited(held) {
			if id == repo.ID {
				return r, ""
			}
		}
	}
	return record.Record{}, fmt.Sprintf("Repository %s (%s) is registered, but no project with a prefix lists it, "+
		"so no project's index is listed.", strings.TrimSpace(repo.Title), repo.ID)
}

// matchRepo returns the repositories the name answers to: one, none, or more
// than one when the first thing that matched was a bare name several share.
func matchRepo(repos []record.Record, named string) []record.Record {
	segs := repoSegments(named)
	for i := len(segs) - 1; i >= 0; i-- {
		if i > 0 {
			pair := segs[i-1] + "/" + segs[i]
			for _, r := range repos {
				if strings.Join(lastTwo(repoSegments(r.Title)), "/") == pair {
					return []record.Record{r}
				}
			}
		}
		var bare []record.Record
		for _, r := range repos {
			if t := repoSegments(r.Title); len(t) > 0 && t[len(t)-1] == segs[i] {
				bare = append(bare, r)
			}
		}
		if len(bare) > 0 {
			return bare
		}
	}
	return nil
}

// repoSegments lowers the name and splits it into its non-empty segments,
// with a trailing ".git" off the last. ":" separates as "/" does, which is
// what makes git@github.com:owner/name.git read as a path.
func repoSegments(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	f := strings.FieldsFunc(s, func(c rune) bool { return c == '/' || c == '\\' || c == ':' })
	if n := len(f); n > 0 {
		f[n-1] = strings.TrimSuffix(f[n-1], ".git")
		if f[n-1] == "" {
			f = f[:n-1]
		}
	}
	return f
}

func lastTwo(s []string) []string {
	if len(s) > 2 {
		return s[len(s)-2:]
	}
	return s
}

func ofPrefix(rs []record.Record, prefix string) []record.Record {
	var out []record.Record
	for _, r := range rs {
		if strings.HasPrefix(r.ID, prefix+"-") {
			out = append(out, r)
		}
	}
	return out
}

func filter(rs []record.Record, kinds ...string) []record.Record {
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	var out []record.Record
	for _, r := range rs {
		if want[r.Kind] {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
