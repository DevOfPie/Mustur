package web

// Move is offered only for somewhere a jot can go, and a failed press says
// whose fault it was (review of PR 108, findings 5 and 7).

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/intake"
)

// A jot whose Names includes something that is not a routing record — gone,
// or never one — shows the banner saying so, offers Move only for the place
// it does name, and refuses a post for the other with 400.
func TestMoveIsOfferedOnlyForAPlace(t *testing.T) {
	st, id := attending(t)
	ctx := context.Background()
	rec, err := st.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for i := range rec.Data {
		if rec.Data[i].Key == intake.NamesField {
			rec.Data[i].Value = "MUS-P-0003, MUS-P-0099"
		}
	}
	if err := st.Append(ctx, rec, "amend", "test"); err != nil {
		t.Fatal(err)
	}
	srv, _ := attendingServer(t, st, false)
	body := bodyOf(t, srv.Client(), srv.URL+"/records/"+id)
	for _, want := range []string{
		"This jot names Archive and MUS-P-0099. Archive takes a jot only when a move is confirmed. MUS-P-0099 is not a place a jot can go.",
		`value="MUS-P-0003"`, "Keep in intake box",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if strings.Contains(body, `value="MUS-P-0099"`) {
		t.Error("Move is offered for somewhere that is not a place")
	}
	nf := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if res := press(t, nf, srv, "/records/"+id+"/move", srv.URL, url.Values{"to": {"MUS-P-0099"}}); res.StatusCode != http.StatusBadRequest {
		t.Errorf("a move to a non-place answered %d, want 400", res.StatusCode)
	}

	// With nothing it names being a place, the banner offers Keep alone.
	for i := range rec.Data {
		if rec.Data[i].Key == intake.NamesField {
			rec.Data[i].Value = "MUS-P-0099"
		}
	}
	if err := st.Append(ctx, rec, "amend", "test"); err != nil {
		t.Fatal(err)
	}
	body = bodyOf(t, srv.Client(), srv.URL+"/records/"+id)
	if !strings.Contains(body, "This jot names MUS-P-0099. MUS-P-0099 is not a place a jot can go.") ||
		strings.Contains(body, `class="primary"`) || !strings.Contains(body, "Keep in intake box") {
		t.Error("a jot naming no place should say so and offer Keep only")
	}
}

// A refusal is the caller's and gets a 4xx; anything else is the server's.
func TestAFailedPressSaysWhoseFaultItWas(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{intake.ErrUnknownDestination, http.StatusBadRequest},
		{&intake.Refusal{}, http.StatusConflict},
		{errors.New("disk I/O error"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		if got := pressStatus(c.err); got != c.want {
			t.Errorf("%v: %d, want %d", c.err, got, c.want)
		}
	}
}

// An agent's bearer token opens /mcp and nothing else: it cannot move or keep.
func TestATokenCannotMoveOrKeep(t *testing.T) {
	st, id := attending(t)
	srv, auth := attendingServer(t, st, true)
	secret, _, err := auth.Accounts.IssueToken(context.Background(), "agent", "MUS", account.Owner, "test", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"move", "keep"} {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/records/"+id+"/"+action, strings.NewReader("to=MUS-P-0003"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", srv.URL)
		req.Header.Set("Authorization", "Bearer "+secret)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("a token's %s answered %d, want 403", action, res.StatusCode)
		}
	}
	rec, err := st.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !intake.NeedsAttention(rec, "MUS-P-0002") {
		t.Error("a token changed the record")
	}
}
