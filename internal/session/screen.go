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
	// Activity is the line the CLI animates while a turn is in flight, taken
	// off the output so the dock can turn a spinner instead of a character
	// (MUS-F-0098). Nil when nothing is running.
	Activity *Activity
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

	mu       sync.Mutex
	last     Frame
	bodySum  [32]byte
	frameSum [32]byte
	// changedAt is when the screen last said something different. Measured on
	// the body with the CLI's furniture stripped off, so a turning spinner does
	// not reset it (MUS-F-0135).
	changedAt time.Time
	ended     bool
	subs      map[chan Frame]struct{}
	refs      int

	linger *time.Timer
	stop   func()
	done   chan struct{}

	// pinned marks a poller the hub keeps running whether or not anybody is
	// watching (MUS-Q-0105). Without it the linger teardown removes a poller
	// two minutes after the last tab closes, and the next supervise tick builds
	// a new one that has to guess when the screen last changed -- from tmux's
	// session_activity, which is not when the session last did anything
	// (MUS-F-0051). Keeping the poller is what keeps the dwell honest.
	pinned bool
}

// ensureLocked returns the poller for a project, starting one if there is none.
// Called with h.mu held, by the viewer path and by the hub's own supervision.
func (h *Hub) ensureLocked(ctx context.Context, project string) *pane {
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
	if p != nil {
		return p
	}
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
	//
	// Since MUS-Q-0105 the hub adopts every owned session, so for most panes
	// this seed is used once at startup and the poller maintains it after.
	p.changedAt = h.lastActive(ctx, project)
	h.panes[project] = p
	h.start(p)
	return p
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
	p := h.ensureLocked(ctx, project)
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
	if p.pinned {
		// The hub is polling this one for everybody, not for this viewer.
		return
	}
	p.linger = time.AfterFunc(LingerAfter, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if p.refs > 0 || p.pinned {
			return
		}
		p.shut()
		delete(h.panes, p.project)
	})
}

// SuperviseEvery is how often the hub looks for owned sessions it is not yet
// polling. Coarse on purpose: it decides how soon a session started elsewhere
// starts being watched, not how fresh any screen is, which is PollEvery's job.
var SuperviseEvery = 5 * time.Second

// Supervise keeps a poller on every session Mustur owns, watched or not.
//
// The owner's answer to MUS-Q-0105, and their own suggestion: a session runs in
// tmux from Start until something stops it, but the reader was started by a
// viewer and stopped two minutes after the last one left -- so nothing knew
// what an unwatched session was doing. The picker had to choose between saying
// nothing and paying a tmux capture per session on every page render.
//
// Polling all of them moves that cost from "per page load" to "per running
// session", which is the right way round: there are three sessions here and
// there can be a hundred page loads. What it gives up is LingerAfter's whole
// point, which was not polling a session nobody is reading. That is the trade
// the owner took, knowing it.
//
// It also gives MUS-D-0159's sweep an honest dwell for free: changedAt is now
// maintained continuously per session rather than seeded from tmux whenever a
// tab happens to open.
func (h *Hub) Supervise(ctx context.Context) {
	t := time.NewTicker(SuperviseEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.adopt(ctx)
		}
	}
}

