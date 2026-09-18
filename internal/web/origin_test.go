package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/question"
)

// MUS-F-0096, on the owner's answer to MUS-Q-0173: the plain form posts refuse
// an Origin naming another site and take a request with none. These are the
// four Origins every such post is tried with. "self" stands for the test
// server's own URL, which is only known once it is running.
var formOrigins = []struct {
	name, origin string
	taken        bool
}{
	{"foreign", "https://evil.example", false},
	{"null", "null", false},
	{"absent", "", true},
	{"matching", "self", true},
}

// postOrigin posts a form with the Origin given ("" sends none) and reports
// the status, without following the redirect a taken post answers with.
func postOrigin(t *testing.T, c *http.Client, srvURL, path string, v url.Values, origin string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srvURL+path, strings.NewReader(v.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin == "self" {
		origin = srvURL
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	nc := *c
	nc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := nc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	return res.StatusCode
}

func TestNotCrossSiteTakesAnAbsentOriginAndNoForeignOne(t *testing.T) {
	cases := []struct {
		origin string
		want   bool
	}{
		{"", true},
		{"https://mustur.devofpie.com", true},
		{"http://MUSTUR.devofpie.com", true},
		{"https://evil.example", false},
		{"https://mustur.devofpie.com.evil.example", false},
		{"null", false},
		{"not a url at all", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodPost, "http://mustur.devofpie.com/intake", nil)
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if got := notCrossSite(r); got != c.want {
			t.Errorf("notCrossSite with Origin %q = %v, want %v", c.origin, got, c.want)
		}
	}
	// The strict check is not loosened by the new one: the socket and the
	// paths that type into an agent still refuse a request with no Origin.
	r := httptest.NewRequest(http.MethodPost, "http://mustur.devofpie.com/compose", nil)
	if sameOrigin(r) {
		t.Error("sameOrigin took a request with no Origin")
	}
}

func TestAJotFromAForeignOriginIsNotFiled(t *testing.T) {
	for _, c := range formOrigins {
		t.Run(c.name, func(t *testing.T) {
			srv, s := serve(t)
			ctx := context.Background()
			before, _ := s.Count(ctx)
			status := postOrigin(t, srv.Client(), srv.URL, "/intake", url.Values{"jot": {"a line worth keeping"}}, c.origin)
			after, _ := s.Count(ctx)
			if c.taken {
				if status != http.StatusSeeOther || after != before+1 {
					t.Errorf("answered %d and moved the record count %d → %d, want 303 and one filed", status, before, after)
				}
				return
			}
			if status != http.StatusForbidden || after != before {
				t.Errorf("answered %d and moved the record count %d → %d, want 403 and nothing filed", status, before, after)
			}
		})
	}
}

func TestAnAnswerFromAForeignOriginIsNotRecorded(t *testing.T) {
	for _, c := range formOrigins {
		t.Run(c.name, func(t *testing.T) {
			srv, s := serveQuestions(t, openQuestion("MUS-Q-0001", "Own the session, or attach?"))
			status := postOrigin(t, srv.Client(), srv.URL, "/questions",
				url.Values{"id": {"MUS-Q-0001"}, "answer": {"It owns the session."}}, c.origin)
			got, err := s.Get(context.Background(), "MUS-Q-0001")
			if err != nil {
				t.Fatal(err)
			}
			answered := question.Status(got) == question.StatusAnswered
			if c.taken {
				if status != http.StatusSeeOther || !answered {
					t.Errorf("answered %d, status %q; want 303 and answered", status, question.Status(got))
				}
				return
			}
			if status != http.StatusForbidden || answered {
				t.Errorf("answered %d, status %q; want 403 and still open", status, question.Status(got))
			}
		})
	}
}

func TestAHeldJotIsNotApprovedOrDiscardedFromAForeignOrigin(t *testing.T) {
	for _, action := range []string{"approve", "discard"} {
		for _, c := range formOrigins {
			t.Run(action+"/"+c.name, func(t *testing.T) {
				h := queueRig(t)
				ctx := context.Background()
				reader, _ := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
				owner, _ := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner})
				held := holdOne(t, h, reader, "")

				status := postOrigin(t, owner, h.srv.URL, "/intake/held/"+held.ID+"/"+action, url.Values{"to": {""}}, c.origin)
				var rows int
				if err := h.st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM held_jot`).Scan(&rows); err != nil {
					t.Fatal(err)
				}
				if c.taken {
					if status != http.StatusSeeOther || rows != 0 {
						t.Errorf("answered %d with %d held row(s) left, want 303 and none", status, rows)
					}
					return
				}
				if status != http.StatusForbidden || rows != 1 {
					t.Errorf("answered %d with %d held row(s) left, want 403 and the jot still held", status, rows)
				}
			})
		}
	}
}
