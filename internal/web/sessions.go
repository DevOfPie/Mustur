package web

// The session surface: a running session's output in a browser tab, and a way
// to answer it.
//
// **This is one of the two surfaces in v1 that carry a client layer, and the
// only one with no alternative.** Every other one is server-rendered with no
// script, no stylesheet, no font and no image, and that stays the rule — a live
// terminal simply cannot be server-rendered. The composer carries the other,
// taken by the owner on MUS-Q-0034 rather than assumed by whoever built it, and
// unlike this one its form posts and works with the script blocked. The stack
// table names both so the rule is not quietly dropped. (A tracked .js file also
// exists under docs/investigations — a fixture from milestone 1, served to
// nobody.)
//
// **The origin check is the control, not hardening.** The composer is always
// writable (MUS-Q-0018), so there is no second layer: this check and the Access
// policy's scope are the only things between a stranger and an agent's input.
// Browsers do not apply the same-origin policy to WebSockets and they send
// cookies with the handshake, so a page the owner merely visits could otherwise
// open a socket here, be authenticated by their existing Access session, and
// type into a running agent. Access authenticates the person; it does nothing
// about who opened the socket.

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/DevOfPie/Mustur/internal/session"
	"github.com/DevOfPie/Mustur/internal/store"
	"github.com/coder/websocket"
)

//go:embed assets/session.js
var sessionJS string

// MaxInput is the largest thing a viewer can type in one message. Generous for
// an answer and finite, which is the whole requirement — a socket that can type
// unboundedly into an agent is a way to burn a plan's usage as much as it is a
// way to misuse it.
const MaxInput = 8 << 10

// InputEvery is the fastest a viewer may send. Typing is a person's pace.
const InputEvery = 250 * time.Millisecond

// AgentsEvery is how often a connected viewer's sub-agent rows are refreshed.
//
// Two seconds because the thing it renders is a tool call, and an agent's tool
// calls are seconds apart rather than milliseconds; a faster tick would stat a
// file more often to show the same row. A slower one would let a sub-agent
// finish and be replaced between ticks.
const AgentsEvery = 2 * time.Second

// IdleTimeout closes a socket nobody is using. A tab left open on a phone in a
// drawer should not hold a writable channel into an agent for a week.
const IdleTimeout = 30 * time.Minute

// Sessions serves the session surface.
type Sessions struct {
	// ShowAccount renders the header link to the account surface, which is
	// served only when an origin is configured. Off means the link is absent
	// rather than dead (MUS-Q-0052).
	ShowAccount bool

	Hub     *session.Hub
	Adapter *session.Adapter
	// Store is read only for the count the Decisions tab carries. Nil means the
	// tab renders without one.
	Store *store.Store
	Actor string
	// HookDir is where the adapter's sub-agent hook logs its events. Empty
	// means the surface shows no sub-agent rows, which is also what a session
	// started without the hook shows.
	HookDir string
	Now     func() time.Time
}

// Routes registers the surface on an existing mux.
func (s *Sessions) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /sessions", s.list)
	mux.HandleFunc("GET /sessions/{project}", s.show)
	mux.HandleFunc("GET /sessions/{project}/ws", s.socket)
	mux.HandleFunc("GET /assets/session.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(sessionJS))
	})
}

func (s *Sessions) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Sessions) actor(r *http.Request) string {
	if who := r.Header.Get("Cf-Access-Authenticated-User-Email"); who != "" {
		return who
	}
	return s.Actor
}

// sameOrigin reports whether the request came from this site.
//
// An absent Origin is refused rather than allowed. Browsers always send it on a
// WebSocket handshake, so its absence means something that is not a browser —
// and a non-browser client has no business on the one path that types into an
// agent. That is the strict reading, and this is the place to take it.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

type sessionRow struct {
	Project string
	Here    bool
	State   string
}

