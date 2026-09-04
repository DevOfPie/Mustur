package session

// Watching a session by reading the screen tmux has already assembled.
//
// This replaces a byte stream. Mustur used to open `tmux pipe-pane` and append
// the pane's raw output to a log, and MUS-F-0049 established what was wrong
// with that: a third of those bytes are cursor addressing — ESC[21;3H, ESC[H,
// ESC[K — which is a protocol for painting a grid, not a transcript. Appending
// it linearly stacks partial frames on top of each other, and stripping the
// codes cannot help, because ESC[21;3H means the text after it *overwrites*
// row 21. Remove the code and you keep the text and lose where it goes.
//
// tmux is a terminal emulator and had already done the work. So this asks it
// for the screen instead of the protocol, on a timer, and sends a frame when
// the screen has changed. The owner chose it on MUS-Q-0060.
//
// **Three things fall out of the change, and they are the point.**
//
// There is no byte offset any more, so no replay, no gap message and no 256KB
// buffer. A viewer that reconnects is sent the current screen, which is the
// whole of what resuming means when the unit is a frame. MUS-Q-0021's buffer
// answered a question this no longer asks.
//
// There is no pipe. That takes MUS-F-0030 and MUS-F-0043 with it: a service
// that could not be stopped while piping, and a dead Mustur that held its
// listening port for as long as its pipe was running.
//
// And the agent's own state comes free. The poller already has the pane in
// hand, so whether a turn is in flight is read from the same capture rather
// than from a second one every two seconds.
//
// **What it costs.** One `tmux capture-pane` per watched project per tick,
// rather than one long-lived pipe. On this machine that is a few milliseconds
// of subprocess two and a half times a second while somebody is looking, and
// nothing at all when nobody is.

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/DevOfPie/Mustur/internal/ansi"
)

// ScreenLines is how much of the pane's history each frame carries above the
// visible screen.
//
// Measured on a pane that had genuinely scrolled: the visible screen alone is
// about 1.2KB, 120 lines is about 7KB and the whole history is 42KB. A frame
// is sent only when the screen changes, so the cost is per change rather than
// per tick — but a deep capture on a busy session is a real number, and 120
// lines is enough to scroll back through what an agent just did without
// sending its entire afternoon every time a spinner turns.
const ScreenLines = "-120"

// PollEvery is how often a watched pane is read.
//
// Fast enough that a session reads as live, slow enough that it is a few
// milliseconds of subprocess rather than a busy loop. Nothing polls a project
// nobody is watching.
var PollEvery = 400 * time.Millisecond

// LingerAfter is how long a poller keeps running after the last viewer leaves.
//
// The reason is unchanged from when this watched a pipe: one owner with one
// phone *is* the last viewer, and a dropped connection that tore the watcher
// down would make reconnecting a second later start from nothing.
var LingerAfter = 2 * time.Minute

// Frame is one rendering of a pane.
type Frame struct {
	// HTML is the screen, already escaped and with its colours resolved.
	HTML string
	// Agent is what the pane says the CLI is doing, read from the same capture.
	Agent Agent
	// Status is what the CLI's own furniture said, once it was taken off.
	Status Status
	// Prompt is a selection the CLI is waiting on, read off the same capture,
	// or nil when there is nothing to read. Nil is the ordinary case and the
	// one the design rests on: no legend means no controls and the terminal is
	// untouched (MUS-D-0142).
	Prompt *Prompt
	// At is when this screen was captured.
	At time.Time
	// Ended is set once, on the last frame, when the session is gone.
	Ended bool
	// ExitAt is when the poller noticed, on an Ended frame.
	ExitAt time.Time
}

// Hub owns one poller per session being watched.
type Hub struct {
	Adapter *Adapter

	mu    sync.Mutex
	panes map[string]*pane
}

// Sub is one viewer's attachment.
type Sub struct {
	C <-chan Frame

	pane *pane
	hub  *Hub
	ch   chan Frame
}

type pane struct {
	project string

	mu        sync.Mutex
	last      Frame
	sum       [32]byte
	changedAt time.Time
	ended     bool
	subs      map[chan Frame]struct{}
	refs      int

	linger *time.Timer
	stop   func()
	done   chan struct{}
}

