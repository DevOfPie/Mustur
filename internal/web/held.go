package web

// What both surfaces need to know about held jots (MUS-D-0189): whose they are,
// when they were sent, and which of them the person looking may approve.

import (
	"context"
	"net/http"
	"time"
	_ "time/tzdata" // Pacific must render on a machine with no zoneinfo.

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/intake"
	"github.com/DevOfPie/Mustur/internal/store"
)

// Roles answers a role on any project, which the guard's own check does not:
// it asks about this install's project only, and a held jot may be routed to
// another.
type Roles interface {
	RoleFor(ctx context.Context, accountID, project string) (account.Role, bool)
}

// pacificZone is where the owner reads times. Every time given to them is
// Pacific and says PDT or PST, never bare.
var pacificZone = func() *time.Location {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		// Unreachable with time/tzdata linked in; UTC still names itself.
		return time.UTC
	}
	return loc
}()

// pacific renders a moment as the owner reads one: "Sep 15 10:42 PDT".
func pacific(t time.Time) string {
	return t.In(pacificZone).Format("Jan 2 15:04 MST")
}

// heldProject is the project a held jot would be filed under if approved to
// `to`: the destination's prefix, or this install's own when it names none.
// A destination that no longer resolves counts as this install's, which is
// where File would refuse it and the owner of this install is who can fix it.
func heldProject(ctx context.Context, st *store.Store, text, to, home string) (intake.Destination, string) {
	d, err := intake.Resolve(ctx, st, text, to)
	if err != nil || d.Prefix == "" {
		return d, home
	}
	return d, d.Prefix
}

// mayApprove says whether this request's viewer owns `project`.
//
// With no signed-in viewer the server is running without --accounts and every
// surface is the owner's, which is CanWrite's rule. With one, the grant on that
// project decides; with no way to ask about other projects, only this install's
// owner role counts, and only for this install's project.
func mayApprove(ctx context.Context, r *http.Request, roles Roles, project, home string) bool {
	viewer, ok := Viewer(r)
	if !ok {
		return CanWrite(r)
	}
	if roles == nil {
		return project == home && CanWrite(r)
	}
	role, granted := roles.RoleFor(ctx, viewer.ID, project)
	return granted && role == account.Owner
}

// heldWaiting is approvable without the destinations, for a count.
func heldWaiting(ctx context.Context, r *http.Request, st *store.Store, roles Roles, home string) []store.Held {
	held, _ := approvable(ctx, r, st, roles, home)
	return held
}

// approvable lists the held jots this request's viewer may approve, each with
// the destination it would be filed to as things stand.
//
// The viewer is read from r and the store is queried under ctx, which are the
// same thing for a page and not for the session view's socket: that request
// was upgraded, and its ticker counts under the connection's own context.
func approvable(ctx context.Context, r *http.Request, st *store.Store, roles Roles, home string) ([]store.Held, []intake.Destination) {
	if st == nil || IsReader(r) {
		return nil, nil
	}
	all, err := st.HeldJots(ctx, "")
	if err != nil || len(all) == 0 {
		return nil, nil
	}
	var out []store.Held
	var dests []intake.Destination
	for _, h := range all {
		d, project := heldProject(ctx, st, h.Text, h.To, home)
		if mayApprove(ctx, r, roles, project, home) {
			out = append(out, h)
			dests = append(dests, d)
		}
	}
	return out, dests
}
