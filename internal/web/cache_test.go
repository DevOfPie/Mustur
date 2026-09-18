package web

// The count cache and a press (review of PR 108, finding 6).

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/store"
)

// A count that started before a press cannot be what a poll after the press
// is answered with. get holds the lock for the whole count, so forget waits for
// a count in flight to be stored and then drops it; the next get counts again.
func TestACountInFlightIsForgotten(t *testing.T) {
	var c countCache
	now := func() time.Time { return time.Unix(0, 0) }
	started, release := make(chan struct{}), make(chan struct{})
	stale := func(context.Context, *store.Store) int {
		close(started)
		<-release
		return 1
	}
	done := make(chan int)
	go func() { done <- c.get(context.Background(), nil, now, stale) }()
	<-started

	// The press: the store has changed, and forget is called while the old
	// count is still being taken.
	forgot := make(chan struct{})
	go func() { c.forget(); close(forgot) }()
	close(release)
	if got := <-done; got != 1 {
		t.Fatalf("the count in flight returned %d", got)
	}
	<-forgot

	fresh := func(context.Context, *store.Store) int { return 0 }
	if got := c.get(context.Background(), nil, now, fresh); got != 0 {
		t.Errorf("a poll after forget was answered %d from before the press", got)
	}
}

// The defect was found on Move, so Move is tested too: poll, move, poll.
func TestAMoveIsSeenByTheNextPoll(t *testing.T) {
	st, id := attending(t)
	srv, _ := attendingServer(t, st, false)
	poll := func() string {
		t.Helper()
		return strings.TrimSpace(bodyOf(t, srv.Client(), srv.URL+"/records/attention/count"))
	}
	if got := poll(); got != `{"waiting":1}` {
		t.Fatalf("before: %s", got)
	}
	nf := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if res := press(t, nf, srv, "/records/"+id+"/move", srv.URL, url.Values{"to": {"MUS-P-0003"}}); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("move answered %d", res.StatusCode)
	}
	if got := poll(); got != `{"waiting":0}` {
		t.Errorf("the poll straight after a move said %s", got)
	}
}