type sessionPage struct {
	Project string
	// ShowAccount renders the header link to the account surface, which is
	// served only when an origin is configured. Off means the link is absent
	// rather than dead (MUS-Q-0052).
	ShowAccount   bool
	Rows          []sessionRow
	Subagents     []subagentRow
	Running       int
	OpenQuestions int
	Missing       bool
}

// A subagentRow is one sub-agent as the page says it, with every value already
// decided here rather than in the template.
type subagentRow struct {
	Title string `json:"title"` // what it was asked to do; empty when that could not be told
	Type  string `json:"type"`
	State string `json:"state"` // the tool in flight, "working", or "finished"
	Done  bool   `json:"done"`
	// For is the age as the first paint renders it. The socket sends the two
	// stamps instead and lets the client count, so a running sub-agent's age
	// moves every second without the server pushing a frame to say so — and so
	// the poll can skip entirely while the log is unchanged.
	For     string `json:"-"`
	Started int64  `json:"started"`
	Ended   int64  `json:"ended,omitempty"`
	Said    string `json:"said,omitempty"`
}

// subagents reads what the hook recorded for this session.
//
// The rows are server-rendered like everything else on this surface bar the
// output stream: a sub-agent starting is not a keystroke-latency event, and the
// page is already reloaded to see one. Nothing here reaches the socket.
func (s *Sessions) subagents(project string) ([]subagentRow, int) {
	if s.HookDir == "" || project == "" {
		return nil, 0
	}
	live, err := session.Subagents(s.HookDir, project)
	if err != nil || len(live) == 0 {
		return nil, 0
	}
	now := s.now()
	rows := make([]subagentRow, 0, len(live))
	running := 0
	for _, a := range live {
		r := subagentRow{
			Title: a.Task, Type: a.Type, For: since(a.For(now)), Said: a.Said,
			Started: a.Started.Unix(),
		}
		if a.Running() {
			running++
			// The tool it is in, or "working" between calls. Both halves are
			// real: the adapter hooks the end of a tool call as well as the
			// start, so a row leaves a tool when the sub-agent does. The first
			// version hooked only the start, and this comment described
			// behaviour the code did not have — a review caught the claim, not
			// the code.
			r.State = a.Doing
			if r.State == "" {
				r.State = "working"
			}
		} else {
			r.Done, r.State, r.Ended = true, "finished", a.Ended.Unix()
		}
		rows = append(rows, r)
	}
	return rows, running
}

