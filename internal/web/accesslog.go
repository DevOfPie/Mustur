package web

// The access log: one line per request, to stderr, which systemd hands to the
// journal.
//
// It exists because a surface the owner saw misrender could not be traced
// afterwards (MUS-F-0162): the journal held the startup banner and nothing
// after it, so the URL the tab asked for, what it was answered and when were
// all gone.
//
// **What it never writes.** A query string's values, because `?invited=` on
// the people page carries a whole invitation link, secret included, and
// `?said=` and `?error=` carry sentences that can name an email; only the keys
// are written, except for the few named in queryValues, which carry a session
// or destination name and are the thing a diagnosis wants. Anything after an
// `invite` segment, however the path spells it, because that is where the
// invitation secret itself travels. No header at all, so no
// Authorization and no cookie. No body, so no form.
//
// **Streams.** A socket or a server-sent stream lasts as long as the tab does,
// so a single line at the end would say nothing about a tab that is open now.
// Such a request gets a line when it opens — at the hijack, or at the first
// flush of a GET — and its ordinary line when it ends, whose duration is how
// long it stayed open.
//
// **The badge poll.** bar.js asks /questions/count every ten seconds from every
// open tab that draws the bar. A successful answer to it is not written; a
// refused or failed one is, because that is the one a diagnosis would want.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DevOfPie/Mustur/internal/account"
)

// queryValues are the query keys whose values are written. Everything else is
// written as a bare key.
var queryValues = map[string]bool{
	"p":   true, // /sessions?p=<session>: which session a tab picked
	"to":  true, // /compose?to=<destination>
	"new": true, // /sessions?new=1
}

// who is filled in by the guard, which is the only thing that knows, and read
// back by the log once the request is done. A pointer on the context rather
// than a value, because the guard runs inside the log and a value it attached
// would never reach the outside.
type who struct {
	name string
	role account.Role
}

type whoKey struct{}

func noteWho(r *http.Request, name string, role account.Role) {
	if w, ok := r.Context().Value(whoKey{}).(*who); ok {
		w.name, w.role = name, role
	}
}

// LogRequests writes a line to out for every request next serves.
func LogRequests(out io.Writer, next http.Handler) http.Handler {
	var mu sync.Mutex
	emit := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = io.WriteString(out, s)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := &who{}
		rec := &recorder{ResponseWriter: w, req: r, start: start, who: id, emit: emit}
		next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), whoKey{}, id)))
		status := rec.status
		if status == 0 {
			if rec.hijacked {
				status = http.StatusSwitchingProtocols
			} else {
				status = http.StatusOK
			}
		}
		if quiet(r, status) {
			return
		}
		verb := "done"
		if rec.opened {
			verb = "close"
		}
		emit(line(start, verb, r, status, rec.bytes, time.Since(start), id))
	})
}

// quiet is the badge poll answering normally.
func quiet(r *http.Request, status int) bool {
	return r.Method == http.MethodGet && r.URL.Path == "/questions/count" && status == http.StatusOK
}

func line(at time.Time, verb string, r *http.Request, status int, bytes int64, dur time.Duration, id *who) string {
	var b strings.Builder
	fmt.Fprintf(&b, "access %s %s %s %s", at.Format(time.RFC3339Nano), verb, r.Method, safePath(r.URL))
	if status != 0 {
		fmt.Fprintf(&b, " status=%d bytes=%d", status, bytes)
	}
	if dur >= 0 {
		fmt.Fprintf(&b, " dur=%s", dur.Round(time.Millisecond/10))
	}
	name := id.name
	if name == "" {
		name = "-"
	}
	role := string(id.role)
	if role == "" {
		role = "-"
	}
	fmt.Fprintf(&b, " who=%s role=%s\n", name, role)
	return b.String()
}

// safePath is the path with the invitation secret taken out, and the query
// reduced to its keys.
func safePath(u *url.URL) string {
	p := redactInvite(u.Path)
	if u.RawQuery == "" {
		return quote(p)
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return quote(p) + "?[unparsed]"
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if queryValues[k] {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(q.Get(k)))
		} else {
			parts = append(parts, url.QueryEscape(k))
		}
	}
	return quote(p) + "?" + strings.Join(parts, "&")
}

