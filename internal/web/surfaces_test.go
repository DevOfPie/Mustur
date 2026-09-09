package web

// The gate that stops a surface being built before it is drawn.
//
// docs/ui-surfaces.md has asked for that in prose since milestone 2c and was
// ignored seven times, twice after the owner had answered on the same subject,
// with the rate going up rather than down (MUS-F-0027). Its own diagnosis is
// that a record read after the fact is not a safeguard. The owner's answer was
// to make the gate enforce it (MUS-Q-0061), which is the same shape as the rule
// about unsurfaced questions: that one holds because `make check` refuses, not
// because a file asks.
//
// So: every page the web package serves must be named by a surface in
// docs/ui-surfaces.md, and every path that file claims to serve must actually
// be served. A new page with no brief fails here, at the commit, rather than
// being recorded afterwards as the eighth instance.
//
// **What this cannot see.** A surface is recognised by its route being
// registered with an explicit `GET` method, which is the house style in this
// package. A page mounted method-less — as `cmd/mustur/main.go` mounts the
// intake fallback, `/mcp` and `/healthz` — is invisible to this, and so is
// anything served outside internal/web. That is a real limit and is written
// down rather than left for somebody to discover.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// notAPage is every reason a registered GET is not a surface.
//
// Small and named on purpose: a list nobody reads is how an exception becomes a
// way of hiding a page from the gate. Each one has to match something, and the
// test fails if it stops — a stale exclusion is a hole that looks like a rule.
var notAPage = []struct {
	why   string
	match func(path string) bool
}{
	{"a script is not a surface", func(p string) bool { return strings.HasPrefix(p, "/assets/") }},
	{"a socket is not a page", func(p string) bool { return strings.HasSuffix(p, "/ws") }},
	{"image bytes, not a page", func(p string) bool { return p == "/records/image/{id}" }},
	// A number, for the badge every surface carries (MUS-Q-0078). Matched
	// exactly rather than by prefix: this is one endpoint behind one control,
	// and a rule that covered /questions/* would let a page through.
	{"a count is not a page", func(p string) bool { return p == "/questions/count" }},
}

// served reads the routes the package actually registers.
//
// Parsed rather than grepped: a string literal in an argument list is exactly
// what go/ast reports, where a regular expression over source would also find
// one in a comment or a test fixture.
func served(t *testing.T, dir string) []string {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}

	var out []string
	for _, pkg := range pkgs {
		ast.Inspect(pkg, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			pattern, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			// Only a GET is a page. A POST is something a surface does.
			if after, found := strings.CutPrefix(pattern, "GET "); found {
				out = append(out, after)
			}
			return true
		})
	}
	sort.Strings(out)
	return out
}

var servesLine = regexp.MustCompile("`(/[^`]*)`")

// briefed reads the paths docs/ui-surfaces.md says are served.
func briefed(t *testing.T) map[string]string {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "ui-surfaces.md"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	surface := "(before any heading)"
	for _, line := range strings.Split(string(doc), "\n") {
		if strings.HasPrefix(line, "### ") {
			surface = strings.TrimPrefix(line, "### ")
			continue
		}
		if !strings.HasPrefix(line, "**Serves**") {
			continue
		}
		for _, m := range servesLine.FindAllStringSubmatch(line, -1) {
			out[m[1]] = surface
		}
	}
	if len(out) == 0 {
		t.Fatal("docs/ui-surfaces.md names no served path at all; the gate would pass on anything")
	}
	return out
}

// Every page this package serves is named by a surface that was briefed first.
func TestEveryServedPageIsABriefedSurface(t *testing.T) {
	brief := briefed(t)
	hit := make([]int, len(notAPage))

	var pages []string
	for _, path := range served(t, ".") {
		excluded := false
		for i, rule := range notAPage {
			if rule.match(path) {
				hit[i]++
				excluded = true
				break
			}
		}
		if !excluded {
			pages = append(pages, path)
		}
	}

	for _, path := range pages {
		if _, ok := brief[path]; !ok {
			t.Errorf("%s is served and no surface in docs/ui-surfaces.md names it.\n"+
				"A surface is briefed and drawn before it is built; this is the eighth instance of "+
				"doing it the other way round (MUS-F-0027). Add it there, with a **Serves** line, "+
				"and take its layout to the owner as a published plan.", path)
		}
	}

	// An exclusion that matches nothing is a hole in the shape of a rule.
	for i, rule := range notAPage {
		if hit[i] == 0 {
			t.Errorf("the %q exclusion matched no route; it is stale and should go", rule.why)
		}
	}
}

// And the other direction: the file cannot claim a surface that is not there.
//
// Without this the gate is satisfied by writing a path down, which is the
// failure it exists to stop wearing the opposite costume.
func TestEveryBriefedPathIsActuallyServed(t *testing.T) {
	live := map[string]bool{}
	for _, path := range served(t, ".") {
		live[path] = true
	}
	for path, surface := range briefed(t) {
		if !live[path] {
			t.Errorf("docs/ui-surfaces.md has %q serving %s, and nothing registers it", surface, path)
		}
	}
}
