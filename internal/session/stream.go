package session

// Reading a session's output, and handing it to however many viewers are
// watching.
//
// Three things shape this file.
//
// **One reader per session, not per viewer.** `tmux pipe-pane` is opened when
// the first viewer arrives, and closed a while after the last one leaves — see
// LingerAfter, which exists because closing it immediately is what would make a
// dropped phone connection lose its place. Two tabs on the same session share
// one reader. That is what keeps the client flat as sessions multiply, which is
// a constraint every surface inherits rather than an optimisation.
//
// **The sequence number is a byte count.** Every byte the session has produced
// since the reader opened has an offset, and a viewer resuming says which
// offset it last saw. Replay is then a slice, a gap is arithmetic, and neither
// needs a per-line index. The buffer holds the last BufferBytes and nothing
// older, so a viewer away longer than that is told what it missed rather than
// shown a hole where it was.
//
// **Nothing here is a record.** The buffer is memory on the serving process:
// not addressable, not exported, not in records/. A session's output is not
// something a reader cites, and a session's exit is an event rather than a
// record either — it is reported on the surface and in the log, and written
// nowhere else.

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// BufferBytes is how much of a session's recent output is kept for replay.
//
// 256 KB is the owner's, on MUS-Q-0021. The "roughly an hour of ordinary agent
// output" this comment used to claim was never measured and is removed rather
// than defended; what is known is the size, and that a reader holds one buffer
// per session with a viewer attached.
const BufferBytes = 256 << 10

// LingerAfter is how long a reader stays open after the last viewer leaves.
//
// The plan said the reader closes when the last viewer does, and building it
// showed that is wrong for the case the milestone exists for. One owner with
// one phone *is* the last viewer: when the connection drops, closing the reader
// throws away the buffer and the byte offsets, so reconnecting a second later
// resumes from zero and replays nothing. "Survives a dropped phone connection"
// is precisely the thing that would not work.
//
// So the reader lingers. A reconnect inside this window is continuous; after
// it, the session is still running and the next viewer simply starts fresh from
// what the pane already holds.
var LingerAfter = 2 * time.Minute

// captureLines is how far back the seed reads the pane's scrollback. tmux keeps
// its own history; this asks for the part of it a viewer might plausibly scroll
// to, and the byte cap in BufferBytes is what actually bounds memory.
const captureLines = "-2000"

// overlapWindow is how much of the seed's tail is compared against the first
// bytes off the pipe, looking for the duplicate seam. Larger than any plausible
// overlap and small enough to compare cheaply once per reader.
const overlapWindow = 8 << 10

// pollEvery is how often a reader checks whether its session is still alive.
// The pipe closing is the usual signal; this catches the case where it does
// not, and is what makes an ended session say so rather than going quiet.
var pollEvery = 2 * time.Second

// Update is what a viewer receives.
type Update struct {
	// Text is output the session produced. Empty on an Ended update.
	Text string
	// Seq is the byte offset immediately after this text.
	Seq int64
	// Ended is set once, when the session is gone.
	Ended bool
	// ExitAt is when the reader noticed, on an Ended update.
	ExitAt time.Time
}

// Stream is one session's output, read once and fanned out.
type Stream struct {
	project string

	mu     sync.Mutex
	buf    []byte // The last BufferBytes of output.
	next   int64  // Byte offset just past everything ever produced.
	lastAt time.Time
	ended  bool
	endAt  time.Time
	subs   map[chan Update]struct{}
	refs   int

	linger *time.Timer
	stop   func()
	done   chan struct{}
}

// Quiet is how long since the session last produced output. It is time since
// the last byte and nothing more: a session waiting for input and a session
// thinking hard look identical from here, and a surface that claimed to tell
// them apart would be guessing.
func (s *Stream) Quiet(now time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastAt.IsZero() {
		return 0
	}
	return now.Sub(s.lastAt)
}

// Hub owns the readers, one per session being watched.
type Hub struct {
	Adapter *Adapter
	// Dir is where the reader's fifos are made. Empty means the OS temp dir.
	Dir string

	mu      sync.Mutex
	streams map[string]*Stream
}

