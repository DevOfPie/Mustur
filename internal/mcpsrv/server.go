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
	Kind string `json:"kind,omitempty" jsonschema:"limit the index to one kind: milestone, work-unit, question, decision, finding, investigation, repository, machine, project"`
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
			"Call with no identifier for the routing and an index of every record; call with an " +
			"identifier for that record in full.",
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
		return fmt.Sprintf("Mustur holds no record %s. Call again without an identifier for the index of what it does hold.\n", args.ID), nil
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
	}

	b.WriteString("## Records\n\n")
	total := 0
	for _, kind := range kinds {
		rs := filter(all, kind)
		if len(rs) == 0 {
			continue
		}
		total += len(rs)
		fmt.Fprintf(&b, "### %s (%d)\n\n", kind, len(rs))
		for _, r := range rs {
			fmt.Fprintf(&b, "- %s — %s\n", r.ID, strings.TrimSpace(r.Title))
		}
		b.WriteString("\n")
	}
	if total == 0 {
		b.WriteString("Mustur holds no records of that kind.\n\n")
	}
	b.WriteString("Call mustur_route again with `id` set to any identifier above for that record in full.\n")
	return b.String(), nil
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
