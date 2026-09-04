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

// A key gets a shorter leash than a message.
//
// InputEvery is 250ms, which is right for a message -- nobody writes two
// sentences in a quarter second, and the limit is there to stop a pane being
// flooded with prose. Arrow keys are the opposite: moving four rows down a list
// at one press per 250ms is the kind of latency that makes a control feel
// broken. 60ms is still a limit, and a key is a few bytes where a message can
// be the read limit's worth.
const KeysEvery = 60 * time.Millisecond

// How often a connected tab is told how many decisions are waiting.
//
// Slower than the sub-agent tick on purpose: OpenCount lists every record in
// the store and filters, where the sub-agent poll stats one file and usually
// stops there. Ten seconds is the latency on a badge, not on the terminal, and
// the alternative measured worse -- that list ran per viewer every two seconds
// for a number that changes a few times a day.
const WaitingEvery = 10 * time.Second

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
	// ID addresses this sub-agent. The hook has recorded one since milestone
	// 4c; nothing downstream could open a row without it reaching the page.
	ID    string `json:"id"`
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
			ID:    a.ID,
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
	// Where the session picker submits.
	//
	// A GET form cannot build a path segment, so the dropdown posts a query
	// here and this turns it into one. With script the change event navigates
	// first and this is never reached; without it, this is the whole of how
	// the picker works.
	if pick := r.URL.Query().Get("p"); pick != "" {
		http.Redirect(w, r, "/sessions/"+url.PathEscape(pick), http.StatusSeeOther)
		return
	}
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
	// Never cached, which every other surface here already says of itself and
	// this one did not.
	//
	// This page is markup and a script that has to agree with it. The script
	// is revalidated on every load; the page was not, so a deploy could leave
	// a reader holding yesterday's markup beside today's script. That is not
	// theoretical — it is how sub-agent rows came to be un-openable after
	// MUS-F-0038 shipped: rows drawn before the change carry no identifier,
	// and the delegated handler looking for one finds nothing to open
	// (MUS-F-0041).
	//
	// no-store rather than no-cache because it also keeps the page out of the
	// back/forward cache, which restores a whole live document — script state
	// and all — from before the deploy.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := sessionTmpl.Execute(w, p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type frame struct {
	T string `json:"t"`
	// Screen is the pane, rendered. It replaces the chunk of bytes this used
	// to carry: the unit is now a whole screen, so there is no offset to
	// resume from, no replay to distinguish and no gap to report (MUS-Q-0060).
	Screen string `json:"screen,omitempty"`
	// Text goes the other way: it is what the composer sends up. Nothing is
	// sent down as text any more — a screen is not a chunk.
	Text string `json:"text,omitempty"`
	// Key is the other thing that goes up: one keypress, named, from the row
	// above the composer. A pane can ask for a key rather than a sentence and
	// the surface could not answer one (MUS-F-0080); MUS-Q-0072 chose this row
	// over sending nothing and over becoming a full keyboard. It is a separate
	// field rather than a reserved Text value so that nothing can type the word
	// "escape" and have it pressed.
	Key   string `json:"key,omitempty"`
	Alive bool   `json:"alive,omitempty"`
	Quiet int    `json:"quiet,omitempty"`
	// Agent is what the CLI's own pane says it is doing: working, waiting, or
	// empty for a pane nothing here can read. Empty is not idle — the surface
	// falls back to counting silence, which is what it did before.
	Agent string `json:"agent,omitempty"`
	// Status is what the CLI's furniture said, taken off the bottom of the
	// screen and shown as Mustur's own row instead of as four lines of the
	// output nobody reads.
	Status *statusRow `json:"status,omitempty"`
	At     string     `json:"at,omitempty"`
	Error  string     `json:"error,omitempty"`
	// Sub-agent rows, pushed rather than waited for (MUS-Q-0029). The owner
	// chose this over a reload, against the builder's recommendation, and it is
	// the one place the client layer models something other than the terminal.
	Agents  []subagentRow `json:"agents,omitempty"`
	Running int           `json:"running,omitempty"`
	None    bool          `json:"none,omitempty"`
	// How many decisions are open, for the tab bar's count (MUS-F-0069). The
	// bar is rendered once by the server and this page outlives that render by
	// hours, so a question raised while somebody watches a session used to
	// leave the badge saying what was true when the tab opened. A pointer
	// rather than an int: dropping to zero is the update that matters most and
	// omitempty would swallow it.
	Waiting *int `json:"waiting,omitempty"`
}

