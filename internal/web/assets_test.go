package web

// Script URLs name their bytes (MUS-F-0180).
//
// That every rendered page names the current version is asserted wherever a
// test reads a page's scripts: scriptsIn returns "/assets/bar.js" only for a
// src carrying the version bar.js hashes to now, so the surface tests in
// sessions_test, records_test, questions_test, compose_test, intake_test,
// markdown_test, auth_test and accountpage_test fail on a page that names a
// script without it. What is here is the rest: no template can bypass the
// helper, the table covers every embedded file, and the handler's answers.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// No page names a script by hand. A literal path in a template is the one way
// back to the defect: it renders, it loads, and it is the URL a cache pinned.
func TestNoTemplateNamesAnAssetByHand(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	literal := regexp.MustCompile(`(src|href)=["']?/assets/`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if literal.MatchString(line) {
				t.Errorf("%s:%d names an asset without its version: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// Every file under assets/ is in the table, so every one can be named with a
// version and is served with the same headers.
func TestEveryEmbeddedFileIsVersioned(t *testing.T) {
	entries, err := os.ReadDir("assets")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		a, ok := assets[e.Name()]
		if !ok {
			t.Errorf("assets/%s is embedded but has no version", e.Name())
			continue
		}
		b, err := os.ReadFile(filepath.Join("assets", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if a.body != string(b) {
			t.Errorf("the table's %s is not the file on disk", e.Name())
		}
		if want := assetVersion(string(b)); a.version != want {
			t.Errorf("%s is versioned %s, its bytes hash to %s", e.Name(), a.version, want)
		}
		if got, want := assetURL(e.Name()), "/assets/"+e.Name()+"?v="+a.version; got != want {
			t.Errorf("assetURL(%q) = %s, want %s", e.Name(), got, want)
		}
	}
	if len(entries) != len(assets) {
		t.Errorf("assets/ holds %d files and the table %d: %v", len(entries), len(assets), assetNames())
	}
}

// A deploy that changes a script changes its URL. That is the whole fix.
func TestChangingTheBytesChangesTheVersion(t *testing.T) {
	before := newAssets(map[string]string{"bar.js": "poll('/questions/count')"})
	after := newAssets(map[string]string{"bar.js": "poll('/questions/count'); poll('/records/attention/count')"})
	if before["bar.js"].version == after["bar.js"].version {
		t.Fatal("two different scripts share a version")
	}
	if again := newAssets(map[string]string{"bar.js": "poll('/questions/count')"}); again["bar.js"].version != before["bar.js"].version {
		t.Error("the same bytes hashed to two versions, so every restart would be a new URL")
	}
	if v := before["bar.js"].version; len(v) != assetVersionLen || strings.Trim(v, "0123456789abcdef") != "" {
		t.Errorf("version %q is not %d hex digits", v, assetVersionLen)
	}
}

// The URL a page names may be cached for good; anything else is revalidated,
// and nothing is refused. A tab rendered before a deploy asks with the old v
// and still gets a script — the current one.
func TestTheHandlerCachesOnlyTheCurrentVersion(t *testing.T) {
	mux := http.NewServeMux()
	BarRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	current := assets["bar.js"].version
	for _, c := range []struct {
		name, query, cache string
	}{
		{"the current version", "?v=" + current, "public, max-age=31536000, immutable"},
		{"a stale version", "?v=000000000000", "no-cache"},
		{"no version", "", "no-cache"},
		{"an empty version", "?v=", "no-cache"},
	} {
		res, err := srv.Client().Get(srv.URL + "/assets/bar.js" + c.query)
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s: %d", c.name, res.StatusCode)
		}
		if got := res.Header.Get("Cache-Control"); got != c.cache {
			t.Errorf("%s: Cache-Control %q, want %q", c.name, got, c.cache)
		}
		if got := res.Header.Get("Content-Type"); got != "application/javascript; charset=utf-8" {
			t.Errorf("%s: Content-Type %q", c.name, got)
		}
		if string(b) != barJS {
			t.Errorf("%s: served something other than the current bar.js", c.name)
		}
	}
}
