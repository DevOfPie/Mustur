package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/account"
)

// Every surface that draws the tab bar renders the count the poll gives, held
// jots included, so an owner with script blocked is still told one is waiting.
// Records, Compose, Account and the session page's first render counted
// questions alone (PR 103's review, finding 1).
func TestEverySurfacesRenderedBadgeCountsHeldJots(t *testing.T) {
	h := queueRig(t)
	ctx := context.Background()
	if err := h.st.Append(ctx, openQuestion("MUS-Q-0001", "Open"), "create", "test"); err != nil {
		t.Fatal(err)
	}
	(&Compose{Store: h.st, Project: "MUS", Actor: "pie", Roles: h.accounts}).Routes(h.mux)
	auth := &Auth{Accounts: h.accounts, Origin: "http://127.0.0.1"}
	(&Accounts{Store: h.accounts, Auth: auth, Project: "MUS", Records: h.st}).Routes(h.mux)
	reader, _ := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
	owner, ownerAcct := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner})
	// Owns MUS only, so a jot sent to the idea inbox is not theirs to count.
	holdOne(t, h, reader, "")
	holdIDW(t, h, reader)

	if n := waitingFor(t, owner, h); n != 2 {
		t.Fatalf("the poll says %d, want one question and the one held jot they may approve", n)
	}
	for _, path := range []string{"/records", "/compose", "/account"} {
		if page := bodyOf(t, owner, h.srv.URL+path); !strings.Contains(page, `<em class="cnt">2</em>`) {
			t.Errorf("the owner's rendered badge on %s is not the poll's 2", path)
		}
		// Compose is owner-only at the guard, so a reader never renders it.
		if path == "/compose" {
			continue
		}
		if page := bodyOf(t, reader, h.srv.URL+path); !strings.Contains(page, `<em class="cnt">1</em>`) {
			t.Errorf("the reader's rendered badge on %s is not the question alone", path)
		}
	}
	// The session page's first render, for a request the guard stamped as the
	// owner's. Rendered directly: listing sessions would need tmux, and the
	// badge does not.
	s := &Sessions{Store: h.st, Project: "MUS", Roles: h.accounts}
	req := httptest.NewRequest(http.MethodGet, "/sessions/x", nil)
	req = req.WithContext(withViewer(withRole(req.Context(), account.Owner), ownerAcct))
	rec := httptest.NewRecorder()
	s.render(rec, req, sessionPage{Project: "x", Missing: true})
	if !strings.Contains(rec.Body.String(), `<em class="cnt">2</em>`) {
		t.Error("the session page's first render is not the poll's 2")
	}
}

// holdIDW holds a second, different line pointed at the idea inbox, which an
// owner of MUS alone may not approve.
func holdIDW(t *testing.T, h heldRig, reader *http.Client) {
	t.Helper()
	v := map[string][]string{"jot": {"An idea for the inbox"}, "to": {"MUS-P-0002"}}
	if res, _ := sendForm(t, reader, h.srv.URL+"/intake", v); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("the reader's send to IDW answered %d", res.StatusCode)
	}
}