// A statusRow is the CLI's status line, ready to render.
//
// Sent whole rather than as one string, so the surface decides what a mode
// looks like next to a failing check. Empty fields are omitted, and a row with
// nothing in it is not sent at all.
type statusRow struct {
	Mode   string   `json:"mode,omitempty"`
	Items  []string `json:"items,omitempty"`
	Note   string   `json:"note,omitempty"`
	Hint   string   `json:"hint,omitempty"`
	Update string   `json:"update,omitempty"`
}

func statusChips(st session.Status) *statusRow {
	if st.Empty() {
		return nil
	}
	return &statusRow{
		Mode: st.Mode, Items: st.Items, Note: st.Note,
		Hint: st.Hint, Update: st.Update,
	}
}

func (s *Sessions) socket(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		// Deliberately terse and deliberately not a redirect: nothing that
		// reached here by accident learns anything from the answer.
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	project := r.PathValue("project")

	ctx := r.Context()
	// No `from`. A viewer resuming is sent the screen as it stands, which is
	// the whole of what resuming means when the unit is a frame.
	sub, now, err := s.Hub.Watch(ctx, project)
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

	// How long the screen has been unchanged, from the poller rather than from
	// this connection. Declared and left at zero, it made the browser start
	// counting at whatever moment the tab attached, so the footer measured the
	// age of the tab (MUS-F-0042). Rounded down to whole seconds, which is what
	// the client renders anyway.
	quiet := int(sub.Quiet(s.now()).Seconds())
	if quiet < 0 {
		quiet = 0
	}
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

	// The first frame is the screen as it stands, and what the pane says the
	// agent is doing — read out of the same capture rather than fetched again.
	// Guarded the way render is: a Sessions with no store still serves the
	// terminal, and the badge is simply not a thing it can count.
	waitingNow := 0
	if s.Store != nil {
		waitingNow = OpenCount(conn, s.Store)
	}
	if err := send(frame{
		T: "hello", Alive: true, Quiet: quiet,
		Screen: now.HTML, Agent: string(now.Agent), Status: statusChips(now.Status),
		Waiting: waitingIf(s.Store != nil, &waitingNow),
	}); err != nil {
		return
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

	// The count in the tab bar, kept current for as long as the tab is open.
	// Sent only when it moves, so a session nobody has raised a question from
	// costs one list every ten seconds and no frames.
	waiting := time.NewTicker(WaitingEvery)
	defer waiting.Stop()
	lastWaiting := waitingNow

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
		case <-waiting.C:
			if s.Store == nil {
				continue
			}
			n := OpenCount(conn, s.Store)
			if n == lastWaiting {
				continue
			}
			lastWaiting = n
			if err := send(frame{T: "waiting", Waiting: &n}); err != nil {
				return
			}
		case <-agents.C:
			// The agent's state used to be captured here, on its own timer.
			// The poller already has the pane in hand and reads it out of the
			// same text, so this tick is back to being only about sub-agents.
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
		case f, ok := <-sub.C:
			if !ok {
				return
			}
			if f.Ended {
				_ = send(frame{T: "ended", At: f.ExitAt.Format("2006-01-02 15:04")})
				return
			}
			if err := send(frame{
				T: "screen", Screen: f.HTML,
				Agent: string(f.Agent), Status: statusChips(f.Status),
			}); err != nil {
				return
			}
			resetIdle(idle, IdleTimeout)
		}
	}
}

