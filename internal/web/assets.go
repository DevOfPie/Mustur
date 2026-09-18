package web

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"sort"
)

// Every script a page loads is named by its content, not only its path
// (MUS-F-0180).
//
// The scripts were served `Cache-Control: no-cache`, which asks every cache to
// revalidate, and after a deploy browsers still ran a bar.js from before it: no
// request for any /assets/ path reached the origin for an hour. Something in
// front of Mustur answered — Cloudflare's edge caches .js by extension, and
// was not confirmed only because Access redirects an unauthenticated check. The
// fix does not depend on which cache it was. A page names
// /assets/bar.js?v=<hash of the bytes>, so a deploy that changes the bytes
// changes the URL, and no cache anywhere holds an answer for a URL it has never
// seen.
//
// Once the URL names the bytes, the answer to it can be cached for good, and
// is: a request whose v matches is told immutable. A request with no v, or with
// a v from an earlier deploy, gets the current bytes and no-cache, never a 404
// — a page rendered before the deploy and still open in a tab is asking for
// the only script there is.

// assetVersionLen is how many hex digits of the SHA-256 name a version. Twelve
// is 48 bits, which is plenty for telling one deploy's script from another's.
const assetVersionLen = 12

// An asset is one embedded file and the version its bytes hash to.
type asset struct {
	body        string
	contentType string
	version     string
}

// assetVersion is the name a file's bytes are served under.
func assetVersion(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])[:assetVersionLen]
}

// newAssets hashes every file once. Separate from the table so a test can
// build one from bytes of its own.
func newAssets(files map[string]string) map[string]asset {
	out := make(map[string]asset, len(files))
	for name, body := range files {
		out[name] = asset{
			body:        body,
			contentType: "application/javascript; charset=utf-8",
			version:     assetVersion(body),
		}
	}
	return out
}

// assets is every file served under /assets/, hashed at startup. A script
// embedded anywhere in this package is listed here or it cannot be named by a
// template: assetURL panics on a name it does not know, which a test rendering
// the page finds before a browser does.
var assets = newAssets(map[string]string{
	"account.js": accountJS,
	"auth.js":    authJS,
	"bar.js":     barJS,
	"compose.js": composeJS,
	"intake.js":  intakeJS,
	"session.js": sessionJS,
})

// assetNames is the table's names in order, for tests that walk it.
func assetNames() []string {
	names := make([]string, 0, len(assets))
	for name := range assets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// assetURL is the one place an asset's URL is built. Templates reach it as
// {{asset "bar.js"}}; nothing writes /assets/ into a page by hand.
func assetURL(name string) string {
	a, ok := assets[name]
	if !ok {
		panic(fmt.Sprintf("web: no embedded asset %q", name))
	}
	return "/assets/" + name + "?v=" + a.version
}

// assetFuncs is handed to every page template before it is parsed.
var assetFuncs = template.FuncMap{"asset": assetURL}

// serveAsset writes one embedded file. Registered per file by whichever Routes
// owns it, as before; only the headers are shared.
func serveAsset(name string) http.HandlerFunc {
	a, ok := assets[name]
	if !ok {
		panic(fmt.Sprintf("web: no embedded asset %q", name))
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", a.contentType)
		if r.URL.Query().Get("v") == a.version {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		_, _ = w.Write([]byte(a.body))
	}
}
