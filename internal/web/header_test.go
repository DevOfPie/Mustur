package web

// The bar, and the one link beside it.
//
// MUS-D-0041 is owner-set at four tabs. A review found three different bars
// shipping from that one decision: the records surface had dropped Sessions and
// added an Account tab, and that tab 404s on any server started without an
// origin — the exact "unbuilt capability described as existing" the bar's own
// comment warns about. MUS-Q-0052 kept the four and put the account surface in
// the header instead.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/store"
)

// tabbed renders each surface that carries the bar, with the flags a server
// would set.
func tabbed(t *testing.T, showSessions, showAccount bool) map[string]string {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	mux := http.NewServeMux()
	mux.Handle("/intake", (&Intake{Store: st, Project: "MUS", Actor: "test",
		ShowSessions: showSessions, ShowAccount: showAccount}).Handler())
	(&Questions{Store: st, Project: "MUS", Actor: "test",
		ShowSessions: showSessions, ShowAccount: showAccount}).Routes(mux)
	(&Records{Store: st, Project: "MUS",
		ShowSessions: showSessions, ShowAccount: showAccount}).Routes(mux)
	(&Compose{Store: st, Project: "MUS", Actor: "test",
		ShowAccount: showAccount}).Routes(mux)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out := map[string]string{}
	for _, path := range []string{"/intake", "/questions", "/records", "/compose"} {
		res, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body := make([]byte, 1<<20)
		n, _ := res.Body.Read(body)
		for n > 0 {
			out[path] += string(body[:n])
			n, _ = res.Body.Read(body)
		}
		res.Body.Close()
	}
	return out
}

// One decision, one bar. Every surface carrying it offers the same four.
func TestOneBarOnEverySurface(t *testing.T) {
	pages := tabbed(t, true, true)
	for path, body := range pages {
		if path == "/compose" {
			// The composer has a back arrow rather than the bar.
			continue
		}
		for _, tab := range []string{"/sessions", "/questions", "/intake", "/records"} {
			if !strings.Contains(body, `href="`+tab+`"`) {
				t.Errorf("%s does not offer %s; MUS-D-0041 set four tabs", path, tab)
			}
		}
		// And not a fifth. The account surface is a header link (MUS-Q-0052).
		if strings.Contains(body, `<nav>`) {
			nav := body[strings.Index(body, "<nav>"):]
			if end := strings.Index(nav, "</nav>"); end > 0 {
				if strings.Contains(nav[:end], "/account") {
					t.Errorf("%s has an Account tab; MUS-Q-0052 put it in the header", path)
				}
			}
		}
	}
}

// The link exists exactly when the surface does. /account is registered only
// when an origin is configured, so with none there is nothing to link to.
func TestTheAccountLinkAppearsOnlyWhenServed(t *testing.T) {
	for _, on := range []bool{true, false} {
		pages := tabbed(t, true, on)
		for path, body := range pages {
			has := strings.Contains(body, `href="/account"`)
			if has != on {
				t.Errorf("%s: account link present=%v, served=%v", path, has, on)
			}
		}
	}
}

// The Sessions tab is offered only where sessions are served, which is the rule
// the records surface was written without.
func TestSessionsTabFollowsTheFlag(t *testing.T) {
	off := tabbed(t, false, true)
	for path, body := range off {
		if strings.Contains(body, `href="/sessions"`) && path != "/compose" {
			t.Errorf("%s offers a Sessions tab on a server not serving sessions", path)
		}
	}
}

// The shell reaches every surface that carries the bar.
//
// Six templates each held their own copy of these rules and had already
// drifted: one used a 1px border where the rest used 1.4px, three made the page
// a full-height column and three did not, and one had lost the rule that marks
// the current tab. That is how the records surface came to ship a different bar
// from everything else. One constant, asserted here to have arrived.
func TestEveryBarSurfaceUsesTheSharedShell(t *testing.T) {
	pages := tabbed(t, true, true)
	for path, body := range pages {
		for _, want := range []string{
			"min-width: 60rem", // the rail
			"position: fixed",  // the bar is anchored to the viewport, not to the page
			"--shell-bar",      // and the room it takes is given back
			"body::after",      // by a spacer, not by each surface's own padding
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s is missing %q; the shared shell did not reach it", path, want)
			}
		}
	}
}

// No surface may grow its own copy of the bar again.
//
// This is the test that would have caught the original drift, and it reads the
// source rather than the output because that is where a second copy would
// appear. `nav` styling belongs in shell.go and nowhere else.
func TestNoTemplateDeclaresItsOwnNavRules(t *testing.T) {
	for _, file := range []string{
		"records.go", "questions.go", "intake.go",
		"accountpage.go", "sessions.go", "compose.go",
	} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "nav ") || strings.HasPrefix(trimmed, "nav{") {
				t.Errorf("%s:%d declares nav CSS: %q\n"+
					"The bar and the rail live in shell.go; a second copy is how they drifted before.",
					file, i+1, trimmed)
			}
		}
	}
}

// The session view caps its own height and scrolls its output pane.
//
// Its stylesheet already said what it wanted — `#out{flex:1}` and
// `nav{margin-top:auto}` describe a capped column — but `min-height` set a
// floor with no ceiling and `#out` had no overflow, so the column grew with the
// output and carried the bar and the composer off the screen (MUS-F-0032).
func TestTheSessionShellIsCappedAndScrollsItsOutput(t *testing.T) {
	src, err := os.ReadFile("sessions.go")
	if err != nil {
		t.Fatal(err)
	}
	css := string(src)
	if !strings.Contains(css, "height: 100dvh") {
		t.Error("the session shell has no height cap, so it will grow with its output")
	}
	if strings.Contains(css, "min-height: 100vh; }") {
		t.Error("the session body still sets only a floor; a floor is what let it grow")
	}
	// Read the #out rule itself rather than the whole stylesheet. The first
	// version of this looked for "overflow-y: auto" anywhere in the file and
	// passed against the sub-agent box, which has carried that declaration all
	// along — a true substring proving nothing about the pane under test.
	start := strings.Index(css, "#out {")
	if start < 0 {
		t.Fatal("no #out rule in the session stylesheet")
	}
	end := strings.Index(css[start:], "}")
	if end < 0 {
		t.Fatal("the #out rule is unterminated")
	}
	rule := css[start : start+end]
	for _, want := range []string{"min-height: 0", "overflow-y: auto"} {
		if !strings.Contains(rule, want) {
			t.Errorf("the #out rule is missing %q, so it expands instead of scrolling:\n%s", want, rule)
		}
	}
}
