package mcpsrv

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The milestone's claim is that one tool call returns the records and the
// routing. This exercises it the way a session does: over HTTP, through the
// protocol, with the tool discovered rather than called directly.
func TestToolIsReachableOverHTTP(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, r := range fixtures() {
		if err := s.Append(ctx, r, "create", "test"); err != nil {
			t.Fatal(err)
		}
	}

	srv := httptest.NewServer(Handler(s))
	defer srv.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: srv.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "mustur_route" {
		t.Fatalf("tools offered: %+v", tools.Tools)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "mustur_route",
		Arguments: map[string]any{"repository": "Mustur", "task": "checking the transport"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool returned an error: %+v", res.Content)
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content was %T", res.Content[0])
	}
	if !strings.Contains(text.Text, "MUS-D-0001") || !strings.Contains(text.Text, "## Routing") {
		t.Errorf("call returned:\n%s", text.Text)
	}
}
