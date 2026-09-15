package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/session"
	"github.com/coder/websocket"
)

// The session view's socket writes the same badge bar.js does, so it has to
// count what /questions/count counts for the same viewer. It counted questions
// alone, and an owner with a held jot watched the badge drop by one on every
// socket push until the next poll put it back.
func TestTheSocketCountsTheHeldJotsItsViewerMayApprove(t *testing.T) {
	h := queueRig(t)
	ctx := context.Background()
	if err := h.st.Append(ctx, openQuestion("MUS-Q-0001", "Open"), "create", "test"); err != nil {
		t.Fatal(err)
	}
	reader, _ := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
	_, owner := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner})
	_, other := h.as(t, "idw@example.com", map[string]account.Role{"MUS": account.Owner, "IDW": account.Owner})
	holdOne(t, h, reader, "MUS-P-0002") // the idea inbox: only `other` owns it

	s := &Sessions{Store: h.st, Project: "MUS", Roles: h.accounts}
	as := func(acct account.Account) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/sessions/x/ws", nil)
		return r.WithContext(withViewer(withRole(r.Context(), account.Owner), acct))
	}
	if n := s.waiting(ctx, as(owner)); n != 1 {
		t.Errorf("an owner of MUS alone counts %d, want the question only", n)
	}
	if n := s.waiting(ctx, as(other)); n != 2 {
		t.Errorf("an owner of IDW counts %d, want the question and the held jot", n)
	}
	// And it agrees with the poll for the same viewer.
	c, _ := h.as(t, "idw2@example.com", map[string]account.Role{"MUS": account.Owner, "IDW": account.Owner})
	if poll := waitingFor(t, c, h); poll != 2 {
		t.Errorf("/questions/count says %d for an IDW owner; the socket says 2", poll)
	}
}

// End to end over a real socket: the hello frame's count includes the held jot.
func TestTheHelloFrameCountsHeldJots(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux on PATH; this test only means something against the real thing")
	}
	h := queueRig(t)
	ctx0 := context.Background()
	if err := h.st.Append(ctx0, openQuestion("MUS-Q-0001", "Open"), "create", "test"); err != nil {
		t.Fatal(err)
	}
	reader, _ := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
	_, owner := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner})
	holdOne(t, h, reader, "")

	dir := t.TempDir()
	a := &session.Adapter{HookDir: dir}
	project := "zzHeldBadge"
	if _, err := a.Start(ctx0, project, t.TempDir(), "sh -c 'sleep 5'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Stop(context.Background(), project) })
	hub := &session.Hub{Adapter: a}
	t.Cleanup(hub.Shutdown)
	s := &Sessions{Hub: hub, Adapter: a, Actor: "pie", HookDir: dir, Store: h.st, Project: "MUS", Roles: h.accounts}
	mux := http.NewServeMux()
	s.Routes(mux)
	// What the guard stamps on a signed-in owner's request.
	stamped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(withViewer(withRole(r.Context(), account.Owner), owner)))
	})
	srv := httptest.NewServer(stamped)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(ctx0, 30*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/sessions/"+project+"/ws",
		&websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{srv.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var f frame
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	if f.T != "hello" || f.Waiting == nil || *f.Waiting != 2 {
		got := -1
		if f.Waiting != nil {
			got = *f.Waiting
		}
		t.Errorf("hello frame %q carries %d waiting, want 2 (a question and a held jot)", f.T, got)
	}
}
