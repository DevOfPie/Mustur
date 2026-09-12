package web

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DevOfPie/Mustur/internal/session"
	"github.com/DevOfPie/Mustur/internal/store"
)

// DefaultCommands is what the surface offers to start when nothing else is
// configured.
//
// One entry, and it names a vendor. MUS-D-0091 allows that where a capability
// belongs to one vendor, and this is one of those: the adapter already reads
// this CLI's status line for the running pill and its dialogs for the prompt
// pop-up, so a session running something else is watched by a surface that
// understands none of it. `--session-cmd` replaces the list for a deployment
// that runs another.
var DefaultCommands = []string{"claude"}

// A startable is a checkout on this machine that a session can be started in.
//
// Repositories rather than projects, which is a reading of MUS-Q-0079's answer
// and worth saying: the owner asked for "a project from the routing records",
// and the directory a session needs is on the repository record — a project
// record carries a prefix and a name, and the idea inbox is a project with no
// checkout at all. Offering repositories is what makes the directory something
// the store already knows rather than something a browser types.
type startable struct {
	ID    string
	Title string
	Dir   string
}

// startables reads the repositories the store holds a checkout path for.
//
// The field's key names the machine — "Checkout on MUS-H-0001" — so it is
// matched by prefix. A repository with no checkout is not offered: there is
// nowhere to run.
func (s *Sessions) startables(ctx context.Context) []startable {
	if s.Store == nil {
		return nil
	}
	all, err := s.Store.List(ctx, "")
	if err != nil {
		return nil
	}
	var out []startable
	for _, r := range all {
		if r.Kind != "repository" {
			continue
		}
		for _, f := range r.Data {
			if !strings.HasPrefix(strings.TrimSpace(f.Key), "Checkout on") {
				continue
			}
			if dir := expandHome(strings.TrimSpace(f.Value)); dir != "" {
				out = append(out, startable{ID: r.ID, Title: r.Title, Dir: dir})
			}
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

// expandHome turns a stored ~ into this account's home. Records are written to
// be read by a person, and a person writes ~.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
	}
	return p
}

// placeOf names the tree a session was started in, for a label a person can
// tell two sessions apart by.
//
// The repository record's title when the directory is one this machine holds a
// checkout of, and the directory's own last segment when it is not — a session
// started somewhere no record knows about still has a place, and saying nothing
// about it would be the one case the label is for. Empty only when nothing was
// remembered about where it ran.
//
// Two sessions can share one working tree (MUS-F-0117), so this narrows what
// has to be read off the name rather than replacing it.
func placeOf(dir string, repos []startable) string {
	if dir == "" {
		return ""
	}
	for _, rp := range repos {
		if rp.Dir == dir {
			return rp.Title
		}
	}
	return filepath.Base(dir)
}

// places maps every session Mustur has started to where it was started.
//
// Read from the same table the lost list is read from: Start writes the project,
// the directory and the command down (MUS-D-0149) and the row survives until the
// session is deliberately stopped, so a *running* session's directory is already
// in the store and was being read and thrown away. No extra tmux call, and no
// per-session capture.
func (s *Sessions) places(ctx context.Context) map[string]string {
	if s.Store == nil {
		return nil
	}
	remembered, err := s.Store.RememberedSessions(ctx)
	if err != nil || len(remembered) == 0 {
		return nil
	}
	repos := s.startables(ctx)
	out := make(map[string]string, len(remembered))
	for _, r := range remembered {
		if where := placeOf(r.Dir, repos); where != "" {
			out[r.Project] = where
		}
	}
	return out
}

// A lostRow is a session Mustur started that is no longer running, offered back.
//
// "Lost" rather than "stopped", and the distinction is the whole feature: a
// session the owner ended is deleted from the store when it is ended, so what
// is left here went without being told to — the machine rebooted, the tmux
// server died, something killed the pane. Those are the ones worth offering.
type lostRow struct {
	Project string
	Dir     string
	// Where is the tree it ran in, named for the picker. The owner's point:
	// with one project every name is unambiguous, and with two nothing but a
	// perfectly chosen name tells them apart (MUS-F-0108).
	Where string
	// Resumes reports whether the conversation comes back with the session.
	// False means there is nothing on disk to bring back — the CLI never said
	// what its conversation was called, or said so and never wrote the file —
	// and the session starts again in the same place and empty. The page says
	// which, rather than promising a transcript it cannot fetch.
	Resumes bool
	When    string
	// Here marks the one being looked at, so the picker shows it selected the
	// way a running session does.
	Here bool
}

// lost is what was started and is not running, given what is.
//
// The subtraction only means anything against a live list, so the caller passes
// one it has already asked tmux for; with no answer there is no call to make
// here at all, because every remembered session would look missing and a page
// offering to start six that are all already running is worse than a page
// offering none.
func (s *Sessions) lost(ctx context.Context, running map[string]bool, here string) []lostRow {
	if s.Store == nil {
		return nil
	}
	remembered, err := s.Store.RememberedSessions(ctx)
	if err != nil || len(remembered) == 0 {
		return nil
	}
	now := s.now()
	repos := s.startables(ctx)
	var out []lostRow
	for _, r := range remembered {
		if running[r.Project] {
			continue
		}
		when := ""
		if !r.Started.IsZero() {
			when = since(now.Sub(r.Started)) + " ago"
		}
		out = append(out, lostRow{
			Project: r.Project, Dir: r.Dir,
			Where:   placeOf(r.Dir, repos),
			Resumes: resumes(r) != r.Cmd,
			When:    when,
			Here:    r.Project == here,
		})
	}
	return out
}

// resumes is the command that starts a session again, with its conversation
// where there is one to have.
//
// The transcript is looked for on disk rather than assumed from the identifier.
// The hook that reports one fires as the CLI starts and the file is not written
// until the conversation has something in it, so a session started and never
// spoken to has an identifier naming nothing — and `--resume` on that is a
// command that exits at once, which arrives as "the session died on startup"
// two steps from anything that explains it. Better to bring the session back
// empty, which is what it was.
//
// Deliberately stat rather than an existence flag written at start time: the
// file appears after the row does, and a flag would have to be refreshed by
// something. Nothing here has anything to refresh it with.
func resumes(r store.Remembered) string {
	if r.CLI == "" || r.Transcript == "" {
		return r.Cmd
	}
	if _, err := os.Stat(r.Transcript); err != nil {
		return r.Cmd
	}
	return session.Resume(r.Cmd, r.CLI)
}

// restore starts a session again, on the conversation it was having.
//
// Nothing about what runs comes from the browser: the project names a row, and
// the row carries where it ran, what it ran and which conversation it was
// having. That is the same rule the start form follows for the same reason — a
// field naming a process is a shell behind Access — and it is stricter here,
// because there is not even a list to choose from.
//
// **This is not a restart loop and is not on a timer.** It runs when somebody
// presses it. An agent CLI that crashed still wants a person, and this is the
// person (MUS-Q-0083).
func (s *Sessions) restore(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "that did not come from this site", http.StatusForbidden)
		return
	}
	project := r.PathValue("project")
	if s.Store == nil {
		s.startFailed(w, r, "nothing here remembers what was running")
		return
	}
	remembered, err := s.Store.RememberedSessions(r.Context())
	if err != nil {
		s.startFailed(w, r, "what was running could not be read back")
		return
	}
	for _, row := range remembered {
		if row.Project != project {
			continue
		}
		if _, err := s.Adapter.Start(r.Context(), project, row.Dir, resumes(row)); err != nil {
			s.startFailed(w, r, err.Error())
			return
		}
		http.Redirect(w, r, "/sessions/"+project, http.StatusSeeOther)
		return
	}
	s.startFailed(w, r, "Mustur is not holding a session by that name")
}

