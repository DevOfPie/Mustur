package web

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/store"
)

// syncBuf is a log the handler goroutines and the test can share.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *syncBuf) lines() []string {
	return strings.Split(strings.TrimRight(s.String(), "\n"), "\n")
}

// waitFor polls the log, because a line is written after the handler returns
// and the client can have its answer first.
func waitFor(t *testing.T, log *syncBuf, want string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, l := range log.lines() {
			if strings.Contains(l, want) {
				return l
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no line containing %q in:\n%s", want, log.String())
	return ""
}

func TestAccessLogWritesOneLinePerRequest(t *testing.T) {
	log := &syncBuf{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /records", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hello"))
	})
	srv := httptest.NewServer(LogRequests(log, mux))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/records")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()

	l := waitFor(t, log, "GET /records ")
	for _, want := range []string{"access ", " done ", "status=418", "bytes=5", "dur=", "who=- role=-"} {
		if !strings.Contains(l, want) {
			t.Errorf("line lacks %q: %s", want, l)
		}
	}
}

// The things that must never reach the journal: an invitation secret in the
// path, an invitation link in ?invited=, a sentence in ?said=, and every
// header, Authorization and Cookie included.
func TestAccessLogWritesNoSecret(t *testing.T) {
	log := &syncBuf{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {})
	srv := httptest.NewServer(LogRequests(log, mux))
	defer srv.Close()

	const secret = "s3cretInvitationToken"
	for _, path := range []string{
		"/invite/" + secret,
		"/invite/" + secret + "/begin",
		// The review's three probes on PR 96, which the first version wrote
		// out, and two more spellings.
		"//invite/" + secret,
		"/invite/./" + secret,
		"/Invite/" + secret,
		"/invite/" + secret + "/extra",
		"/invite/" + secret[:6] + "%2F" + secret[6:],
		"/account/people?invited=https%3A%2F%2Fx%2Finvite%2F" + secret + "&said=bob%40example.com",
		"/sessions?p=Mustur&error=" + secret,
	} {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+secret)
		req.AddCookie(&http.Cookie{Name: SessionCookie, Value: secret})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	waitFor(t, log, "/sessions?")
	waitFor(t, log, "/invite/[secret]/begin")
	waitFor(t, log, "GET /Invite/[secret] ")
	waitFor(t, log, "GET /invite/[secret]/[secret] ")
	got := log.String()
	// The halves too, because the encoded slash splits the secret in two.
	for _, leak := range []string{secret, secret[:6], secret[6:], "example.com"} {
		if strings.Contains(got, leak) {
			t.Fatalf("%q reached the log:\n%s", leak, got)
		}
	}
	for _, want := range []string{
		"GET /invite/[secret] ",
		"GET /account/people?invited&said ",
		"GET /sessions?error&p=Mustur ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("log lacks %q:\n%s", want, got)
		}
	}
}

// The rule itself, spelling by spelling: whatever follows an `invite` segment
// is gone, and only a route's own verb survives in last place.
func TestRedactInvite(t *testing.T) {
	for in, want := range map[string]string{
		"/invite/S":         "/invite/[secret]",
		"/invite/S/begin":   "/invite/[secret]/begin",
		"/invite/S/finish":  "/invite/[secret]/finish",
		"//invite/S":        "//invite/[secret]",
		"/invite//S":        "/invite//[secret]",
		"/invite/./S":       "/invite/[secret]/[secret]",
		"/invite/../S":      "/invite/[secret]/[secret]",
		"/Invite/S":         "/Invite/[secret]",
		"/INVITE/S/":        "/INVITE/[secret]/",
		"/invite/S/extra":   "/invite/[secret]/[secret]",
		"/invite/begin":     "/invite/[secret]",
		"/invite/S/begin/x": "/invite/[secret]/[secret]/[secret]",
		"/x/invite/S":       "/x/invite/[secret]",
		"/records":          "/records",
		"/invitees/S":       "/invitees/S",
		"/invite":           "/invite",
	} {
		if got := redactInvite(in); got != want {
			t.Errorf("redactInvite(%q) = %q, want %q", in, got, want)
		}
	}
}

