package session

// Reading a session's output, and handing it to however many viewers are
// watching.
//
// Three things shape this file.
//
// **One reader per session, not per viewer.** `tmux pipe-pane` is opened when
// the first viewer arrives and closed when the last one leaves. Two tabs on the
// same session share it. That is what keeps the client flat as sessions
// multiply, which is a constraint every surface inherits rather than an
// optimisation.
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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// BufferBytes is how much of a session's recent output is kept for replay.
// Roughly an hour of ordinary agent output, and small enough that several
// sessions cost nothing on a machine with 1.5 GB free.
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
	delete(s.subs, sub.ch)
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

	close(sub.ch)
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
	if from >= s.next {
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

func (s *Stream) end(at time.Time) {
	s.mu.Lock()
	if !s.ended {
		s.ended, s.endAt = true, at
		s.broadcast(Update{Seq: s.next, Ended: true, ExitAt: at})
	}
	s.mu.Unlock()
}

// broadcast is called with the lock held. A viewer whose buffer is full is
// skipped rather than waited for: one stalled tab must not hold up the reader
// or the other viewers, and the viewer will resume from its own sequence.
func (s *Stream) broadcast(u Update) {
	for ch := range s.subs {
		select {
		case ch <- u:
		default:
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
			"capture-pane", "-p", "-J", "-S", "-2000", "-t", name); err == nil {
			if trimmed := strings.TrimRight(out, "\n \t"); trimmed != "" {
				s.append([]byte(trimmed+"\n"), time.Now())
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
		for {
			n, err := r.Read(chunk)
			if n > 0 {
				s.append(chunk[:n], time.Now())
			}
			if err != nil {
				return
			}
		}
	}()
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
				s.end(time.Now())
				return
			}
		}
	}
}
