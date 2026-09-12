package session

// Taking a CLI update by restarting the session that announced it.
//
// This is the first thing Mustur does to a session without being pressed, and
// the only one. MUS-D-0149 says a session lost with the machine waits for a
// person, and that stands: a CLI that died said nothing about why, so nothing
// here can know that starting it again is right. MUS-D-0159 is the narrow
// exception the owner granted — a CLI that has printed "Update installed ·
// Restart to update" has said what it wants, and a CLI sitting at an empty
// prompt is not doing anything that a restart interrupts.
//
// Every condition below is one the owner set on MUS-Q-0104, and each one exists
// because the sweep would otherwise destroy something:
//
//   - a turn in flight — read off the pane, not timed (MUS-D-0130)
//   - a browser tab open on the session, or a terminal attached to it — either
//     way somebody is there, and the owner's clause is about presence rather
//     than about which client they used
//   - a line typed into the pane's own box and not sent — read as a boolean and
//     never as text. Note what this is not: a draft written in Mustur's
//     composer lives in the browser until Send, so a restart cannot destroy it
//     and never could. This catches a line typed by somebody attached in a
//     terminal, which is the only way text sits in that box unsent
//   - a screen that moved inside the dwell — the turn ended, and the person who
//     asked for it has not read the answer yet
//
// None of it is a guess and none of it is a clock standing in for a fact,
// except the dwell, which is the owner's number.

import (
	"context"
	"log/slog"
	"time"
)

// Quiet is how long a session's screen must have been unchanged before an
// update is taken. The owner's number, on MUS-Q-0104.
//
// It is not a guard against interrupting a turn — the pane says whether one is
// in flight and that is exact. It is the gap between a turn ending and the
// person who asked for it reading the answer, which nothing can observe, so it
// is a number somebody chose rather than one anything measured.
var Quiet = 30 * time.Minute

// SweepEvery is how often the sweep looks. Derived from Quiet rather than
// chosen beside it: a sweep coarser than the dwell would round it up by its own
// interval, and one much finer buys nothing but captures.
func SweepEvery() time.Duration { return Quiet / 30 }

// Recaller reads back what Start wrote down.
//
// The narrow half of the store this needs, as an interface for the same reason
// Rememberer is one: this package shells out to tmux and nothing else, and a
// caller with no store passes nothing and sweeps nothing.
type Recaller interface {
	Remembered(ctx context.Context, project string) (dir, cmd string, ok bool)
}

// Watcher is what the hub knows about a session it is polling.
//
// Since MUS-Q-0105 the hub keeps a poller on every owned session, so everything
// the sweep needs about a screen is already in memory and captured once for all
// readers rather than once per reader. The sweep does no tmux capture of its
// own any more; a nil Watcher means nothing is being polled, and the sweep then
// restarts nothing, which is the right answer for a server with no session
// surface.
type Watcher interface {
	// Watching reports whether a browser tab is holding this session open.
	Watching(project string) bool
	// Doing is what the CLI's own pane last said the session was up to.
	Doing(project string) Agent
	// Quiet is how long the screen has been unchanged, and whether that is
	// known at all. Not known means the poller has not seen it twice yet.
	Quiet(project string, now time.Time) (time.Duration, bool)
	// Furniture is what the CLI's status line last said.
	Furniture(project string) (Status, bool)
}

// A Sweeper restarts idle sessions that have announced a CLI update.
type Sweeper struct {
	Adapter *Adapter
	Recall  Recaller
	Watch   Watcher
	Log     *slog.Logger

	// Now is the clock, injectable so a test does not wait half an hour.
	Now func() time.Time
}

func (s *Sweeper) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Sweeper) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

// Run sweeps until the context is cancelled.
func (s *Sweeper) Run(ctx context.Context) {
	t := time.NewTicker(SweepEvery())
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Sweep(ctx)
		}
	}
}

// Sweep looks once at every session Mustur owns.
//
// Errors are logged and the sweep continues: one unreadable pane must not stop
// the others being looked at, and there is nobody watching this to be told.
func (s *Sweeper) Sweep(ctx context.Context) {
	if s.Adapter == nil {
		return
	}
	live, err := s.Adapter.List(ctx)
	if err != nil {
		// With tmux unanswering nothing is restarted, which is the same answer
		// every other path gives to the same silence (MUS-D-0062).
		return
	}
	for _, sn := range live {
		// Attached is somebody sitting in the session in a terminal. The
		// owner's clause was "no browser tab open on it", and a terminal is the
		// same presence reached another way -- tmux already reports it, so
		// refusing on it costs nothing and asking only about tabs would have
		// restarted a session with a person looking straight at it.
		if sn.Attached {
			continue
		}
		s.consider(ctx, sn.Project)
	}
}

func (s *Sweeper) consider(ctx context.Context, project string) {
	if s.Watch == nil {
		return // nothing is polling, so nothing is known
	}
	st, seen := s.Watch.Furniture(project)
	if !seen {
		return // the poller has not read this session yet
	}
	if st.Update == "" {
		return // nothing to take
	}
	if s.Watch.Doing(project) != AgentWaiting {
		return // a turn is in flight, or the pane has not started
	}
	if st.Typed {
		return // a line typed and not sent, which a restart would destroy
	}
	if s.Watch.Watching(project) {
		return // somebody has it open
	}
	// The dwell comes from the poller, which measures it on the body with the
	// CLI's furniture stripped off -- so a turning spinner does not reset it
	// (MUS-F-0135). Not known means the screen has not been read twice yet,
	// which is not the same as unchanged, and it waits.
	quiet, known := s.Watch.Quiet(project, s.now())
	if !known || quiet < Quiet {
		return
	}
	s.restart(ctx, project, st.Update)
}

// restart ends the session and starts it again where it was, on its own
// conversation.
//
// Not Stop followed by the restore path: Stop forgets the session, which is
// what makes the restore list mean "went without being told to" (MUS-D-0149).
// A session taken for an update was told to, and the row has to survive, so the
// tmux session is killed directly and Start writes the row again.
func (s *Sweeper) restart(ctx context.Context, project, notice string) {
	if s.Recall == nil {
		return
	}
	dir, cmd, ok := s.Recall.Remembered(ctx, project)
	if !ok || dir == "" || cmd == "" {
		// Nothing written down is nothing to start again. A session Mustur
		// started always has a row; one that does not is not ours to guess at.
		s.log().Warn("update not taken: nothing remembered", "project", project)
		return
	}
	if err := s.Adapter.Kill(ctx, project); err != nil {
		s.log().Error("update not taken: session would not stop", "project", project, "err", err)
		return
	}
	if _, err := s.Adapter.Start(ctx, project, dir, cmd); err != nil {
		// The session is gone and did not come back. Loud, because this is the
		// one failure mode of the whole feature that costs the owner something
		// they did not ask to lose, and the restore list is where it lands.
		s.log().Error("session stopped for an update and did not start again",
			"project", project, "dir", dir, "err", err)
		return
	}
	s.log().Info("session restarted to take a CLI update",
		"project", project, "notice", notice)
}
