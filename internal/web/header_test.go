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
