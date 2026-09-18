package main

// Correcting a jot that "Route it for me" put in the wrong place. What a
// correction is, and why it files a new record rather than renaming the old
// one, is written where it is done: internal/intake/reroute.go. It is there
// rather than here because the Move button on a record is the same act.

import (
	"flag"
	"fmt"
	"time"

	"github.com/DevOfPie/Mustur/internal/intake"
)

func cmdReroute(args []string) error {
	fs := flag.NewFlagSet("reroute", flag.ContinueOnError)
	db := dbFlag(fs)
	project := fs.String("project", "MUS", "identifier prefix for a store holding more than one project")
	actor := fs.String("actor", defaultActor(), "who is making the correction")
	to := fs.String("to", "", "the destination it should have gone to")
	why := fs.String("why", "", "one line on why the first routing was wrong")
	id, err := parseWithPositional(fs, args, "reroute needs one identifier")
	if err != nil {
		return err
	}

	s, ctx, err := openStore(*db)
	if err != nil {
		return err
	}
	defer s.Close()

	done, err := intake.Reroute(ctx, s, intake.RerouteRequest{
		Project: *project, ID: id, To: *to, Actor: *actor, Why: *why, Now: time.Now(),
	})
	if err != nil {
		return err
	}
	fmt.Printf("%s now carries it, routed to %s.\n%s stays, superseded and still resolving.\n",
		done.Fresh.ID, done.Dest.Name, done.Old.ID)
	if done.Moved > 0 {
		fmt.Printf("%d picture(s) moved across.\n", done.Moved)
	}
	return nil
}
