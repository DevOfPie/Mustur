package web

// The guard: what a role actually stops somebody doing.
//
// Roles are worth nothing until something refuses. This wraps the whole mux
// rather than each surface, because a rule applied per handler is a rule a new
// handler forgets — and this repository has now watched a promise go unkept in
// three templates at once.
//
// **Two questions, in order.** Is anybody signed in, and may they write? A
// reader gets every reading surface and nothing else; a request with no account
// gets the sign-in page. Nobody has a role on a project they were never granted
// one on, which is not the same as being a reader there.
//
// **Off by default.** Enforcement is a flag, for the same reason `--sessions`
// is: turning it on before the owner holds a passkey locks the owner out of
// their own running service. It is turned on deliberately, once somebody can
// get in.

import (
	"net/http"
	"strings"

	"github.com/DevOfPie/Mustur/internal/account"
)

// Guard refuses what a role may not do.
type Guard struct {
	Auth *Auth
	// Project is which project's roles this install checks. One today; the
	// field exists so the day there are two, the thing to change is a value
	// rather than a rule.
	Project string
}

// public is everything reachable without an account.
//
// Deliberately a short list of exact prefixes rather than a pattern. The
// sign-in surface has to be reachable by somebody with no account — that is
// what it is for — and everything else is not.
func public(path string) bool {
	switch {
	case path == "/signin",
		path == "/signout",
		path == "/healthz",
		path == "/assets/auth.js":
		return true
	case strings.HasPrefix(path, "/signin/"),
		strings.HasPrefix(path, "/invite/"):
		return true
	}
	return false
}

// selfService is the one subtree this guard does not decide.
//
// Managing your own passkeys is a write by method and is not a write in the
// sense this guard means: a reader who cannot add a second passkey is a reader
// one lost device away from being locked out of an account they are entitled
// to. The handlers under /account authorise themselves — every one of them
// begins by asking who it is for, and the owner-only ones say so — which is
// why the exemption is here, named, in one place, rather than spread across
// them as a habit anybody could forget.
//
// It is still behind the guard's *first* question: a stranger reaches none of
// it, and neither does an account with no role on this project.
func selfService(path string) bool {
	return path == "/account" || strings.HasPrefix(path, "/account/")
}

// writes reports whether a request would change something, or reach a surface
// that types into a running agent.
//
// Method is the general rule and the two agent surfaces are named, because
// reading a live session is not a read: the page holds a socket that carries
// keystrokes back, so a reader who could open it would be a writer.
func writes(r *http.Request) bool {
	if selfService(r.URL.Path) {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return true
	}
	return strings.HasPrefix(r.URL.Path, "/sessions") || strings.HasPrefix(r.URL.Path, "/compose")
}

// Wrap puts the guard in front of everything.
func (g *Guard) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if public(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		acct, ok := g.Auth.Whoever(r.Context(), r)
		if !ok {
			// A browser being pointed at the sign-in page; anything else told
			// plainly. A redirect answering a POST would turn a refused write
			// into a GET of the sign-in page and look like success.
			if r.Method == http.MethodGet {
				http.Redirect(w, r, "/signin", http.StatusSeeOther)
				return
			}
			http.Error(w, "not signed in", http.StatusForbidden)
			return
		}
		role, granted := g.Auth.Accounts.RoleFor(r.Context(), acct.ID, g.Project)
		if !granted {
			// No role is not a lesser role. An account with nothing on this
			// project cannot read it either.
			http.Error(w, "no access to this project", http.StatusForbidden)
			return
		}
		if writes(r) && !role.CanWrite() {
			http.Error(w, "this account can read but not write", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Reader is a convenience for tests and callers that want the role without the
// guard's refusal.
func (g *Guard) Reader(r *http.Request) (account.Account, account.Role, bool) {
	acct, ok := g.Auth.Whoever(r.Context(), r)
	if !ok {
		return account.Account{}, "", false
	}
	role, granted := g.Auth.Accounts.RoleFor(r.Context(), acct.ID, g.Project)
	if !granted {
		return acct, "", false
	}
	return acct, role, true
}
