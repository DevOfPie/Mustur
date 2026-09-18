package tmuxtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// reachesTmux is how a test file was found to reach tmux when MUS-F-0161 was
// reviewed on PR 91: it execs tmux by name, or builds a session.Adapter — whose
// zero Runner shells out to plain tmux — or calls session.New.
var reachesTmux = regexp.MustCompile(`"tmux"|Adapter\{|session\.New\b`)

// Every package whose tests can reach tmux must route its test binary through
// Main, or a plain `go test ./...` reaches the owner's server again. Nothing
// else notices a missing TestMain: the tests pass either way, on the live
// socket (PR 91 review, finding 1).
func TestEveryPackageThatReachesTmuxCallsMain(t *testing.T) {
	root := moduleRoot(t)
	reaches := map[string]string{} // dir -> first file that reaches tmux
	guarded := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor") {
				return filepath.SkipDir
			}
			// A nested module (a checkout CI fetches) is not this module's.
			if path != root {
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		dir := filepath.Dir(path)
		if reachesTmux.Match(src) {
			if _, ok := reaches[dir]; !ok {
				reaches[dir] = filepath.Base(path)
			}
		}
		if callsMain(t, path, src) {
			guarded[dir] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(reaches) == 0 {
		t.Fatalf("no test package under %s reaches tmux; the walk is looking in the wrong place", root)
	}
	var missing []string
	for dir, file := range reaches {
		if !guarded[dir] {
			rel, _ := filepath.Rel(root, dir)
			missing = append(missing, rel+" ("+file+")")
		}
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("package %s reaches tmux in its tests but has no TestMain calling tmuxtest.Main (MUS-F-0161)", m)
	}
}

// callsMain reports whether the file declares a TestMain whose body calls
// tmuxtest.Main — or Main, inside this package.
func callsMain(t *testing.T, path string, src []byte) bool {
	f, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != "TestMain" || fn.Body == nil {
			continue
		}
		found := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				if x, ok := fun.X.(*ast.Ident); ok && x.Name == "tmuxtest" && fun.Sel.Name == "Main" {
					found = true
				}
			case *ast.Ident:
				if fun.Name == "Main" && f.Name.Name == "tmuxtest" {
					found = true
				}
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

func moduleRoot(t *testing.T) string {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test's directory")
		}
		dir = parent
	}
}