func (s *Sessions) commands() []string {
	if len(s.Commands) > 0 {
		return s.Commands
	}
	return DefaultCommands
}

// start launches a session the owner asked for.
//
// The three things it needs come from three different places on purpose
// (MUS-D-0146). The name is typed, because it is a label and nothing runs it.
// The directory is looked up from the repository record the owner picked, so
// no path is ever submitted. The command is matched against an allowlist, so
// what a browser sends chooses between known things rather than naming a
// process — `mustur session start --cmd` runs whatever it is given, and a form
// with that field would be a shell behind Access.
func (s *Sessions) start(w http.ResponseWriter, r *http.Request) {
	// The guard already refuses a POST from a reader; this refuses one from
	// another site. No other POST in this package checks, which is its own
	// finding and not a reason for the path that starts processes to skip it.
	if !sameOrigin(r) {
		http.Error(w, "that did not come from this site", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.startFailed(w, r, "that did not arrive as a form")
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	repo := strings.TrimSpace(r.PostFormValue("repo"))
	cmd := strings.TrimSpace(r.PostFormValue("cmd"))

	if _, err := session.NameFor(name); err != nil {
		s.startFailed(w, r, err.Error())
		return
	}
	var dir string
	for _, c := range s.startables(r.Context()) {
		if c.ID == repo {
			dir = c.Dir
			break
		}
	}
	if dir == "" {
		s.startFailed(w, r, "pick somewhere for it to run")
		return
	}
	var allowed bool
	for _, c := range s.commands() {
		if c == cmd {
			allowed = true
			break
		}
	}
	if !allowed {
		// Named rather than echoed: the value came from a browser and a
		// refusal is not a reason to print it back onto the page.
		s.startFailed(w, r, "that is not a command this may start")
		return
	}

	if _, err := s.Adapter.Start(r.Context(), name, dir, cmd); err != nil {
		s.startFailed(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/sessions/"+name, http.StatusSeeOther)
}

func (s *Sessions) startFailed(w http.ResponseWriter, r *http.Request, why string) {
	http.Redirect(w, r, "/sessions?error="+urlQuery(why), http.StatusSeeOther)
}

func urlQuery(s string) string { return strings.ReplaceAll(queryEscape(s), "+", "%20") }

func queryEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			strings.ContainsRune("-_.~ ", r) {
			b.WriteRune(r)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", r)
	}
	return strings.ReplaceAll(b.String(), " ", "+")
}

// stop ends a session the owner asked to end.
//
// MUS-Q-0080 put it on the session's own page behind the tick that Withdraw
// uses, and the tick is gone again (MUS-F-0103). It was copied from a surface
// where blocking script leaves a page that still works; this is not one. The
// session view is a live terminal and there is nothing here without script --
// no output, no composer, no keys -- so a tick guarding the no-script case was
// guarding a case that does not exist, and it sat stacked above the button
// looking like it did.
//
// What is in front of it now is the confirmation the owner asked for, which
// names the session, plus the two that were always the real ones: the origin
// check here, and the guard's owner-only rule on any POST.
//
// It can only end a session Mustur started: Stop prefixes the name and asks
// Alive first, and Alive reads the marker Mustur set at start. A project named
// in a URL cannot reach a tmux session belonging to somebody's own terminal.
func (s *Sessions) stop(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "that did not come from this site", http.StatusForbidden)
		return
	}
	project := r.PathValue("project")
	if err := r.ParseForm(); err != nil {
		s.startFailed(w, r, "that did not arrive as a form")
		return
	}
	if err := s.Adapter.Stop(r.Context(), project); err != nil {
		http.Redirect(w, r, "/sessions/"+project+"?error="+urlQuery(err.Error()), http.StatusSeeOther)
		return
	}
	// Nowhere to go back to: the session this was reached from is gone, so the
	// page that starts one is where this lands.
	http.Redirect(w, r, "/sessions?new=1", http.StatusSeeOther)
}