// since renders an age for the first paint, coarse because a sub-agent's precise
// second is not a thing anyone acts on. It matches the client's age() rather
// than the client's quiet counter, which stops at whole hours — a review caught
// this comment claiming the quiet counter's format, which would put two
// different ages for the same sub-agent on the same page after an hour.
func since(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

func (s *Sessions) rows(ctx context.Context, here string) ([]sessionRow, bool) {
	live, err := s.Adapter.List(ctx)
	if err != nil {
		return nil, false
	}
	rows := make([]sessionRow, 0, len(live))
	found := false
	for _, sn := range live {
		if sn.Project == here {
			found = true
		}
		state := "running"
		if sn.Attached {
			state = "running · attached"
		}
		rows = append(rows, sessionRow{Project: sn.Project, Here: sn.Project == here, State: state})
	}
	return rows, found
}

func (s *Sessions) list(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.rows(r.Context(), "")
	if len(rows) > 0 {
		http.Redirect(w, r, "/sessions/"+url.PathEscape(rows[0].Project), http.StatusSeeOther)
		return
	}
	s.render(w, r, sessionPage{Project: "", Rows: nil, Missing: true})
}

func (s *Sessions) show(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	rows, found := s.rows(r.Context(), project)
	agents, running := s.subagents(project)
	s.render(w, r, sessionPage{
		Project: project, Rows: rows, Missing: !found,
		Subagents: agents, Running: running,
	})
}

func (s *Sessions) render(w http.ResponseWriter, r *http.Request, p sessionPage) {
	if s.Store != nil {
		p.OpenQuestions = OpenCount(r.Context(), s.Store)
	}
	// Set here rather than at the call sites: a page built without it renders
	// a header missing its only route to the account surface.
	p.ShowAccount = s.ShowAccount
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := sessionTmpl.Execute(w, p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type frame struct {
	T     string `json:"t"`
	Seq   int64  `json:"seq,omitempty"`
	Text  string `json:"text,omitempty"`
	Alive bool   `json:"alive,omitempty"`
	Quiet int    `json:"quiet,omitempty"`
	Lost  int64  `json:"lostBytes,omitempty"`
	At    string `json:"at,omitempty"`
	Error string `json:"error,omitempty"`
	// Sub-agent rows, pushed rather than waited for (MUS-Q-0029). The owner
	// chose this over a reload, against the builder's recommendation, and it is
	// the one place the client layer models something other than the terminal.
	Agents  []subagentRow `json:"agents,omitempty"`
	Running int           `json:"running,omitempty"`
	None    bool          `json:"none,omitempty"`
}

func (s *Sessions) socket(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		// Deliberately terse and deliberately not a redirect: nothing that
		// reached here by accident learns anything from the answer.
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	project := r.PathValue("project")

	from := int64(0)
	if v := r.URL.Query().Get("from"); v != "" {
		fmt.Sscanf(v, "%d", &from)
	}

	ctx := r.Context()
	sub, backlog, at, gap, err := s.Hub.Attach(ctx, project, from)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer sub.Close()

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// The check above is ours and stricter; this stops the library doing a
		// second, laxer one of its own.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer c.CloseNow()

	conn, cancel := context.WithCancel(context.Background())
	defer cancel()

	quiet := 0
	// Both the reader loop and the input goroutine write frames now — the
	// second only to report that a message was discarded — and a WebSocket
	// permits one writer at a time.
	var writing sync.Mutex
	send := func(f frame) error {
		b, err := json.Marshal(f)
		if err != nil {
			return err
		}
		writing.Lock()
		defer writing.Unlock()
		wctx, wcancel := context.WithTimeout(conn, 10*time.Second)
		defer wcancel()
		return c.Write(wctx, websocket.MessageText, b)
	}

	if err := send(frame{T: "hello", Alive: true, Seq: at, Quiet: quiet}); err != nil {
		return
	}
	if gap {
		// at is the oldest byte still held, so the loss is at-from. Written the
		// other way round it reported a negative count, which the client
		// rendered to the reader verbatim: "[-7988948 bytes … were not kept]".
		// A viewer whose offset is ahead of this stream was reading a previous
		// one, and how much it missed is not knowable — zero says so.
		lost := at - from
		if lost < 0 {
			lost = 0
		}
		if err := send(frame{T: "gap", Lost: lost}); err != nil {
			return
		}
	}
	if len(backlog) > 0 {
		if err := send(frame{T: "out", Seq: at + int64(len(backlog)), Text: string(backlog)}); err != nil {
			return
		}
	}

	// Sub-agent rows go down the same socket, on a ticker rather than in the
	// output path: a hook writes to a file this server does not own, so there
	// is nothing to be notified by. The file's size and modification time are
	// checked first, so a quiet session costs one stat per tick and no parse.
	agents := time.NewTicker(AgentsEvery)
	defer agents.Stop()
	var lastAgents, lastStamp string

	// Reset on activity. The timer used to be created once and never touched,
	// which made it a cap on the connection's age rather than on its idleness:
	// after thirty minutes a tab watching a working session was told the
	// session had ended, and the client stopped reconnecting. Idle means the
	// session is quiet, not that the tab is old (MUS-Q-0022).
	idle := time.NewTimer(IdleTimeout)
	defer idle.Stop()

	go s.readInput(conn, cancel, c, project, s.actor(r), idle, send)

	for {
		select {
		case <-conn.Done():
			return
		case <-idle.C:
			// Not "ended" — the session is very likely still running and only
			// this connection is going. The client treats it as a disconnect
			// and reconnects if anyone is still looking.
			_ = c.Close(websocket.StatusNormalClosure, "idle")
			return
		case <-agents.C:
			if stamp := session.SubagentStamp(s.HookDir, project); stamp == lastStamp {
				continue
			} else {
				lastStamp = stamp
			}
			rows, running := s.subagents(project)
			// Compared on what would be sent, so a tick that changes nothing
			// sends nothing — including the ages, which move on their own and
			// are exactly what the viewer is watching.
			b, err := json.Marshal(rows)
			if err != nil {
				continue
			}
			if string(b) == lastAgents {
				continue
			}
			lastAgents = string(b)
			// An empty list still goes, once: a session whose rows were cleared
			// has to be able to say so, and omitempty would have sent a frame
			// the client could not tell from a frame about nothing.
			if err := send(frame{T: "agents", Agents: rows, Running: running, None: len(rows) == 0}); err != nil {
				return
			}
		case u, ok := <-sub.C:
			if !ok {
				return
			}
			if u.Ended {
				_ = send(frame{T: "ended", Seq: u.Seq, At: u.ExitAt.Format("2006-01-02 15:04")})
				return
			}
			if err := send(frame{T: "out", Seq: u.Seq, Text: u.Text}); err != nil {
				return
			}
			resetIdle(idle, IdleTimeout)
		}
	}
}

// resetIdle restarts the timer, draining it first so a reset that races the
// fire does not leave a stale tick queued.
func resetIdle(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}

// readInput carries what the viewer typed into the session.
//
// Ownership is re-checked on every message rather than only at connect: a
// socket opened against a live session must not keep writing to that project's
// name after the session ends and a different one is started under it.
func (s *Sessions) readInput(ctx context.Context, cancel func(), c *websocket.Conn, project, actor string, idle *time.Timer, say func(frame) error) {
	defer cancel()
	c.SetReadLimit(MaxInput)
	last := time.Time{}
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		var f frame
		if err := json.Unmarshal(data, &f); err != nil || f.T != "input" {
			continue
		}
		text := strings.TrimSpace(f.Text)
		if text == "" {
			continue
		}
		// Every path out of here used to be silent: a message inside the rate
		// limit was skipped, a dead session closed the socket, and a failed
		// Send closed it too. The owner saw a pill change and their text gone.
		// frame.Error existed in this file the whole time and was never
		// assigned; the client now has a branch for it and keeps the draft.
		if now := s.now(); now.Sub(last) < InputEvery {
			_ = say(frame{T: "error", Error: "sent too quickly; that one was not delivered"})
			continue
		} else {
			last = now
		}
		live, err := s.Adapter.Alive(ctx, project)
		if err != nil || !live {
			_ = say(frame{T: "error", Error: "that session is no longer running, so nothing was sent"})
			return
		}
		if err := s.Adapter.Send(ctx, project, text); err != nil {
			_ = say(frame{T: "error", Error: "the session did not take it: " + err.Error()})
			return
		}
		// Typing counts as activity, so a session the owner is talking to does
		// not time out mid-conversation.
		resetIdle(idle, IdleTimeout)
	}
}

var sessionTmpl = template.Must(template.New("sessions").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mustur — sessions</title>
<style>
  :root { color-scheme: light dark; --edge: #8884; --accent: #6a8fd8;
          --accent-soft: #6a8fd820; }
  /* Capped, not floored. min-height let the column grow with the output and
     carry the bar and the composer off the screen with it (MUS-F-0032); the
     shell's own min-height is harmless beside a height that holds. */
  body { font: 17px/1.5 system-ui, sans-serif; margin: 0; max-width: 46rem;
         margin-inline: auto; display: flex; flex-direction: column;
         height: 100vh; height: 100dvh; }
  header { display: flex; align-items: center; gap: .5rem; padding: .75rem 1rem;
           border-bottom: 1.4px solid var(--edge); white-space: nowrap; }
  header .pill { border: 1px solid var(--edge); border-radius: 999px;
                 padding: .1rem .55rem; font-size: .78em; }
  header .pill.on { border-color: var(--accent); background: var(--accent-soft); }
  /* MUS-Q-0052: the account surface is reached from here rather than from
     a fifth tab, so MUS-D-0041's four stand. Rendered only when the server
     actually serves it — a link that goes nowhere is the failure the bar
     itself was written to avoid. */
  .acct { font-size: .82em; opacity: .6; text-decoration: none;
          color: inherit; margin-left: .6rem; }
  header .who { margin-left: auto; opacity: .6; font-size: .82em; }
  /* Chrome and output are visually separate. Anything Mustur says about the
     session sits on a tinted strip; anything the session said is plain text. */
  .strip { display: flex; align-items: center; gap: .5rem; padding: .4rem 1rem;
           background: #8881; border-bottom: 1.4px solid var(--edge);
           font-size: .82em; opacity: .8; white-space: nowrap; }
  .strip .grow { flex: 1; overflow: hidden; text-overflow: ellipsis; }
  /* The only thing that scrolls. min-height:0 is what lets a flex child be
     smaller than its content — without it the pane grows instead of
     scrolling, which is the whole of the bug.

     The bottom padding is the dock's own height, measured by the script and
     written back as --dock-h: the output runs behind the dock, as the owner
     asked, but its last lines still come to rest above it rather than under
     it. The fallback is a sensible dock height for the moment before the
     script has measured one. */
  #out { flex: 1; min-height: 0; overflow-y: auto;
         padding: .8rem 1rem calc(var(--dock-h, 9rem) + .8rem);
         margin: 0; white-space: pre-wrap;
         word-break: break-word; font-size: .9em;
         overscroll-behavior: contain; }

  /* The quiet timer and the composer, locked to the bottom of the screen.

     Fixed rather than last-in-a-column, because a column only holds its shape
     while the shell's height is what the browser says it is — and the owner
     watched the whole lower section walk off the bottom of a phone. Fixed
     elements are anchored to the viewport and cannot be pushed anywhere. It
     sits above the tab bar; where the bar becomes a rail there is nothing
     below it, and --shell-dock-offset is 0. */
  .dock { position: fixed; bottom: var(--shell-dock-offset, 0px);
          left: var(--shell-dock-left, 0px);
          width: var(--shell-dock-width, 100%);
          z-index: 2; background: var(--paper);
          border-top: 1.4px solid var(--edge); }
  #foot { padding: .4rem 1rem; background: #8881;
          font-size: .82em; opacity: .75; }
  form { display: flex; flex-direction: column; gap: .4rem; padding: .7rem 1rem;
         border-top: 1.4px solid var(--edge); }
  form .row { display: flex; gap: .5rem; align-items: flex-end; }
  /* Destination above the box, not inside it. Thought first, destination
     second: the line says where this is going and is changeable without the
     draft being at risk. */
  .dest { display: flex; align-items: baseline; gap: .5rem; font-size: .8em;
          opacity: .7; white-space: nowrap; }
  .dest .grow { flex: 1; overflow: hidden; text-overflow: ellipsis; }
  .dest a { color: inherit; }
  #kept { opacity: .75; font-style: italic; }
  /* Grows with what is typed, to a point, then scrolls. A phone keyboard eats
     half the screen, so the cap is small deliberately. */
  textarea { flex: 1; min-width: 0; font: inherit; padding: .55rem;
             border: 1px solid var(--edge); border-radius: .5rem;
             background: transparent; color: inherit; resize: none;
             max-height: 9rem; overflow-y: auto; line-height: 1.4; }
  button { font: inherit; padding: .55rem 1rem; border: 1px solid var(--accent);
           border-radius: .5rem; background: var(--accent-soft); color: inherit; }
  .rail { display: flex; gap: .4rem; padding: .5rem 1rem; overflow-x: auto;
          border-bottom: 1.4px solid var(--edge); white-space: nowrap; }
  .rail a { flex: 0 0 auto; border: 1px solid var(--edge); border-radius: 999px;
            padding: .2rem .7rem; font-size: .82em; text-decoration: none;
            color: inherit; opacity: .65; }
  .rail a.here { opacity: 1; border-color: var(--accent);
                 background: var(--accent-soft); }
  .none { opacity: .6; padding: 2rem 1rem; text-align: center; }
  /* Sub-agents sit above the session's own output, because they are Mustur
     talking about the session rather than the session talking. Same tint as
     every other strip for the same reason. */
  .agents { padding: .6rem 1rem; background: #8881;
            border-bottom: 1.4px solid var(--edge); font-size: .85em; }
  .agents > .count { opacity: .6; font-size: .9em; }
  .agent { display: flex; align-items: baseline; gap: .5rem; padding: .3rem 0;
           white-space: nowrap; }
  .agent .what { flex: 1; overflow: hidden; text-overflow: ellipsis; }
  .agent .what.untitled { opacity: .55; font-style: italic; }
  .agent .pill { border: 1px solid var(--edge); border-radius: 999px;
                 padding: .05rem .5rem; font-size: .78em; }
  .agent .pill.done { border-color: var(--accent); background: var(--accent-soft); }
  .agent .age { opacity: .6; font-size: .82em; }
  .said { margin: 0 0 .4rem .2rem; padding-left: .6rem;
          border-left: 1.4px solid var(--edge); white-space: pre-wrap;
          word-break: break-word; opacity: .85; font-size: .92em; }
` + shellCSS + `
</style>
</head>
<body data-project="{{.Project}}">
<header><strong>{{if .Project}}{{.Project}}{{else}}Sessions{{end}}</strong>
  <span class="pill" id="state">connecting</span>
  <span class="who">whippy-vm</span>{{if .ShowAccount}}<a class="acct" href="/account">Account</a>{{end}}</header>
{{if .Rows}}<div class="rail" id="rail">
  {{range .Rows}}<a href="/sessions/{{.Project}}"{{if .Here}} class="here"{{end}}>{{.Project}}</a>{{end}}
</div>{{end}}
{{if .Missing}}
<p class="none">{{if .Project}}Mustur did not start a session for {{.Project}}, so there is nothing to show.{{else}}No sessions.{{end}}<br>
<small>A session left running in a terminal is not here and will not appear.</small><br>
<small><a href="/compose">Compose</a> still works: with nothing running it files to the idea inbox.</small></p>
{{else}}
<div class="strip"><span class="grow" id="scrollback">connecting</span></div>
<div class="agents"{{if not .Subagents}} hidden{{end}}>
  {{if .Subagents}}<div class="count">{{len .Subagents}} sub-agent{{if ne (len .Subagents) 1}}s{{end}}{{if .Running}} · {{.Running}} running{{end}}</div>
  {{range .Subagents}}<div class="agent">
    {{if .Title}}<span class="what">{{.Title}}</span>{{else}}<span class="what untitled">{{.Type}}</span>{{end}}
    <span class="pill{{if .Done}} done{{end}}">{{.State}}</span><span class="age">{{.For}}</span>
  </div>{{if .Said}}<p class="said">{{.Said}}</p>{{end}}{{end}}{{end}}
</div>
<pre id="out"></pre>
<div class="dock">
<div id="foot">quiet 0s</div>
<form id="say">
  <div class="dest"><span class="grow" id="dest">Send to {{.Project}}</span><a href="/compose" id="compose-link">Compose…</a><span id="kept" hidden>draft kept</span></div>
  <div class="row">
    <textarea id="text" rows="1" placeholder="Reply to this session"
              spellcheck="true" autocapitalize="sentences" autocorrect="on"
              autocomplete="off"></textarea>
    <button type="submit">Send</button>
  </div>
</form>
</div>
{{end}}
<nav>
  <a href="/sessions" class="here">Sessions</a>
  <a href="/questions">Decisions{{if .OpenQuestions}} · {{.OpenQuestions}}{{end}}</a>
  <a href="/intake">Intake</a>
  <a href="/records">Records</a>
</nav>
{{if not .Missing}}<script src="/assets/session.js"></script>{{end}}
</body>
</html>
`))