// The badge poll answering normally is not written; refused, it is.
func TestAccessLogQuietsTheBadgePoll(t *testing.T) {
	log := &syncBuf{}
	refuse := false
	mux := http.NewServeMux()
	mux.HandleFunc("GET /questions/count", func(w http.ResponseWriter, r *http.Request) {
		if refuse {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"waiting":0}`))
	})
	mux.HandleFunc("GET /records", func(w http.ResponseWriter, r *http.Request) {})
	srv := httptest.NewServer(LogRequests(log, mux))
	defer srv.Close()

	get := func(p string) {
		res, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	get("/questions/count")
	get("/records") // a marker, so the absence above is not a race
	waitFor(t, log, "GET /records ")
	if strings.Contains(log.String(), "/questions/count") {
		t.Fatalf("a normal poll was written:\n%s", log.String())
	}
	refuse = true
	get("/questions/count")
	waitFor(t, log, "GET /questions/count status=403")
}

// The session view's socket goes through the log: the hijack still works, and
// the socket gets a line when it opens and one when it closes.
func TestAccessLogKeepsTheSocketWorking(t *testing.T) {
	log := &syncBuf{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sessions/{project}/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept through the log: %v", err)
			return
		}
		defer c.CloseNow()
		_ = c.Write(r.Context(), websocket.MessageText, []byte("frame"))
		// Held until the client goes, as the real socket is.
		_, _, _ = c.Read(r.Context())
	})
	srv := httptest.NewServer(LogRequests(log, mux))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/sessions/Mustur/ws", nil)
	if err != nil {
		t.Fatalf("dial through the log: %v", err)
	}
	_, msg, err := c.Read(ctx)
	if err != nil || string(msg) != "frame" {
		t.Fatalf("read = %q, %v", msg, err)
	}
	open := waitFor(t, log, " open GET /sessions/Mustur/ws")
	if !strings.Contains(open, "status=101") {
		t.Errorf("open line: %s", open)
	}
	if strings.Contains(log.String(), " close ") {
		t.Fatalf("closed before the socket did:\n%s", log.String())
	}
	_ = c.Close(websocket.StatusNormalClosure, "")
	closed := waitFor(t, log, " close GET /sessions/Mustur/ws")
	if !strings.Contains(closed, "status=101") || !strings.Contains(closed, "dur=") {
		t.Errorf("close line: %s", closed)
	}
}

// A GET that streams — the tool call's server-to-client channel — can still
// flush through the log, the client sees the first chunk while the handler is
// still running, and the open line is written then.
func TestAccessLogKeepsTheStreamFlushing(t *testing.T) {
	log := &syncBuf{}
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /mcp", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Error("the log hid http.Flusher")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(": ok\n\n"))
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush through the log: %v", err)
		}
		<-release
	})
	srv := httptest.NewServer(LogRequests(log, mux))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := make([]byte, 6)
	if _, err := io.ReadFull(res.Body, buf); err != nil || string(buf) != ": ok\n\n" {
		t.Fatalf("first chunk = %q, %v", buf, err)
	}
	waitFor(t, log, " open GET /mcp status=200")
	close(release)
	waitFor(t, log, " close GET /mcp status=200")
}

// Behind the guard, the line says who: an account by email and role, a token
// by its label, and nobody for a request the guard turned away.
func TestAccessLogSaysWhoTheGuardFound(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	accounts := account.New(st.DB())
	mux := http.NewServeMux()
	mux.HandleFunc("/records", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {})
	auth := &Auth{Accounts: accounts, Origin: "http://127.0.0.1"}
	log := &syncBuf{}
	srv := httptest.NewServer(LogRequests(log, (&Guard{Auth: auth, Project: "MUS"}).Wrap(mux)))
	defer srv.Close()

	owner := signedInAs(t, srv, accounts, "owner@example.test", "MUS", account.Owner)
	res, err := owner.Get(srv.URL + "/records")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if l := waitFor(t, log, "GET /records status=200"); !strings.Contains(l, "who=owner@example.test role=owner") {
		t.Errorf("signed-in line: %s", l)
	}

	secret, _, err := accounts.IssueToken(ctx, "claude code on test", "MUS", account.Reader, "test", 0)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	l := waitFor(t, log, "POST /mcp status=200")
	if !strings.Contains(l, `who=token:"claude code on test"`) {
		t.Errorf("token line: %s", l)
	}
	if strings.Contains(log.String(), secret) {
		t.Fatalf("the token secret reached the log:\n%s", log.String())
	}

	anon := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err = anon.Get(srv.URL + "/records?p=x")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if l := waitFor(t, log, "GET /records?p=x status=303"); !strings.Contains(l, "who=- role=-") {
		t.Errorf("refused line: %s", l)
	}
}

// Every control byte is escaped, not only the ones that break a line: an
// escape sequence or a backspace written raw repaints a terminal reading the
// journal (the review on PR 96).
func TestAccessLogQuotesEveryControlByte(t *testing.T) {
	for c := 0; c < 0x20; c++ {
		p := "/a" + string(rune(c)) + "b"
		if got := quote(p); got == p || strings.ContainsRune(got, rune(c)) {
			t.Errorf("quote(%q) = %q: byte %#x written raw", p, got, c)
		}
	}
	if got := quote("/a\x7fb"); strings.ContainsRune(got, 0x7f) {
		t.Errorf("quote left DEL raw: %q", got)
	}
	if got := quote("/records/MUS-D-0001"); got != "/records/MUS-D-0001" {
		t.Errorf("an ordinary path was quoted: %q", got)
	}
}

// who= is free text from two places, an account's email and a token's label,
// and both are quoted the same way, so neither can fake a field or a line.
func TestAccessLogQuotesWho(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	accounts := account.New(st.DB())
	mux := http.NewServeMux()
	mux.HandleFunc("/records", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {})
	auth := &Auth{Accounts: accounts, Origin: "http://127.0.0.1"}
	log := &syncBuf{}
	srv := httptest.NewServer(LogRequests(log, (&Guard{Auth: auth, Project: "MUS"}).Wrap(mux)))
	defer srv.Close()

	// The invitation accepts this: it refuses whitespace and nothing else.
	const email = "a\x1b[2j\"role=owner\"@example.test"
	reader := signedInAs(t, srv, accounts, email, "MUS", account.Reader)
	res, err := reader.Get(srv.URL + "/records")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	l := waitFor(t, log, "GET /records status=200")
	if want := "who=" + fmt.Sprintf("%q", email) + " role=reader"; !strings.HasSuffix(l, want) {
		t.Errorf("email line: %q\nwant suffix %q", l, want)
	}

	const label = "bot\x1b[31m\x7f"
	secret, _, err := accounts.IssueToken(ctx, label, "MUS", account.Reader, "test", 0)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	l = waitFor(t, log, "POST /mcp status=200")
	if want := "who=token:" + fmt.Sprintf("%q", label); !strings.Contains(l, want) {
		t.Errorf("token line: %q\nwant %q", l, want)
	}
	if strings.ContainsAny(log.String(), "\x1b\x7f") {
		t.Fatalf("a control byte reached the log raw: %q", log.String())
	}
}