// Sub is one viewer's attachment to a session.
type Sub struct {
	C      <-chan Update
	stream *Stream
	hub    *Hub
	ch     chan Update
}

// Attach starts watching a project's session, replaying from `from` — the byte
// offset the viewer last saw, or 0 for everything the buffer still holds.
//
// It refuses a session Mustur did not start, by the same ownership check every
// other path uses. A viewer cannot reach a session by naming it.
func (h *Hub) Attach(ctx context.Context, project string, from int64) (*Sub, []byte, int64, bool, error) {
	if _, err := NameFor(project); err != nil {
		return nil, nil, 0, false, err
	}
	live, err := h.Adapter.Alive(ctx, project)
	if err != nil {
		return nil, nil, 0, false, err
	}
	if !live {
		return nil, nil, 0, false, fmt.Errorf("%s has no session Mustur started", project)
	}

	h.mu.Lock()
	if h.streams == nil {
		h.streams = map[string]*Stream{}
	}
	s := h.streams[project]
	// A stream that has ended is not this session's, whatever it is keyed by.
	// A session stopped and restarted under the same project inside the linger
	// window used to hand the new viewer the dead one's buffer, labelled
	// running, with no reader open and no ended frame — a frozen replay of
	// something that was over. Discard it and read the live pane instead.
	if s != nil && s.hasEnded() {
		if s.linger != nil {
			s.linger.Stop()
			s.linger = nil
		}
		if s.stop != nil {
			s.stop()
		}
		delete(h.streams, project)
		s = nil
	}
	if s == nil {
		s = &Stream{project: project, subs: map[chan Update]struct{}{}, done: make(chan struct{})}
		h.streams[project] = s
		h.start(s)
	}
	// A viewer arriving inside the linger window keeps the reader — and with it
	// the buffer and the byte offsets a reconnect resumes from.
	if s.linger != nil {
		s.linger.Stop()
		s.linger = nil
	}
	s.refs++
	h.mu.Unlock()

	ch := make(chan Update, 64)
	s.mu.Lock()
	// Replay under the same lock that appends, so a viewer cannot miss a chunk
	// written between the snapshot and the subscription.
	backlog, at, gap := s.since(from)
	s.subs[ch] = struct{}{}
	s.mu.Unlock()

	return &Sub{C: ch, stream: s, hub: h, ch: ch}, backlog, at, gap, nil
}

// Close detaches one viewer, and stops the reader if it was the last.
func (sub *Sub) Close() {
	s := sub.stream
	s.mu.Lock()
	// Only close it if broadcast has not already, which it does to a viewer
	// that fell too far behind.
	if _, live := s.subs[sub.ch]; live {
		delete(s.subs, sub.ch)
		close(sub.ch)
	}
	s.mu.Unlock()

	h := sub.hub
	h.mu.Lock()
	s.refs--
	if s.refs <= 0 && s.linger == nil {
		// Not stopped, held. See LingerAfter: stopping here is what would make
		// a dropped phone connection lose the session's continuity.
		s.linger = time.AfterFunc(LingerAfter, func() {
			h.mu.Lock()
			stop := s.refs <= 0 && h.streams[s.project] == s
			if stop {
				delete(h.streams, s.project)
			}
			s.linger = nil
			h.mu.Unlock()
			if stop && s.stop != nil {
				s.stop()
			}
		})
	}
	h.mu.Unlock()
}

// Shutdown stops every reader. For a server going down, and for a test that
// wants the fifos gone rather than lingering.
func (h *Hub) Shutdown() {
	h.mu.Lock()
	streams := make([]*Stream, 0, len(h.streams))
	for _, s := range h.streams {
		if s.linger != nil {
			s.linger.Stop()
			s.linger = nil
		}
		streams = append(streams, s)
	}
	h.streams = nil
	h.mu.Unlock()

	for _, s := range streams {
		if s.stop != nil {
			s.stop()
			<-s.done
		}
	}
}

