package web

// The session surface: a running session's output in a browser tab, and a way
// to answer it.
//
// **This is the one surface in v1 that carries a client layer.** Every other
// one is server-rendered with no script, no stylesheet, no font and no image,
// and that stays the rule — a live terminal simply cannot be server-rendered.
// The stack table names this as the exception so the rule is not quietly
// dropped, and the script here is the only script in the tree.
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

// IdleTimeout closes a socket nobody is using. A tab left open on a phone in a
// drawer should not hold a writable channel into an agent for a week.
const IdleTimeout = 30 * time.Minute

// Sessions serves the session surface.
type Sessions struct {
	Hub     *session.Hub
	Adapter *session.Adapter
	// Store is read only for the count the Decisions tab carries. Nil means the
	// tab renders without one.
	Store   *store.Store
	Project string
	Actor   string
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
	Project       string
	Rows          []sessionRow
	OpenQuestions int
	Missing       bool
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
	s.render(w, r, sessionPage{Project: project, Rows: rows, Missing: !found})
}

func (s *Sessions) render(w http.ResponseWriter, r *http.Request, p sessionPage) {
	if s.Store != nil {
		p.OpenQuestions = OpenCount(r.Context(), s.Store)
	}
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
	send := func(f frame) error {
		b, err := json.Marshal(f)
		if err != nil {
			return err
		}
		wctx, wcancel := context.WithTimeout(conn, 10*time.Second)
		defer wcancel()
		return c.Write(wctx, websocket.MessageText, b)
	}

	if err := send(frame{T: "hello", Alive: true, Seq: at, Quiet: quiet}); err != nil {
		return
	}
	if gap {
		if err := send(frame{T: "gap", Lost: from - at}); err != nil {
			return
		}
	}
	if len(backlog) > 0 {
		if err := send(frame{T: "out", Seq: at + int64(len(backlog)), Text: string(backlog)}); err != nil {
			return
		}
	}

	go s.readInput(conn, cancel, c, project, s.actor(r))

	idle := time.NewTimer(IdleTimeout)
	defer idle.Stop()

	for {
		select {
		case <-conn.Done():
			return
		case <-idle.C:
			_ = send(frame{T: "ended", Error: "idle"})
			return
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
		}
	}
}

// readInput carries what the viewer typed into the session.
//
// Ownership is re-checked on every message rather than only at connect: a
// socket opened against a live session must not keep writing to that project's
// name after the session ends and a different one is started under it.
func (s *Sessions) readInput(ctx context.Context, cancel func(), c *websocket.Conn, project, actor string) {
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
		if now := s.now(); now.Sub(last) < InputEvery {
			continue
		} else {
			last = now
		}
		live, err := s.Adapter.Alive(ctx, project)
		if err != nil || !live {
			return
		}
		if err := s.Adapter.Send(ctx, project, text); err != nil {
			return
		}
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
  body { font: 17px/1.5 system-ui, sans-serif; margin: 0; max-width: 46rem;
         margin-inline: auto; display: flex; flex-direction: column;
         min-height: 100vh; }
  header { display: flex; align-items: center; gap: .5rem; padding: .75rem 1rem;
           border-bottom: 1.4px solid var(--edge); white-space: nowrap; }
  header .pill { border: 1px solid var(--edge); border-radius: 999px;
                 padding: .1rem .55rem; font-size: .78em; }
  header .pill.on { border-color: var(--accent); background: var(--accent-soft); }
  header .who { margin-left: auto; opacity: .6; font-size: .82em; }
  /* Chrome and output are visually separate. Anything Mustur says about the
     session sits on a tinted strip; anything the session said is plain text. */
  .strip { display: flex; align-items: center; gap: .5rem; padding: .4rem 1rem;
           background: #8881; border-bottom: 1.4px solid var(--edge);
           font-size: .82em; opacity: .8; white-space: nowrap; }
  .strip .grow { flex: 1; overflow: hidden; text-overflow: ellipsis; }
  #out { flex: 1; padding: .8rem 1rem; margin: 0; white-space: pre-wrap;
         word-break: break-word; font-size: .9em; }
  #foot { padding: .4rem 1rem; background: #8881;
          border-top: 1.4px solid var(--edge); font-size: .82em; opacity: .75; }
  form { display: flex; gap: .5rem; padding: .7rem 1rem;
         border-top: 1.4px solid var(--edge); }
  input[type=text] { flex: 1; min-width: 0; font: inherit; padding: .55rem;
             border: 1px solid var(--edge); border-radius: .5rem;
             background: transparent; color: inherit; }
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
  nav { display: flex; border-top: 1.4px solid var(--edge); white-space: nowrap;
        margin-top: auto; }
  nav a { flex: 1; padding: .7rem .25rem; text-align: center; font-size: .85em;
          text-decoration: none; color: inherit; opacity: .6; }
  nav a.here { opacity: 1; font-weight: 600; }
</style>
</head>
<body data-project="{{.Project}}">
<header><strong>{{if .Project}}{{.Project}}{{else}}Sessions{{end}}</strong>
  <span class="pill" id="state">connecting</span>
  <span class="who">whippy-vm</span></header>
{{if .Rows}}<div class="rail">
  {{range .Rows}}<a href="/sessions/{{.Project}}"{{if .Here}} class="here"{{end}}>{{.Project}}</a>{{end}}
</div>{{end}}
{{if .Missing}}
<p class="none">{{if .Project}}Mustur did not start a session for {{.Project}}, so there is nothing to show.{{else}}No sessions.{{end}}<br>
<small>A session left running in a terminal is not here and will not appear.</small></p>
{{else}}
<div class="strip"><span class="grow" id="scrollback">connecting</span></div>
<pre id="out"></pre>
<div id="foot">quiet 0s</div>
<form id="say"><input type="text" id="text" placeholder="Reply to this session" autocomplete="off"><button type="submit">Send</button></form>
{{end}}
<nav>
  <a href="/sessions" class="here">Sessions</a>
  <a href="/questions">Decisions{{if .OpenQuestions}} · {{.OpenQuestions}}{{end}}</a>
  <a href="/intake">Intake</a>
</nav>
<script src="/assets/session.js"></script>
</body>
</html>
`))