// Watch starts reading a project's pane and returns the screen as it stands.
//
// It refuses a session Mustur did not start, by the same ownership check every
// other path uses: a viewer cannot reach a session by naming it.
func (h *Hub) Watch(ctx context.Context, project string) (*Sub, Frame, error) {
	if _, err := NameFor(project); err != nil {
		return nil, Frame{}, err
	}
	live, err := h.Adapter.Alive(ctx, project)
	if err != nil {
		return nil, Frame{}, err
	}
	if !live {
		return nil, Frame{}, fmt.Errorf("%s has no session Mustur started", project)
	}

	h.mu.Lock()
	if h.panes == nil {
		h.panes = map[string]*pane{}
	}
	p := h.panes[project]
	// A pane that has ended is not this session's, whatever it is keyed by. A
	// session stopped and restarted under the same project inside the linger
	// window used to hand the new viewer the dead one's last screen.
	if p != nil && p.hasEnded() {
		p.shut()
		delete(h.panes, project)
		p = nil
	}
	if p == nil {
		p = &pane{project: project, subs: map[chan Frame]struct{}{}, done: make(chan struct{})}
		// Give it a height worth scrolling. Once per poller rather than per
		// tick: it costs two tmux calls and the CLI redraws when it changes.
		if err := h.Adapter.Fit(ctx, project); err != nil {
			// Not fatal. A pane that could not be resized is a short pane, not
			// an unreadable one.
			log.Printf("session %s: %v", project, err)
		}
		// Seeded from tmux, or the first frame would say a session silent since
		// Sunday had just this moment moved.
		//
		// This is MUS-F-0042 in its third form. The poller learns when the
		// screen changed by watching it change, which is the right answer for
		// every moment after the first and no answer at all for the first —
		// exactly the case the counter exists for. tmux has known all along:
		// session_activity is already read by List for the route row's default.
		p.changedAt = h.lastActive(ctx, project)
		h.panes[project] = p
		h.start(p)
	}
	if p.linger != nil {
		p.linger.Stop()
		p.linger = nil
	}
	p.refs++
	h.mu.Unlock()

	// The first frame is taken here rather than waited for, so a viewer sees
	// the session immediately instead of a blank pane for up to a tick.
	now := p.read(ctx, h.Adapter, time.Now())

	ch := make(chan Frame, 8)
	p.mu.Lock()
	p.subs[ch] = struct{}{}
	p.mu.Unlock()

	return &Sub{C: ch, pane: p, hub: h, ch: ch}, now, nil
}

// Quiet is how long since the screen last changed.
//
// Time since the pane last looked different and nothing more. Whether the
// session is waiting for input or thinking hard is not knowable from this —
// Frame.Agent answers that, from the CLI's own status line.
func (sub *Sub) Quiet(now time.Time) time.Duration {
	if sub == nil || sub.pane == nil {
		return 0
	}
	sub.pane.mu.Lock()
	defer sub.pane.mu.Unlock()
	if sub.pane.changedAt.IsZero() {
		return 0
	}
	return now.Sub(sub.pane.changedAt)
}

// Close detaches one viewer, and lets the poller linger if it was the last.
func (sub *Sub) Close() {
	p := sub.pane
	p.mu.Lock()
	if _, still := p.subs[sub.ch]; still {
		delete(p.subs, sub.ch)
		close(sub.ch)
	}
	p.mu.Unlock()

	h := sub.hub
	h.mu.Lock()
	defer h.mu.Unlock()
	p.refs--
	if p.refs > 0 || p.linger != nil {
		return
	}
	p.linger = time.AfterFunc(LingerAfter, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if p.refs > 0 {
			return
		}
		p.shut()
		delete(h.panes, p.project)
	})
}

// lastActive asks tmux when this session last did anything. A zero time means
// tmux could not say, and Quiet treats that as "no idea" rather than "just now".
func (h *Hub) lastActive(ctx context.Context, project string) time.Time {
	if h.Adapter == nil {
		return time.Time{}
	}
	sessions, err := h.Adapter.List(ctx)
	if err != nil {
		return time.Time{}
	}
	for _, s := range sessions {
		if s.Project == project {
			return s.Activity
		}
	}
	return time.Time{}
}

// Shutdown stops every poller. Used when the server is going down.
func (h *Hub) Shutdown() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, p := range h.panes {
		if p.linger != nil {
			p.linger.Stop()
		}
		p.shut()
		delete(h.panes, k)
	}
}

// start runs the poller for one pane.
func (h *Hub) start(p *pane) {
	ctx, cancel := context.WithCancel(context.Background())
	p.stop = cancel
	go func() {
		defer close(p.done)
		t := time.NewTicker(PollEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if p.read(ctx, h.Adapter, time.Now()).Ended {
					return
				}
			}
		}
	}()
}