// since returns the buffered output from a byte offset, the offset it starts
// at, and whether output older than that was already dropped.
func (s *Stream) since(from int64) (out []byte, at int64, gap bool) {
	oldest := s.next - int64(len(s.buf))
	if from <= 0 || from < oldest {
		// Either a new viewer or one that was away too long. Both get what
		// exists; only the second is told something is missing.
		return append([]byte(nil), s.buf...), oldest, from > 0 && from < oldest
	}
	if from > s.next {
		// The viewer is ahead of this stream, which means it is not the stream
		// it was reading: the reader was restarted while it was away and the
		// byte offsets began again at zero. Silently handing it the new
		// stream's position loses everything between, which a review measured
		// at 5,550 lines with nothing said. It is a gap, and the honest answer
		// is that the size is unknown.
		return append([]byte(nil), s.buf...), oldest, true
	}
	if from == s.next {
		return nil, s.next, false
	}
	return append([]byte(nil), s.buf[from-oldest:]...), from, false
}

func (s *Stream) append(text []byte, at time.Time) {
	s.mu.Lock()
	s.buf = append(s.buf, text...)
	if len(s.buf) > BufferBytes {
		s.buf = append([]byte(nil), s.buf[len(s.buf)-BufferBytes:]...)
	}
	s.next += int64(len(text))
	s.lastAt = at
	u := Update{Text: string(text), Seq: s.next}
	s.broadcast(u)
	s.mu.Unlock()
}

func (s *Stream) hasEnded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ended
}

func (s *Stream) end(at time.Time) {
	s.mu.Lock()
	if !s.ended {
		s.ended, s.endAt = true, at
		s.broadcast(Update{Seq: s.next, Ended: true, ExitAt: at})
	}
	s.mu.Unlock()
}

// broadcast is called with the lock held.
//
// A viewer whose buffer is full is **disconnected**, not skipped. Skipping was
// the first attempt and it is worse than it looks: Go discards the new item, so
// the viewer keeps receiving a contiguous but ever-staler prefix with no
// sequence jump for anything to notice. A review measured a viewer ending 8 MB
// behind with zero holes and zero notice.
//
// Closing the channel ends that viewer's socket, and the client reconnects
// asking to resume from the offset it actually reached — which is what the old
// comment claimed happened and nothing implemented. One stalled tab still does
// not hold up the reader or the other viewers.
func (s *Stream) broadcast(u Update) {
	for ch := range s.subs {
		select {
		case ch <- u:
		default:
			delete(s.subs, ch)
			close(ch)
		}
	}
}

