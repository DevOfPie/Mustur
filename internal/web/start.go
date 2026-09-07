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
// uses, and the tick is gone again (MUS-F-0102). It was copied from a surface
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