// read captures the pane once and broadcasts it if anything changed.
func (p *pane) read(ctx context.Context, a *Adapter, now time.Time) Frame {
	raw, err := a.Capture(ctx, p.project, ScreenLines)
	if err != nil {
		// A capture failing is not a death on its own — tmux can be busy, and
		// a context can be cancelled. Ask before declaring one.
		if live, aliveErr := a.Alive(ctx, p.project); aliveErr == nil && !live {
			return p.end(now)
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.last
	}

	sum := sha256.Sum256([]byte(raw))
	p.mu.Lock()
	// The zero sum never matches a real capture, so the first read always
	// produces a frame — but it must not claim the screen changed just now,
	// because all that happened is that somebody started watching.
	first := p.sum == [32]byte{}
	if !first && sum == p.sum {
		// Nothing moved. The agent's state can still have changed underneath a
		// screen that looks the same, but it cannot: it is read out of this
		// same text.
		last := p.last
		p.mu.Unlock()
		return last
	}
	p.sum = sum
	if !first || p.changedAt.IsZero() {
		p.changedAt = now
	}
	// The CLI's own furniture comes off before anything is rendered, so the
	// output is what the session said and the hundred blank rows a tall pane
	// leaves above the input box become trailing blanks that trim away.
	body, st := SplitChrome(raw)
	f := Frame{
		HTML:   ansi.HTML(trimBlank(body)),
		Status: st,
		Agent:  DoingIn(raw),
		// Read from the raw capture rather than from body: SplitChrome takes
		// the CLI's own furniture off, and a dialog's legend is furniture by
		// every test that function applies.
		Prompt: ReadPrompt(raw),
		At:     now,
	}
	p.last = f
	p.broadcast(f)
	p.mu.Unlock()
	return f
}

// end marks the pane dead and tells everyone watching, once.
func (p *pane) end(now time.Time) Frame {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ended {
		return p.last
	}
	p.ended = true
	f := p.last
	f.Ended, f.ExitAt = true, now
	p.last = f
	p.broadcast(f)
	return f
}

// broadcast sends to every viewer. Called with the lock held.
//
// A viewer that cannot keep up is dropped rather than blocking the poller: one
// stalled socket must not stop every other tab on the same session.
func (p *pane) broadcast(f Frame) {
	for ch := range p.subs {
		select {
		case ch <- f:
		default:
			delete(p.subs, ch)
			close(ch)
		}
	}
}

func (p *pane) hasEnded() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ended
}

// shut stops the poller and closes every viewer's channel.
func (p *pane) shut() {
	if p.stop != nil {
		p.stop()
	}
	p.mu.Lock()
	for ch := range p.subs {
		delete(p.subs, ch)
		close(ch)
	}
	p.mu.Unlock()
}

// trimBlank drops the blank lines tmux pads the capture out to the pane's
// height with, and the spaces it pads each line out to the pane's width with.
//
// Both matter more on a phone than they look. The pane is 80 columns and the
// screen is not: a line ending in seventy spaces wraps into two or three empty
// lines of its own at 390px, so the padding of one status bar becomes a hole
// in the middle of the output. Trailing spaces carry nothing — a background
// that ends a line ends it at the last character anyone can see.
func trimBlank(raw string) string {
	lines := strings.Split(raw, "\n")
	end := len(lines)
	for end > 0 && strings.TrimSpace(stripSGR(lines[end-1])) == "" {
		end--
	}
	// And from the top. Which end the padding lands on depends on where the CLI
	// anchors what it is drawing: a transcript grows downward and leaves the gap
	// below, a modal is pinned to the bottom of the pane and leaves it above.
	// Trimming one end only left a session with a dialogue open showing seven
	// hundred pixels of nothing and its content at the very bottom.
	start := 0
	for start < end && strings.TrimSpace(stripSGR(lines[start])) == "" {
		start++
	}
	// And collapse the middle. A tall pane draws the transcript at the top and
	// pins whatever is anchored — an input box, a dialogue — to the bottom, so
	// what is between them is a hundred rows of nothing. Trimming the ends
	// cannot reach it, because it is not at an end.
	//
	// Two blank lines is a paragraph break and anything past that is padding.
	// No transcript has ever meant a hundred of them.
	kept := make([]string, 0, end-start)
	blanks := 0
	for _, line := range lines[start:end] {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(stripSGR(line)) == "" {
			blanks++
			if blanks > maxBlankRun {
				continue
			}
		} else {
			blanks = 0
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// maxBlankRun is how many blank lines in a row survive. Two is a paragraph
// break; more is the pane's own padding.
const maxBlankRun = 2

// stripSGR removes escape sequences so a line of pure colour codes counts as
// blank, and so the CLI's furniture can be recognised by what it says.
//
// It delegates rather than approximating. The version it replaces scanned to
// the next "m", which is right for a colour and wrong for everything else: an
// OSC 8 hyperlink has no "m" in it, so "PR #31" — which the CLI links — was
// eaten as far as the next colour code and came out as a piece of its own URL.
// That corrupted the status chips and, worse, the divider and caret detection
// that decides which lines are furniture at all.
func stripSGR(line string) string { return ansi.Plain(line) }