// inviteVerbs are the segments the invitation routes put after the secret
// (auth.go: /invite/{token}/begin and /finish). They are the only thing after
// an `invite` segment that is ever written as itself.
var inviteVerbs = map[string]bool{"begin": true, "finish": true}

// redactInvite replaces every segment after any segment spelled `invite`, in
// any case, with [secret] — except an empty one, and a route's own verb in last
// place straight after a redacted segment.
//
// It deliberately does not match on a prefix of the path. The first version
// did, and wrote the secret for `//invite/S`, `/invite/./S` and `/Invite/S`
// (the review on PR 96): the mux redirects the first two only after the line
// is written, and the third is a 404 whose path still holds the secret.
// Cleaning the path first would fix those three and leave the rule depending
// on the cleaner agreeing with the mux. Looking at every segment does not: a
// doubled slash or a dot is one more segment, an encoded slash has already
// been decoded into two by the time u.Path is read, and whatever follows the
// word is taken out, however many segments that is.
func redactInvite(p string) string {
	segs := strings.Split(p, "/")
	after := false
	for i, s := range segs {
		if after {
			// An empty segment, from a doubled or trailing slash, holds
			// nothing to hide.
			verb := i == len(segs)-1 && segs[i-1] == "[secret]" && inviteVerbs[s]
			if s != "" && !verb {
				segs[i] = "[secret]"
			}
			continue
		}
		after = strings.EqualFold(s, "invite")
	}
	return strings.Join(segs, "/")
}

// quote keeps one request on one line whatever its path holds.
func quote(p string) string {
	if needsQuote(p, ` "`) {
		return fmt.Sprintf("%q", p)
	}
	return p
}

// quoteField keeps free text — a token label, an email — to one field of one
// log line.
func quoteField(s string) string {
	if s == "" || needsQuote(s, ` "=`) {
		return fmt.Sprintf("%q", s)
	}
	return s
}

// needsQuote is any control byte, or any of also. Every control byte rather
// than the four that break a line: an escape sequence or a backspace written
// raw can repaint what a terminal reading the journal shows, and %q escapes
// all of them once the string is quoted.
func needsQuote(s, also string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c == 0x7f {
			return true
		}
	}
	return strings.ContainsAny(s, also)
}

// recorder counts what was written without hiding what the writer underneath
// can do. The session view's socket hijacks the connection and the tool call's
// stream flushes; a wrapper that dropped either would break both surfaces
// rather than log them.
type recorder struct {
	http.ResponseWriter
	req      *http.Request
	start    time.Time
	who      *who
	emit     func(string)
	status   int
	bytes    int64
	hijacked bool
	opened   bool
}

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += int64(n)
	return n, err
}

// Flush opens a stream's line on a GET's first flush. A POST that flushes is
// the tool call answering one message, which ends in moments and gets its one
// line then.
func (r *recorder) Flush() {
	if !r.opened && r.req.Method == http.MethodGet {
		r.open(r.status)
	}
	_ = http.NewResponseController(r.ResponseWriter).Flush()
}

func (r *recorder) FlushError() error {
	if !r.opened && r.req.Method == http.MethodGet {
		r.open(r.status)
	}
	return http.NewResponseController(r.ResponseWriter).Flush()
}

func (r *recorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("access log: the writer underneath cannot hijack")
	}
	c, rw, err := hj.Hijack()
	if err == nil {
		r.hijacked = true
		if r.status == 0 {
			r.status = http.StatusSwitchingProtocols
		}
		r.open(r.status)
	}
	return c, rw, err
}

// Unwrap lets http.ResponseController reach anything else underneath.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *recorder) open(status int) {
	r.opened = true
	if status == 0 {
		status = http.StatusOK
	}
	r.emit(line(r.start, "open", r.req, status, r.bytes, -1, r.who))
}