// adopt starts a poller for every owned session and drops the ones that have
// gone.
func (h *Hub) adopt(ctx context.Context) {
	if h.Adapter == nil {
		return
	}
	live, err := h.Adapter.List(ctx)
	if err != nil {
		// tmux could not be asked. Nothing is adopted and nothing is dropped:
		// a listing that failed is not a machine with no sessions on it
		// (MUS-D-0062).
		return
	}
	running := make(map[string]bool, len(live))
	for _, sn := range live {
		running[sn.Project] = true
		h.mu.Lock()
		p := h.ensureLocked(ctx, sn.Project)
		if p != nil {
			p.pinned = true
		}
		h.mu.Unlock()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for project, p := range h.panes {
		if running[project] || p.refs > 0 {
			continue
		}
		// Gone, and nobody is holding it. A tab still open on a dead session
		// keeps its pane so the viewer is told it ended rather than finding an
		// empty page.
		p.shut()
		delete(h.panes, project)
	}
}

// Doing is what the CLI's own pane last said this session was up to.
//
// Read from the poller's last frame rather than captured on the spot, which is
// the whole of what MUS-Q-0105 buys: the picker can say which session is
// working and which is waiting without a tmux call per session per render. An
// unknown answer means nothing has polled it yet, not that it is idle.
func (h *Hub) Doing(project string) Agent {
	h.mu.Lock()
	p := h.panes[project]
	h.mu.Unlock()
	if p == nil {
		return AgentUnknown
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.last.Agent
}

// Quiet is how long this session's screen has been unchanged, and whether that
// is known at all. Read from the poller, which has been maintaining it since
// the session was adopted.
func (h *Hub) Quiet(project string, now time.Time) (time.Duration, bool) {
	h.mu.Lock()
	p := h.panes[project]
	h.mu.Unlock()
	if p == nil {
		return 0, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.changedAt.IsZero() {
		return 0, false
	}
	return now.Sub(p.changedAt), true
}

// Furniture is what the CLI's own status line last said, for a caller that
// needs to know whether an update is waiting or something is typed.
func (h *Hub) Furniture(project string) (Status, bool) {
	h.mu.Lock()
	p := h.panes[project]
	h.mu.Unlock()
	if p == nil {
		return Status{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.last.At.IsZero() {
		return Status{}, false
	}
	return p.last.Status, true
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

// Watching reports whether any viewer is holding this session open.
//
// The refcount the poller is already keeping, read rather than re-derived. The
// update sweep asks because the owner's threshold includes it: a session with a
// tab open on it is one somebody is reading, and it is not restarted under them
// (MUS-Q-0104). A lingering poller with no viewers left answers false, which is
// right -- linger is about not tearing down a reader somebody may come back to,
// not about somebody being there.
func (h *Hub) Watching(project string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	p, ok := h.panes[project]
	if !ok {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.refs > 0
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

	// Split before hashing, not after, which is the whole of MUS-F-0135.
	//
	// This used to take the sum over the capture and suppress a frame when it
	// matched. The capture includes the CLI's own status line, and that line
	// moves on its own -- a spinner turns, a token count ticks -- so a screen
	// that had said nothing for an hour produced a new frame several times a
	// second, and anything reading "the capture changed" as "the session did
	// something" was reading a spinner. Two sums now, because there are two
	// different questions and they had been answered by one.
	//
	// The CLI's own furniture comes off before anything is rendered, so the
	// output is what the session said and the hundred blank rows a tall pane
	// leaves above the input box become trailing blanks that trim away.
	body, st := SplitChrome(raw)
	// The live line is furniture too: it is the CLI redrawing one row to move a
	// glyph, and every redraw is a frame. Off the output, into the dock.
	body, act := SplitActivity(body)
	agent, prompt := DoingIn(raw), ReadPrompt(raw)

	// Did the *screen* change? Only the body counts, so this is what a dwell
	// can honestly be measured on (MUS-D-0159's sweep reads it).
	bodySum := sha256.Sum256([]byte(body))
	// Is there anything new to send? The chips and the dock are drawn from the
	// furniture, so a status line that moved is a frame worth sending even
	// though the screen did not change. Hashing the body alone here would have
	// frozen the chips.
	frameSum := sha256.Sum256([]byte(body + "\x00" + fmt.Sprint(st, act, agent, prompt)))

	p.mu.Lock()
	// The zero sum never matches a real capture, so the first read always
	// produces a frame — but it must not claim the screen changed just now,
	// because all that happened is that somebody started watching.
	first := p.frameSum == [32]byte{}
	if !first && frameSum == p.frameSum {
		last := p.last
		p.mu.Unlock()
		return last
	}
	p.frameSum = frameSum
	if first {
		if p.changedAt.IsZero() {
			p.changedAt = now
		}
	} else if bodySum != p.bodySum {
		p.changedAt = now
	}
	p.bodySum = bodySum
	f := Frame{
		HTML:     ansi.HTML(trimBlank(body)),
		Status:   st,
		Activity: act,
		Agent:    agent,
		// Read from the raw capture rather than from body: SplitChrome takes
		// the CLI's own furniture off, and a dialog's legend is furniture by
		// every test that function applies.
		Prompt: prompt,
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
