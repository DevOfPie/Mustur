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
//
// **Two kinds of caller.** A person carries a session cookie earned with a
// passkey; an agent carries a bearer token and reaches exactly one path. That
// second kind is milestone 5c, and it exists because 5b built this guard and
// then could not switch it on: `/mcp` is on this mux, an MCP call is a POST, and
// every agent was answered 403. The owner chose a token over exempting the path
// (MUS-Q-0051).

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

// toolCall is the one path an agent token opens, and the only one.
//
// A token is a long-lived secret living in a systemd unit or a process's
// environment — a weaker place than a device's secure element — so it is scoped
// to the thing an agent actually needs and nothing else. It cannot read the
// records surface, cannot open a session, cannot sign in.
//
// That scope argument is the builder's, not the owner's: MUS-Q-0051 chose a
// token over exempting this path and said nothing about scope. An earlier
// version of this comment cited the question for it, which is somebody else's
// reasoning wearing the owner's name.
func toolCall(path string) bool {
	// Exactly the path main.go registers, and not the subtree. A prefix would
	// hand every handler later mounted under /mcp/ both the token bypass and
	// the missing write check above, without anybody editing this file.
	return path == "/mcp"
}

// agent resolves a bearer token, if the request carries one.
//
// Absent or unusable both return false and the request falls through to the
// browser rules, so a signed-in owner can still exercise the tool call from a
// browser and an agent with a revoked token gets the same 403 as one with none.
func (g *Guard) agent(r *http.Request) (account.Token, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return account.Token{}, false
	}
	// Case-insensitive on the scheme, because clients differ and this is not
	// the place to be strict about a thing that carries no meaning.
	if len(h) < 7 || !strings.EqualFold(h[:7], "bearer ") {
		return account.Token{}, false
	}
	t, err := g.Auth.Accounts.ByToken(r.Context(), h[7:])
	if err != nil {
		return account.Token{}, false
	}
	return t, true
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
		// An agent, before a person. The mandated tool call has to work while
		// this gate is on, which is what milestone 5c exists for: 5b built a
		// guard that answered 403 to every agent and could therefore never be
		// turned on.
		if toolCall(r.URL.Path) {
			if t, ok := g.agent(r); ok {
				if t.Project != g.Project {
					http.Error(w, "no access to this project", http.StatusForbidden)
					return
				}
				// No write check. This surface serves one tool and it reads,
				// which internal/mcpsrv's TestToolIsReachableOverHTTP asserts and
				// says why — a second tool fails there and names this line. An
				// earlier version of this comment cited a test that did not
				// exist, which is a safety argument discharged against nothing.
				_ = g.Auth.Accounts.UsedToken(r.Context(), t.ID)
				next.ServeHTTP(w, r)
				return
			}
			// No usable token: fall through, so an owner can still exercise it
			// from a signed-in browser and a revoked token is refused exactly
			// as an absent one is.
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