// start opens the reader for a session. tmux pipe-pane runs a shell command
// that receives the pane's output, so the command writes into a fifo this
// process reads. The fifo lives and dies with the reader.
func (h *Hub) start(s *Stream) {
	ctx, cancel := context.WithCancel(context.Background())
	s.stop = cancel

	go func() {
		defer close(s.done)
		defer func() {
			// Detach the pipe whatever happened, or tmux keeps writing into a
			// fifo nobody reads and the next viewer gets a stale one.
			off, offCancel := context.WithTimeout(context.Background(), 5*time.Second)
			name, _ := NameFor(s.project)
			_, _ = h.Adapter.runner().Run(off, "tmux", "pipe-pane", "-t", name)
			offCancel()
		}()

		var seedTail string

		dir := h.Dir
		if dir == "" {
			dir = os.TempDir()
		}
		fifo := filepath.Join(dir, fmt.Sprintf("mustur-%s-%d.fifo", s.project, os.Getpid()))
		_ = os.Remove(fifo)
		if err := syscall.Mkfifo(fifo, 0o600); err != nil {
			s.end(time.Now())
			return
		}
		defer os.Remove(fifo)

		name, err := NameFor(s.project)
		if err != nil {
			s.end(time.Now())
			return
		}
		// -o so the pipe stops if it is already piping somewhere else.
		if _, err := h.Adapter.runner().Run(ctx, "tmux", "pipe-pane", "-o", "-t", name,
			fmt.Sprintf("cat > %q", fifo)); err != nil {
			s.end(time.Now())
			return
		}

		go h.watch(ctx, s)

		// Seed with what is already on the pane.
		//
		// pipe-pane only carries output produced after it is enabled, so
		// without this the first viewer of a session that has been running for
		// an hour opens an empty screen and waits for the next line. That is
		// not "2,140 earlier lines"; it is a blank page that looks like a
		// hung session.
		//
		// capture-pane gives the scrollback tmux is already keeping. It is
		// taken once, before the first byte off the pipe, so the buffer reads
		// in order.
		if out, err := h.Adapter.runner().Run(ctx, "tmux",
			"capture-pane", "-p", "-J", "-S", captureLines, "-t", name); err == nil {
			if trimmed := strings.TrimRight(out, "\n \t"); trimmed != "" {
				seeded := trimmed + "\n"
				s.append([]byte(seeded), time.Now())
				// The seed and the pipe overlap: anything the pane printed
				// between capture-pane reading it and pipe-pane delivering it
				// arrives twice. A review saw 6-11 duplicated lines on four of
				// six sessions at 50 lines/s, invisible at 5. Keep the tail so
				// the first chunk off the pipe can have that overlap removed.
				seedTail = seeded
				if len(seedTail) > overlapWindow {
					seedTail = seedTail[len(seedTail)-overlapWindow:]
				}
			}
		}

		// Opening for read blocks until tmux's `cat` opens the write end, so
		// this is also how we learn the pipe is live.
		f, err := os.OpenFile(fifo, os.O_RDONLY, 0)
		if err != nil {
			s.end(time.Now())
			return
		}
		defer f.Close()
		go func() { <-ctx.Done(); f.Close() }()

		r := bufio.NewReader(f)
		chunk := make([]byte, 4096)
		first := true
		for {
			n, err := r.Read(chunk)
			if n > 0 {
				text := chunk[:n]
				if first {
					text = dropOverlap(seedTail, text)
					first = false
				}
				if len(text) > 0 {
					s.append(text, time.Now())
				}
			}
			if err != nil {
				return
			}
		}
	}()
}

// dropOverlap removes from next any prefix that duplicates a suffix of seed.
//
// capture-pane and pipe-pane both carry whatever the pane printed while the
// reader was being set up, so the join between them repeats a few lines. Line
// endings differ between the two — capture gives \n, the pipe gives \r\n — so
// the comparison is made on normalised copies and the cut is applied to the
// original.
func dropOverlap(seed string, next []byte) []byte {
	if seed == "" || len(next) == 0 {
		return next
	}
	norm := func(b []byte) []byte { return []byte(strings.ReplaceAll(string(b), "\r\n", "\n")) }
	ns, nn := norm([]byte(seed)), norm(next)

	max := len(ns)
	if len(nn) < max {
		max = len(nn)
	}
	for n := max; n > 0; n-- {
		if string(ns[len(ns)-n:]) != string(nn[:n]) {
			continue
		}
		// Walk the original forward until the same number of normalised bytes
		// has been consumed, so \r\n pairs are cut whole.
		consumed, i := 0, 0
		for i < len(next) && consumed < n {
			if next[i] == '\r' && i+1 < len(next) && next[i+1] == '\n' {
				i += 2
			} else {
				i++
			}
			consumed++
		}
		return next[i:]
	}
	return next
}

// watch is supervision, and it is the whole of it: notice that a session is
// gone and say so. It does not restart anything. An agent CLI that crashed
// wants a person, not a loop that starts it again into a fresh context having
// lost what it was doing.
func (h *Hub) watch(ctx context.Context, s *Stream) {
	t := time.NewTicker(pollEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			live, err := h.Adapter.Alive(ctx, s.project)
			if err != nil {
				continue
			}
			if !live {
				at := time.Now()
				s.end(at)
				// Recorded where an exit belongs: the surface and the log, and
				// nowhere in records/ (MUS-Q-0019). The claim that this is
				// "reported in the log" was in the file's own header before
				// anything logged anything.
				log.Printf("mustur: session %s%s ended at %s",
					Prefix, s.project, at.Format("2006-01-02 15:04:05"))
				return
			}
		}
	}
}
