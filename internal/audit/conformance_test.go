package audit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// StrucGu ships a fixture tree per check, and an expected.yaml saying what a
// correct checker produces on each. That file is the specification's own
// statement of where the boundaries are, so running against it is the only
// evidence available that this implementation is a conforming one rather than
// merely a working one — the specification says an ambiguity between two
// correct-looking checkers is a missing fixture, which makes the fixtures the
// arbiter.
//
// Skipped when there is no StrucGu checkout beside this repository. Nothing
// here vendors a copy: a pinned copy of somebody else's specification goes
// stale silently, and a conformance test against a stale copy proves conformance
// to nothing.

type expectations struct {
	Schema   string                       `yaml:"schema"`
	Checks   map[string]map[string]string `yaml:"checks"`
	Judgment map[string]string            `yaml:"judgment"`
}

func TestConformsToTheCatalogFixtures(t *testing.T) {
	catalogRoot := catalogPath(t)
	cat, err := LoadCatalog(catalogRoot)
	if err != nil {
		t.Fatal(err)
	}
	modules, err := filepath.Glob(filepath.Join(catalogRoot, "modules", "*", "fixtures", "expected.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) == 0 {
		t.Fatalf("no fixture expectations under %s", catalogRoot)
	}
	sort.Strings(modules)

	trees, compared := 0, 0
	for _, path := range modules {
		module := filepath.Base(filepath.Dir(filepath.Dir(path)))
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var want expectations
		if err := yaml.Unmarshal(b, &want); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		fixtures := filepath.Dir(path)

		for _, tree := range sortedKeys(want.Checks) {
			root := filepath.Join(fixtures, tree)
			if _, err := os.Stat(root); err != nil {
				t.Errorf("%s expects a tree %s that is not there", module, tree)
				continue
			}
			trees++
			t.Run(module+"/"+tree, func(t *testing.T) {
				root := prepare(t, root)
				rep, err := Run(root, cat, time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC))
				if err != nil {
					t.Fatalf("the audit could not run: %v", err)
				}
				got := map[string]State{}
				for _, res := range rep.Results {
					got[res.ID] = res.State
				}
				for _, id := range sortedKeys(want.Checks[tree]) {
					compared++
					if got[id] != State(want.Checks[tree][id]) {
						t.Errorf("%s = %q, want %q", id, got[id], want.Checks[tree][id])
					}
				}
				// One judgment line per entry on every tree, whether or not
				// anyone is available to judge it. The state does not vary by
				// tree; only the annotation does.
				for _, id := range sortedKeys(want.Judgment) {
					compared++
					if got[id] != State(want.Judgment[id]) {
						t.Errorf("%s = %q, want %q", id, got[id], want.Judgment[id])
					}
				}
			})
		}
	}
	t.Log(fmt.Sprintf("%d fixture trees, %d expected states compared", trees, compared))
}

var setupBlock = regexp.MustCompile("(?s)```sh\n(.*?)```")

// prepare copies a fixture that is not self-contained and runs the shell block
// in its SETUP.md, which is how the one history fixture gets a history — a
// fixture directory cannot carry a nested repository. StrucGu records that as a
// known gap rather than papering over it, and every other tree is read as it
// stands.
//
// The copy is the point: the catalog is somebody else's checkout and an audit
// writes nothing to what it audits, so a fixture that needs a git repository
// gets one somewhere else.
func prepare(t *testing.T, tree string) string {
	t.Helper()
	setupPath := filepath.Join(tree, "SETUP.md")
	setup, err := os.ReadFile(setupPath)
	if os.IsNotExist(err) {
		return tree
	}
	if err != nil {
		t.Fatal(err)
	}
	block := setupBlock.FindSubmatch(setup)
	if block == nil {
		t.Fatalf("%s has no shell block to run", setupPath)
	}

	scratch := t.TempDir()
	target := filepath.Join(scratch, filepath.Base(tree))
	if out, err := exec.Command("cp", "-r", tree, target).CombinedOutput(); err != nil {
		t.Fatalf("copy fixture: %v %s", err, out)
	}
	if err := os.Remove(filepath.Join(target, "SETUP.md")); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", "-c", "set -e\ngit config --global init.defaultBranch main\n"+string(block[1]))
	cmd.Dir = target
	cmd.Env = append(os.Environ(),
		"HOME="+scratch,
		"GIT_CONFIG_GLOBAL="+filepath.Join(scratch, "gitconfig"),
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.org",
		"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.org",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("setup for %s failed: %v\n%s", filepath.Base(tree), err, out)
	}
	return target
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// A fixture directory that no expectation names is a tree nothing asserts
// about, which is the same as an unpinned boundary.
func TestEveryFixtureTreeIsExpectedSomewhere(t *testing.T) {
	catalogRoot := catalogPath(t)
	paths, err := filepath.Glob(filepath.Join(catalogRoot, "modules", "*", "fixtures", "expected.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var want expectations
		if err := yaml.Unmarshal(b, &want); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if e.Name() == ".git" {
				continue
			}
			if _, ok := want.Checks[e.Name()]; !ok {
				t.Errorf("%s holds a tree %s that %s does not expect anything of",
					filepath.Dir(path), e.Name(), filepath.Base(path))
			}
		}
		if !strings.HasPrefix(want.Schema, "strucgu/expected@") {
			t.Errorf("%s declares schema %q", path, want.Schema)
		}
	}
}
