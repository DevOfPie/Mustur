package web

// A second press, and two presses at once (review of PR 108, finding 1).

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/DevOfPie/Mustur/internal/intake"
)

// Two presses of Move at once both land on the one record the move filed, and
// only one is filed. A press after the move lands there too, rather than on a
// bare 409.
func TestASecondMoveLandsWhereTheFirstWent(t *testing.T) {
	st, id := attending(t)
	srv, _ := attendingServer(t, st, false)
	nf := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	to := url.Values{"to": {"MUS-P-0003"}}

	locs := make([]string, 2)
	codes := make([]int, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res := press(t, nf, srv, "/records/"+id+"/move", srv.URL, to)
			codes[i], locs[i] = res.StatusCode, res.Header.Get("Location")
		}()
	}
	close(start)
	wg.Wait()
	for i := range 2 {
		if codes[i] != http.StatusSeeOther {
			t.Fatalf("press %d answered %d", i, codes[i])
		}
	}
	if locs[0] != locs[1] || !strings.HasSuffix(locs[0], "?moved="+id) {
		t.Errorf("the two presses landed on %q and %q", locs[0], locs[1])
	}

	late := press(t, nf, srv, "/records/"+id+"/move", srv.URL, to)
	if late.StatusCode != http.StatusSeeOther || late.Header.Get("Location") != locs[0] {
		t.Errorf("a late press answered %d to %q, want 303 to %q", late.StatusCode, late.Header.Get("Location"), locs[0])
	}

	all, err := st.List(context.Background(), "finding")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range all {
		for _, ref := range f.Refs {
			if ref.Key == "Corrects" && ref.Value == id {
				n++
			}
		}
	}
	if n != 1 {
		t.Errorf("%d records correct %s, want 1", n, id)
	}
}

// A second Keep goes back to the record without writing.
func TestASecondKeepWritesNothing(t *testing.T) {
	st, id := attending(t)
	srv, _ := attendingServer(t, st, false)
	nf := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for range 2 {
		res := press(t, nf, srv, "/records/"+id+"/keep", srv.URL, nil)
		if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/records/"+id+"?kept=1" {
			t.Fatalf("keep answered %d to %q", res.StatusCode, res.Header.Get("Location"))
		}
	}
	_, version, err := st.GetVersioned(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := st.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range rec.Data {
		if f.Key == intake.KeptField {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d Kept fields after two presses", n)
	}
	press(t, nf, srv, "/records/"+id+"/keep", srv.URL, nil)
	if _, again, _ := st.GetVersioned(context.Background(), id); again != version {
		t.Errorf("a third press wrote: version %d became %d", version, again)
	}
}
