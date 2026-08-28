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
	"github.com/DevOfPie/Mustur/internal/session"
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
		// The bar is still four. The account entry is in the nav markup, but it
		// is the rail's own footer entry and the stylesheet keeps it out of the
		// bar below the breakpoint — MUS-Q-0052 put the account link in the
		// header, and the owner has since asked for it at the foot of the rail
		// on a wide screen. The bar's four tabs are what that decision was
		// protecting, and they are untouched.
		if strings.Contains(body, `<nav>`) {
			nav := body[strings.Index(body, "<nav>"):]
			if end := strings.Index(nav, "</nav>"); end > 0 {
				inner := nav[:end]
				if strings.Contains(inner, "/account") && !strings.Contains(inner, `class="me`) {
					t.Errorf("%s has a plain Account tab; it belongs at the foot of the rail, not in the row of four", path)
				}
				if n := strings.Count(inner, `<a href="/`); n != 4 {
					t.Errorf("%s has %d plain tabs, want the four from MUS-D-0041", path, n)
				}
			}
		}
		// And it is hidden in the bar, shown at the foot of the rail. Asserted
		// on the stylesheet because there is no layout here to measure.
		if strings.Contains(body, `class="me`) {
			if !strings.Contains(body, "nav a.me { display: none; }") {
				t.Errorf("%s puts the account entry in the bar as a fifth tab", path)
			}
			if !strings.Contains(body, "margin-top: auto") {
				t.Errorf("%s does not sink the account entry to the foot of the rail", path)
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

// The session view's lower half is locked to the bottom of the screen.
//
// The owner watched the quiet timer and the composer walk off the bottom of a
// phone and had to scroll to find them. A column holds its shape only while the
// browser agrees about how tall the viewport is; a fixed element is anchored to
// it and cannot be pushed anywhere.
func TestTheSessionDockIsLockedToTheBottom(t *testing.T) {
	src, err := os.ReadFile("sessions.go")
	if err != nil {
		t.Fatal(err)
	}
	css := string(src)

	// The quiet timer and the composer are one docked block, not two loose
	// children at the end of a column.
	if !strings.Contains(css, `<div class="dock">`) {
		t.Error("the quiet timer and the composer are not docked together")
	}
	start := strings.Index(css, ".dock {")
	if start < 0 {
		t.Fatal("no .dock rule in the session stylesheet")
	}
	rule := css[start : start+strings.Index(css[start:], "}")]
	for _, want := range []string{"position: fixed", "bottom:"} {
		if !strings.Contains(rule, want) {
			t.Errorf("the dock rule is missing %q, so it can be pushed off the screen:\n%s", want, rule)
		}
	}
	// Beside a rail the dock lines up with the reading column. Spanning the
	// viewport ran it underneath the rail and took the quiet timer and the
	// composer's first inch with it.
	if strings.Contains(rule, "left: 0") || strings.Contains(rule, "right: 0") {
		t.Errorf("the dock is pinned to the viewport edges, so the rail covers it:\n%s", rule)
	}
	for _, want := range []string{"--shell-dock-left", "--shell-dock-width"} {
		if !strings.Contains(rule, want) {
			t.Errorf("the dock does not take %q from the shell, so it cannot know where the rail ends:\n%s", want, rule)
		}
	}

	// And the output keeps its tail above the dock rather than under it.
	outAt := strings.Index(css, "#out {")
	outRule := css[outAt : outAt+strings.Index(css[outAt:], "}")]
	if !strings.Contains(outRule, "--dock-h") {
		t.Errorf("#out does not reserve room for the dock, so its last lines sit under it:\n%s", outRule)
	}

	// The script is what measures it, because CSS cannot measure a sibling and
	// the composer changes height as it is typed into.
	js, err := os.ReadFile("assets/session.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), "--dock-h") {
		t.Error("nothing keeps --dock-h current, so the reservation is a guess")
	}
}

// Nothing but the output pane may take room from the chrome rows.
//
// The sub-agent box had no cap and grew to 8,211px on a real session. It is a
// flex item in a column capped at the viewport, so every other row was squeezed
// around it: the rail collapsed to a third of its height and the session chips
// spilled under the strip below, and the composer was pushed off the screen
// (MUS-F-0035).
func TestOnlyTheOutputPaneFlexes(t *testing.T) {
	src, err := os.ReadFile("sessions.go")
	if err != nil {
		t.Fatal(err)
	}
	css := string(src)

	// The sub-agent list is not in this column at all any more — it lives in a
	// drawer that is shut on arrival (MUS-F-0038, MUS-Q-0057). So the rule that
	// used to cap it is gone, and what has to be true instead is that the box
	// it moved into scrolls inside itself rather than growing.
	if strings.Contains(css, ".agents {") {
		t.Error("the sub-agent box is back in the column with the terminal")
	}
	at := strings.Index(css, ".dlist {")
	if at < 0 {
		t.Fatal("no .dlist rule, so the sub-agent list has nowhere bounded to live")
	}
	rule := css[at : at+strings.Index(css[at:], "}")]
	for _, want := range []string{"min-height: 0", "overflow-y: auto"} {
		if !strings.Contains(rule, want) {
			t.Errorf("the sub-agent list is missing %q, so it can grow without limit:\n%s", want, rule)
		}
	}

	// The rows that carry the session's chrome hold their own height. The strip
	// that used to be one of them is gone: it said "live" across the full width
	// while the pill beside the project name already said "running".
	if !strings.Contains(css, "header, .rail { flex: 0 0 auto; }") {
		t.Error("the chrome rows can be shrunk, so anything that grows takes their height first")
	}
	if strings.Contains(css, ".strip") {
		t.Error("the live strip is back")
	}
}

// The tabs are drawings in the bar and drawings with words in the rail, and
// the word never leaves the page.
//
// Four constraints, each of which has already been got wrong once somewhere in
// this codebase: nothing filled with a colour of its own (the first speech
// bubble had a white tail, which is a white block on a dark page), one box size
// and one border width across all five, the word present in the markup even
// where the bar hides it, and every tab naming itself for anyone who cannot see
// the drawing.
func TestTheTabsCarryDrawingsAndKeepTheirWords(t *testing.T) {
	src, err := os.ReadFile("shell.go")
	if err != nil {
		t.Fatal(err)
	}
	css := string(src)

	// Five icons, drawn the same way.
	for _, ic := range []string{"ic-sess", "ic-dec", "ic-in", "ic-rec", "ic-acc"} {
		if !strings.Contains(css, "nav ."+ic) {
			t.Errorf("no rule for %s", ic)
		}
	}
	// One size and one border, and now no exceptions: the bubble's ellipse is a
	// child sitting high inside the shared box rather than the box itself.
	if !strings.Contains(css, "width: 22px; height: 22px") {
		t.Error("the icons do not share one box size")
	}
	if strings.Contains(css, "nav .ic-in {") {
		t.Error("the bubble overrides the shared box again")
	}

	// The layering is the thing that makes the bubble correct, so it is the
	// thing worth holding. A filled tail drawn over an outlined bubble shows
	// its top edge inside the outline — a wedge across the interior, which
	// shipped twice. The order has to be tail, then an opaque bubble over it,
	// then the dots.
	tail := strings.Index(css, "nav .ic-in::before")
	bubble := strings.Index(css, "nav .ic-in > b")
	dots := strings.Index(css, "nav .ic-in::after")
	if tail < 0 || bubble < 0 || dots < 0 {
		t.Fatal("the bubble is not built from a tail, a bubble and dots")
	}
	if !(tail < bubble && bubble < dots) {
		t.Error("the bubble's three parts are out of paint order; the tail will cut into it")
	}
	if !strings.Contains(css[bubble:dots], "background: Canvas") {
		t.Error("the bubble is not opaque, so it cannot hide the half of the tail inside it")
	}
	// Centred by arithmetic. Guessing put the dots 1.85px off on a 22px icon.
	if !strings.Contains(css[dots:], "left: 50%;\n      margin-left: -4.55px") {
		t.Error("the dots are not centred against the group's own width")
	}
	if n := strings.Count(css, "1.7px solid currentColor"); n < 5 {
		t.Errorf("%d borders at the shared width; the five do not match", n)
	}
	// Nothing carries a colour of its own. currentColor inherits the theme;
	// anything else needs a dark-mode branch nothing here has.
	nav := css[strings.Index(css, "nav .ic {"):]
	if end := strings.Index(nav, "@media"); end > 0 {
		nav = nav[:end]
	}
	// Comments first, or this reads the prose rather than the rules — the
	// comment beside these icons explains the #fff bug by name, and matching
	// that is the test measuring the wrong thing again.
	nav = stripComments(nav)
	// Canvas is deliberately allowed: it is the same system colour the surface's
	// own background resolves to, so it follows the theme. What is banned is a
	// colour that does not.
	for _, banned := range []string{"#fff", "#000", "white", "black"} {
		if strings.Contains(nav, banned) {
			t.Errorf("an icon carries %q, which does not follow the theme", banned)
		}
	}

	// The word is hidden in the bar and shown in the rail — hidden, not dropped.
	if !strings.Contains(css, "nav a > span { display: none; }") {
		t.Error("the bar does not hide the word")
	}
	wide := strings.Index(css, "@media (min-width: 60rem)")
	if wide < 0 || !strings.Contains(css[wide:], "nav a > span { display: inline; }") {
		t.Error("the rail does not bring the word back")
	}

	// And on every surface: the drawing, the word, and the name.
	for path, body := range tabbed(t, true, true) {
		if strings.Contains(body, "/compose") && !strings.Contains(body, "<nav>") {
			continue
		}
		for _, want := range []string{
			`<i class="ic ic-sess"></i><span>Sessions</span>`,
			`<i class="ic ic-dec">?</i><span>Decisions</span>`,
			`<i class="ic ic-in"><b></b></i><span>Intake</span>`,
			`<i class="ic ic-rec"></i><span>Records</span>`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s is missing %s", path, want)
			}
		}
		for _, name := range []string{"Sessions", "Decisions", "Intake", "Records"} {
			if !strings.Contains(body, `aria-label="`+name+`"`) {
				t.Errorf("%s does not name its %s tab for a screen reader", path, name)
			}
		}
		// No SVG survives anywhere: the account icon was the last one.
		if strings.Contains(body, "<svg") {
			t.Errorf("%s still ships an SVG icon", path)
		}
	}

	// And the session surface, which this harness does not build.
	//
	// It was missed by exactly that gap: its Decisions tab carries a count, so
	// the pass that rewrote the plain tabs did not match it, and the surface
	// shipped with three drawings and one word. Every test here was green.
	dir := t.TempDir()
	a := &session.Adapter{Run: fakeRunner{listing: owned("mustur/Mustur")}}
	sess := &Sessions{Hub: &session.Hub{Adapter: a}, Adapter: a, Actor: "pie", HookDir: dir}
	mux := http.NewServeMux()
	sess.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	body := getFrom(t, srv, "/sessions/Mustur")
	for _, want := range []string{
		`<i class="ic ic-sess"></i><span>Sessions</span>`,
		`<i class="ic ic-dec">?</i><span>Decisions</span>`,
		`<i class="ic ic-in"><b></b></i><span>Intake</span>`,
		`<i class="ic ic-rec"></i><span>Records</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the session surface is missing %s", want)
		}
	}
}

// stripComments removes CSS block comments, so a rule can be searched without
// its own explanation answering for it.
func stripComments(css string) string {
	var b strings.Builder
	for {
		at := strings.Index(css, "/*")
		if at < 0 {
			b.WriteString(css)
			return b.String()
		}
		b.WriteString(css[:at])
		rest := css[at+2:]
		end := strings.Index(rest, "*/")
		if end < 0 {
			return b.String()
		}
		css = rest[end+2:]
	}
}