// waitingIf keeps the frame honest about a count nobody took: a zero sent by a
// server with no store is indistinguishable from a zero that was counted, and
// the client would clear a badge the page had rendered correctly.
func waitingIf(ok bool, n *int) *int {
	if !ok {
		return nil
	}
	return n
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
	// Keys are paced separately, so holding an arrow down does not spend the
	// composer's budget and a message does not have to wait behind one.
	lastKey := time.Time{}
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		var f frame
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		if f.T != "input" && f.T != "key" {
			continue
		}
		text := strings.TrimSpace(f.Text)
		key := strings.TrimSpace(f.Key)
		// A key frame carries a key and nothing else. Both empty is the
		// composer's own dropped empty submit arriving anyway.
		if f.T == "key" {
			text = ""
		} else {
			key = ""
		}
		if text == "" && key == "" {
			continue
		}
		// Every path out of here used to be silent: a message inside the rate
		// limit was skipped, a dead session closed the socket, and a failed
		// Send closed it too. The owner saw a pill change and their text gone.
		// frame.Error existed in this file the whole time and was never
		// assigned; the client now has a branch for it and keeps the draft.
		every, when := InputEvery, &last
		if key != "" {
			every, when = KeysEvery, &lastKey
		}
		if now := s.now(); now.Sub(*when) < every {
			_ = say(frame{T: "error", Error: "sent too quickly; that one was not delivered"})
			continue
		} else {
			*when = now
		}
		live, err := s.Adapter.Alive(ctx, project)
		if err != nil || !live {
			_ = say(frame{T: "error", Error: "that session is no longer running, so nothing was sent"})
			return
		}
		if key != "" {
			// A rejected key is not a reason to drop the socket: the row is a
			// handful of buttons and a name this server does not know means the
			// page is older than the binary, not that the session is gone.
			if err := s.Adapter.SendKey(ctx, project, key); err != nil {
				_ = say(frame{T: "error", Error: "the session did not take that key: " + err.Error()})
				continue
			}
		} else if err := s.Adapter.Send(ctx, project, text); err != nil {
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
  /* The 46rem below applies under the breakpoint only; above it the shell's
     own rule wins and gives this page the width the rail leaves, which is now
     what every surface gets. It was set here first, for the terminal alone,
     before the owner asked for the rest. */
  body { font: 17px/1.5 system-ui, sans-serif; margin: 0; max-width: 46rem;
         margin-inline: auto; display: flex; flex-direction: column;
         height: 100vh; height: 100dvh; }
  /* The chrome rows keep their own height. A flex item shrinks by default, so
     anything that grew — the sub-agent box did — took its room out of these
     first, which is how the session chips ended up half-height and hidden
     behind the row beneath them. Only #out flexes. */
  header, .rail { flex: 0 0 auto; }
  header { display: flex; align-items: center; gap: .5rem; padding: .75rem 1rem;
           border-bottom: 1.4px solid var(--edge); white-space: nowrap; }
  header .pill { border: 1px solid var(--edge); border-radius: 999px;
                 padding: .1rem .55rem; font-size: .78em; }
  header .pill.on { border-color: var(--accent); background: var(--accent-soft); }
  /* The ring around the status pill is smaller than the one around the
     sub-agent button, because the pill is smaller and a 190% square around it
     would spill a long way past a short word. */
  header .ring { padding: 1.2px; }
  header .ring.live::before { width: 260%; }
  /* The ring is a gradient in a padding box, so whatever it wraps has to be
     opaque and painted above it, or the two bright arms sweep across the word
     instead of around it (MUS-F-0068). .toggle was both and this was neither:
     an unpositioned box loses to an absolutely positioned pseudo-element, and
     --accent-soft is 12.5% alpha, so the gradient came through the fill as
     well as over it. The on state layers the same token over the page rather
     than replacing it, which keeps the tint and stops the light. */
  header .ring > .pill { position: relative; background: var(--paper); }
  header .ring > .pill.on { background:
      linear-gradient(var(--accent-soft), var(--accent-soft)), var(--paper); }
  /* MUS-Q-0052: the account surface is reached from here rather than from
     a fifth tab, so MUS-D-0041's four stand. Rendered only when the server
     actually serves it — a link that goes nowhere is the failure the bar
     itself was written to avoid. */
  .acct { font-size: .82em; opacity: .6; text-decoration: none;
          color: inherit; margin-left: .6rem; }
  header .who { margin-left: auto; opacity: .6; font-size: .82em; }
  /* The strip that used to sit here said "live" across the whole width, and
     the pill in the header beside the project name already said "running".
     Two places saying one thing, one of them a full-width band above the
     output. The owner asked for it gone; what it alone carried — the time a
     session ended — moved into the pill. */
  /* The only thing that scrolls. min-height:0 is what lets a flex child be
     smaller than its content — without it the pane grows instead of
     scrolling, which is the whole of the bug.

     The bottom padding is the dock's own height, measured by the script and
     written back as --dock-h: the output runs behind the dock, as the owner
     asked, but its last lines still come to rest above it rather than under
     it. The fallback is a sensible dock height for the moment before the
     script has measured one. */
  /* What the CLI's status line said, as Mustur's own row.

     The owner asked for the four lines of furniture at the bottom of the pane
     to come out of the output and for the useful parts to be shown better.
     These are those parts: the mode, whatever the CLI is tracking, a failing
     check, and a hint or an update notice.

     One row, and only when there is something in it. The strip removed on
     MUS-D-0129 was removed for saying what the pill already said; this says
     what nothing else does. */
  .chips { display: flex; flex-wrap: wrap; align-items: center; gap: .35rem;
           padding: .4rem 1rem; border-bottom: 1.4px solid var(--edge);
           font-size: .78em; flex: 0 0 auto; }
  .chips[hidden] { display: none; }
  .chips span { border: 1px solid var(--edge); border-radius: 999px;
                padding: .05rem .5rem; opacity: .75; white-space: nowrap; }
  .chips span.mode { border-color: var(--accent); background: var(--accent-soft);
                     opacity: 1; }
  /* A failing check is the one thing on this row worth interrupting for. */
  .chips span.note { border-color: #c0392b; color: #c0392b; opacity: 1; }
  .chips span.hint { border-style: dashed; }

  /* The pane, painted rather than accumulated.
     
     A monospaced face because this is a screen tmux laid out in columns, and a
     proportional one would break every box it draws. white-space: pre-wrap
     keeps its spacing while still wrapping a long line to the reader's width
     rather than the pane's. */
  #out { flex: 1; min-height: 0; overflow-y: auto;
         padding: .8rem 1rem calc(var(--dock-h, 9rem) + .8rem);
         margin: 0; white-space: pre-wrap;
         font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
         word-break: break-word; font-size: .82em; line-height: 1.35;
         overscroll-behavior: contain; }
  /* Something Mustur says about the session, as opposed to something the
     session said. Under the screen, because the screen is replaced whole and
     anything written into it would go with the next frame. */
  #out .note { margin: .6rem 0 0; padding: .3rem .6rem;
               border-left: 2px solid var(--accent); opacity: .8;
               font-family: system-ui, sans-serif; white-space: normal; }

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
  /* The key row. Scrolls sideways rather than wrapping, because a wrapped row
     changes the dock's height and the output is positioned off it. */
  .keys { display: flex; align-items: center; gap: .4rem; margin-bottom: .45rem;
          overflow-x: auto; scrollbar-width: none; }
  .keys::-webkit-scrollbar { display: none; }
  .keys button { font: inherit; font-size: .8em; line-height: 1;
                 padding: .4rem .6rem; min-width: 2.2rem; flex: 0 0 auto;
                 border: 1px solid var(--edge); border-radius: .45rem;
                 background: var(--paper); color: inherit; cursor: pointer; }
  .keys button:active { background: var(--accent-soft); border-color: var(--accent); }
  /* The four arrows read as one control, so they are joined into one. */
  .keys .pad { display: inline-flex; flex: 0 0 auto; }
  .keys .pad button { border-radius: 0; margin-left: -1px; }
  .keys .pad button:first-child { border-radius: .45rem 0 0 .45rem; margin-left: 0; }
  .keys .pad button:last-child { border-radius: 0 .45rem .45rem 0; }
  /* Ctrl-C ends a turn. It sits apart from the keys that move around inside
     one, which is the whole of the distinction this draws -- there is no tick
     in front of it, because a row of seven buttons that each need confirming
     is not a row anybody would use. */
  .keys .stopish { margin-left: auto; opacity: .8; }
  textarea { flex: 1; min-width: 0; font: inherit; padding: .55rem;
             border: 1px solid var(--edge); border-radius: .5rem;
             background: transparent; color: inherit; resize: none;
             max-height: 9rem; overflow-y: auto; line-height: 1.4; }
  button { font: inherit; padding: .55rem 1rem; border: 1px solid var(--accent);
           border-radius: .5rem; background: var(--accent-soft); color: inherit; }
  /* The session picker.

     A dropdown rather than a row of chips. MUS-D-0121 answered the identical
     problem on the intake row: a row that scrolls sideways hides its last
     choice behind a swipe with nothing on screen saying so, and a jot went to
     the very chip nobody could see. A native select has no off-screen end,
     however many sessions there are.

     The form is real, and its button lives in a noscript element.

     A GET form cannot build a path segment, so the select posts a query to
     /sessions and the server turns it into a path. With script the change
     event navigates first and the button is not wanted; without it, the button
     is the only way to submit.

     noscript rather than hiding it, because the first version did hide it and
     that was the defect. A control the server draws and the script removes is
     a control that can fail visible, and it did: the owner met a stale page
     carrying new markup beside old script, and found a full-size button under
     the dropdown that had never been in the wireframes. Clearing cookies made
     it go away, which is the same cache mismatch as MUS-F-0041 rather than a
     fix for anything.

     noscript has neither half of that problem. The browser decides, from
     whether scripting is enabled, and it decides at parse time — so a page
     whose script is stale, blocked, or never arrives still gets exactly the
     control it needs and nothing else. */
  .rail { display: flex; align-items: center; gap: .5rem; padding: .5rem 1rem;
          border-bottom: 1.4px solid var(--edge); min-width: 0; }
  /* flex-direction and padding are set here because they have to be undone,
     not because a row needs declaring. The bare form rule above was written
     for the composer (column, gap, its own padding) and a bare element
     selector reshapes every form added afterwards. This one came out stacked
     and centred inside 69px of nothing, which is exactly the giant button
     under the dropdown the owner reported. */
  .pick { display: flex; flex-direction: row; align-items: center; gap: .3rem;
          padding: 0; flex: 1; min-width: 0; }
  .pick select { flex: 1; min-width: 0; font: inherit; font-size: .85em; }
  /* Nothing sets display on the noscript, and that is deliberate.

     It had display: contents, to make the button a flex item of this row
     rather than the noscript being one. That override also cancels the rule
     every browser applies when scripting is enabled — noscript { display:
     none } — so the element's contents were shown, and the contents of a
     noscript with scripting on are its own markup as text. The row rendered
     the literal string <button type="submit" class="go">Go</button> next to
     the dropdown, which is what the owner would have seen.

     Without the override the browser decides again: hidden with scripting on,
     an ordinary inline element with it off. The row is a flex row either way,
     which is what actually put the button beside the select. */
  .pick .go { flex: 0 0 auto; font: inherit; font-size: .75em; line-height: 1;
              padding: .2rem .45rem; border: 1px solid var(--edge);
              border-radius: .35rem; background: none; color: inherit;
              opacity: .6; cursor: pointer; }

  /* The button that opens the drawer, and the ring that says something is
     running.

     A word rather than a glyph, because everything else on this surface is
     words and a glyph said nothing about what was behind it.

     The ring is a conic gradient with two bright points 180 degrees apart,
     painted once and turned with the rotate property. Not an animated
     gradient angle: that repaints the whole gradient every frame, where
     rotating a pre-painted layer is composited and costs a phone almost
     nothing — which is what makes it affordable on a page already streaming a
     terminal over a socket. Constant speed, because a rotation that eases
     reads as a stutter rather than as light travelling. No hard stops in the
     gradient, or the seam becomes an edge sweeping round the rim. */
  body { --drawer-w: 17rem; }
  .ring { position: relative; flex: 0 0 auto; display: inline-flex;
          padding: 1.4px; border-radius: 999px; overflow: hidden; }
  /* Two layers of the same token rather than a new colour: --accent-soft is
     12.5% alpha, which at one layer is too faint to read as a glow at all.
     Nothing here needs tuning per theme — --accent is a single mid-blue that
     carries on both, which is why there is no dark-mode branch. */
  .ring.live { box-shadow: 0 0 .75rem var(--accent-soft),
                           0 0 .25rem var(--accent-soft); }
  .ring.live::before {
      content: ""; position: absolute; left: 50%; top: 50%;
      width: 190%; aspect-ratio: 1; translate: -50% -50%;
      background: conic-gradient(from 0deg, transparent 0deg,
                  var(--accent) 40deg, transparent 95deg, transparent 180deg,
                  var(--accent) 220deg, transparent 275deg, transparent 360deg);
      animation: turn 3s linear infinite; }
  @keyframes turn { from { rotate: 0deg } to { rotate: 1turn } }
  /* An indefinite rotation is exactly the motion this setting exists for. The
     accent colour carries the state on its own, so holding still costs the
     movement and no information. */
  @media (prefers-reduced-motion: reduce) {
    .ring.live::before { animation: none; }
  }
  .toggle { position: relative; display: inline-flex; align-items: center;
            gap: .4rem; padding: .25rem .7rem; border: 1px solid var(--edge);
            border-radius: 999px; background: var(--paper); color: inherit;
            font: inherit; font-size: .82em; cursor: pointer;
            white-space: nowrap; }
  /* Whatever the ring is wrapped around loses its own border, or the rim and
     the border sit a pixel apart and read as two rings. Written for the
     sub-agent button and now also worn by the status pill, which is why it
     names a child rather than that button. */
  .ring.live > * { border-color: transparent; }
  .toggle[data-empty] { opacity: .5; }
  .badge { border: 1px solid var(--edge); border-radius: 999px;
           padding: 0 .4rem; font-size: .85em; }
  .ring.live .badge { border-color: var(--accent); background: var(--accent-soft); }

  /* The drawer.

     Shut by default, so the sub-agent list takes none of the screen until it
     is asked for — it is what squeezed the rail to 17px, the terminal to a
     line and the composer off the bottom of a phone (MUS-F-0035, MUS-F-0038).

     On a phone it opens over the terminal: at 390px a 17rem drawer would leave
     about 110px of it. On a wide screen it pushes instead, which is the whole
     reason for a drawer rather than a sheet — the terminal and the list at
     once. */
  .drawer[hidden] { display: none; }
  .drawer { position: fixed; inset: 0; z-index: 20; }
  .veil { position: absolute; inset: 0; background: #0007; }
  .panel { position: absolute; top: 0; right: 0; bottom: 0;
           width: 86%; max-width: 22rem; box-sizing: border-box;
           background: var(--paper); border-left: 1.4px solid var(--edge);
           display: flex; flex-direction: column; }
  .dhead { display: flex; align-items: center; gap: .5rem; padding: .7rem 1rem;
           border-bottom: 1.4px solid var(--edge); flex: 0 0 auto; }
  .dhead > strong { flex: 1; overflow: hidden; text-overflow: ellipsis;
                    white-space: nowrap; }
  .dhead > strong.untitled { opacity: .55; font-style: italic; }
  .dhead .count { opacity: .6; font-size: .85em; white-space: nowrap; }
  .back, .shut { background: none; border: 0; color: inherit; font: inherit;
                 cursor: pointer; padding: 0 .3rem; opacity: .7; }
  /* Its own scroller from the start, rather than after the measurement says
     8,211px again. */
  .dlist { flex: 1; min-height: 0; overflow-y: auto;
           overscroll-behavior: contain; padding: .2rem 1rem; }
  .dmeta { padding: .5rem 1rem; opacity: .6; font-size: .85em; flex: 0 0 auto;
           border-bottom: 1.4px solid var(--edge); }
  .dread { flex: 1; min-height: 0; overflow-y: auto;
           overscroll-behavior: contain; padding: .9rem 1rem;
           white-space: pre-wrap; word-break: break-word; }
  .dread.quiet { opacity: .6; }
  .agent { display: flex; align-items: baseline; gap: .5rem; padding: .5rem 0;
           white-space: nowrap; width: 100%; text-align: left; cursor: pointer;
           background: none; border: 0; border-bottom: 1px solid var(--edge);
           color: inherit; font: inherit; font-size: .9em; }
  .agent:last-of-type { border-bottom: 0; }
  .agent .more { opacity: .5; }
  .agent .what { flex: 1; overflow: hidden; text-overflow: ellipsis; }
  .agent .what.untitled { opacity: .55; font-style: italic; }
  .agent .pill { border: 1px solid var(--edge); border-radius: 999px;
                 padding: .05rem .5rem; font-size: .78em; }
  .agent .pill.done { border-color: var(--accent); background: var(--accent-soft); }
  .agent .age { opacity: .6; font-size: .82em; }
  /* Never drawn. Where the reading pane gets a sub-agent's final message from,
     so a tap is answered before the socket has sent a frame — the first paint
     is the server's. display:none, so it has no layout box. */
  .say { display: none; }

  /* Drag the drawer wider, on a wide screen only (IDW-F-0004).

     The grip is a real control rather than a decorated edge: focusable, with
     a separator role, and it moves on the arrow keys as well as the pointer.
     A drag handle that only answers a mouse is a drag handle half the people
     using it cannot reach.

     It only exists above 60rem. On a phone the drawer is 86% of the screen and
     there is nothing to widen it into. */
  .grip { display: none; }

  @media (min-width: 60rem) {
    /* Only its own column, so the terminal beside it stays clickable. */
    .drawer { inset: 0 0 0 auto; width: var(--drawer-w); }
    .veil { display: none; }
    .panel { width: var(--drawer-w); max-width: none; }
    .grip { display: block; position: absolute; left: -3px; top: 0; bottom: 0;
            width: 7px; cursor: col-resize; background: none; border: 0;
            padding: 0; z-index: 1; }
    .grip:hover, .grip:focus-visible { background: var(--accent-soft); }
    .grip:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
    /* While dragging, nothing else should be selecting text or handing the
       pointer to the terminal underneath. */
    body.dragging { user-select: none; cursor: col-resize; }
    /* The push, and the reason it needs saying out loud.

       The composer is placed by --shell-dock-left and --shell-dock-width
       rather than by flow, so it does not narrow with the content and would
       slide under the drawer. The reading column and the dock therefore take
       the same expression.

       min(), because the free space is usually already there: at 1366px the
       reading column is 736px with 406px empty beside it, so a 17rem drawer
       fits and nothing moves at all. It is only near 60rem, where that space
       runs out, that either of them narrows. */
    body.pushed { max-width: min(var(--shell-content, 46rem),
                    calc(100vw - var(--shell-dock-left) - var(--drawer-w) - var(--shell-gutter)));
                  --shell-dock-width: min(var(--shell-content, 46rem),
                    calc(100vw - var(--shell-dock-left) - var(--drawer-w) - var(--shell-gutter))); }
  }
` + shellCSS + `
</style>
</head>
<body data-project="{{.Project}}">
<header><strong>{{if .Project}}{{.Project}}{{else}}Sessions{{end}}</strong>
  <span class="ring" id="statering"><span class="pill" id="state">connecting</span></span>
  <span class="who">whippy-vm</span>{{if .ShowAccount}}<a class="acct" href="/account">Account</a>{{end}}</header>
{{if .Rows}}<div class="rail" id="rail">
  <form class="pick" method="get" action="/sessions">
    <select name="p" id="pick" aria-label="Session">
      {{range .Rows}}<option value="{{.Project}}"{{if .Here}} selected{{end}}>{{.Project}}</option>{{end}}
    </select><noscript><button type="submit" class="go">Go</button></noscript>
  </form>
  <span class="ring{{if .Running}} live{{end}}" id="ring"><button type="button" class="toggle" id="toggle"
    aria-expanded="false" aria-controls="drawer"{{if not .Subagents}} data-empty{{end}}>Sub-agents<span
    class="badge" id="badge"{{if not .Subagents}} hidden{{end}}>{{if .Running}}{{.Running}}{{else}}{{len .Subagents}}{{end}}</span></button></span>
</div>{{end}}
{{if .Missing}}
<p class="none">{{if .Project}}Mustur did not start a session for {{.Project}}, so there is nothing to show.{{else}}No sessions.{{end}}<br>
<small>A session left running in a terminal is not here and will not appear.</small><br>
<small><a href="/compose">Compose</a> still works: with nothing running it files to the idea inbox.</small></p>
{{else}}
<div class="chips" id="chips" hidden></div>
<div class="drawer" id="drawer" hidden>
  <div class="veil" id="veil"></div>
  <aside class="panel" role="dialog" aria-label="Sub-agents">
    <button type="button" class="grip" id="grip" role="separator"
      aria-orientation="vertical" aria-label="Resize the drawer"></button>
    <div class="dhead">
      <button type="button" class="back" id="back" hidden aria-label="Back to the list">&larr;</button>
      <strong id="dtitle">Sub-agents</strong>
      <small class="count" id="dcount">{{if .Subagents}}{{len .Subagents}}{{if .Running}} · {{.Running}} running{{end}}{{end}}</small>
      <button type="button" class="shut" id="shut" aria-label="Close">&times;</button>
    </div>
    <div class="dmeta" id="dmeta" hidden></div>
    <div class="dlist" id="dlist">
      {{if .Subagents}}{{range .Subagents}}<button type="button" class="agent" data-id="{{.ID}}">
      {{if .Title}}<span class="what">{{.Title}}</span>{{else}}<span class="what untitled">{{.Type}}</span>{{end}}
      <span class="pill{{if .Done}} done{{end}}">{{.State}}</span><span class="age">{{.For}}</span><span class="more">&rsaquo;</span>
    </button>{{if .Said}}<div class="say" data-for="{{.ID}}">{{.Said}}</div>{{end}}{{end}}{{else}}<p class="none">Nothing has been launched from this session.</p>{{end}}
    </div>
    <div class="dread" id="dread" hidden></div>
  </aside>
</div>
<pre id="out"></pre>
<div class="dock">
<div id="foot">quiet 0s</div>
<!-- The keys, above the composer where MUS-Q-0072 put them.

     Rendered only where the composer is, because they go down the same socket
     and are useless without it. They are buttons in a div rather than in the
     form: a button inside it submits it, which is the defect this row exists
     to be the opposite of.

     Escape first because getting off a dialog is the case that produced the
     question, the arrows grouped because they are one control, and Ctrl-C last
     and apart because it ends a turn. -->
<div class="keys" id="keys">
  <button type="button" data-key="escape">Esc</button>
  <span class="pad">
    <button type="button" data-key="left" aria-label="Left">&larr;</button
    ><button type="button" data-key="up" aria-label="Up">&uarr;</button
    ><button type="button" data-key="down" aria-label="Down">&darr;</button
    ><button type="button" data-key="right" aria-label="Right">&rarr;</button>
  </span>
  <button type="button" data-key="enter">Enter</button>
  <button type="button" data-key="cancel" class="stopish">Ctrl-C</button>
</div>
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
  <a href="/sessions" class="here" aria-label="Sessions"><i class="ic ic-sess"></i><span>Sessions</span></a>
  <a href="/questions" aria-label="Decisions"><i class="ic ic-dec">?</i><span>Decisions</span>{{if .OpenQuestions}}<em class="cnt">{{.OpenQuestions}}</em>{{end}}</a>
  <a href="/intake" aria-label="Intake"><i class="ic ic-in"><b></b></i><span>Intake</span></a>
  <a href="/records" aria-label="Records"><i class="ic ic-rec"></i><span>Records</span></a>
  {{if .ShowAccount}}<a class="me" href="/account" title="Account" aria-label="Account"><i class="ic ic-acc"></i></a>{{end}}
</nav>
{{if not .Missing}}<script src="/assets/session.js"></script>{{end}}
</body>
</html>
`))
