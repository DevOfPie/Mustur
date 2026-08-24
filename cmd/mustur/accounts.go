package main

// `mustur account` — the command line half of who is asking.
//
// It exists for two moments a browser cannot serve.
//
// **The first owner.** A fresh store knows nobody, so there is nobody to send
// an invitation. The command line is where the first one comes from, and it is
// the right place: anyone who can run this can already read the database, so it
// grants nothing that was not already theirs.
//
// **The last owner locked out.** The owner asked for a passkey that can be
// replaced when a device is lost (MUS-D-0104). The ordinary answer is a second
// passkey or an invitation reissued by somebody else with an owner role. When
// neither exists — one owner, one device, gone — this is the way back, and it
// needs a shell on the machine rather than a password anybody could phish.

import (
	"flag"
	"fmt"
	"strings"

	"github.com/DevOfPie/Mustur/internal/account"
)

func cmdAccount(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("account needs a verb: invite, list, grant")
	}
	verb, rest := args[0], args[1:]

	switch verb {
	case "invite":
		fs := flag.NewFlagSet("account invite", flag.ContinueOnError)
		db := dbFlag(fs)
		email := fs.String("email", "", "who the invitation is for")
		project := fs.String("project", "MUS", "the project the role applies to")
		role := fs.String("role", string(account.Reader), "owner or reader")
		base := fs.String("base", "", "the site the link points at, e.g. https://mustur.devofpie.com")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if strings.TrimSpace(*email) == "" {
			return fmt.Errorf("account invite needs --email")
		}
		s, storeCtx, err := openStore(*db)
		if err != nil {
			return err
		}
		defer s.Close()
		accounts := account.New(s.DB())
		secret, err := accounts.Invite(storeCtx, *email, *project, account.Role(*role), defaultActor())
		if err != nil {
			return err
		}
		// Printed once and never recoverable. An invitation that goes missing
		// is reissued; there is nothing to look up, because only its hash was
		// written.
		fmt.Printf("invited %s as %s on %s\n", strings.ToLower(*email), *role, *project)
		if *base != "" {
			fmt.Printf("  %s/invite/%s\n", strings.TrimRight(*base, "/"), secret)
		} else {
			fmt.Printf("  /invite/%s\n", secret)
			fmt.Println("  (pass --base to print a whole URL)")
		}
		fmt.Printf("  good for %s, and once\n", account.InviteLife)
		return nil

	case "list":
		fs := flag.NewFlagSet("account list", flag.ContinueOnError)
		db := dbFlag(fs)
		if err := fs.Parse(rest); err != nil {
			return err
		}
		s, storeCtx, err := openStore(*db)
		if err != nil {
			return err
		}
		defer s.Close()
		accounts := account.New(s.DB())
		people, err := accounts.Accounts(storeCtx)
		if err != nil {
			return err
		}
		pending, err := accounts.Pending(storeCtx)
		if err != nil {
			return err
		}
		if len(people) == 0 && len(pending) == 0 {
			fmt.Println("nobody yet")
			fmt.Println("  mustur account invite --email you@example.com --role owner")
			return nil
		}
		for _, p := range people {
			grants, err := accounts.Grants(storeCtx, p.ID)
			if err != nil {
				return err
			}
			creds, err := accounts.Credentials(storeCtx, p.ID)
			if err != nil {
				return err
			}
			var roles []string
			for _, g := range grants {
				roles = append(roles, g.Project+":"+string(g.Role))
			}
			state := ""
			if p.Disabled {
				state = "  disabled"
			}
			fmt.Printf("%-32s %-24s %d passkey(s)%s\n",
				p.Email, strings.Join(roles, " "), len(creds), state)
		}
		// An invitation issued and not yet accepted, so nobody reads "nobody
		// yet" a minute after sending one and sends another.
		for _, inv := range pending {
			fmt.Printf("%-32s %-24s invited, not yet accepted\n",
				inv.Email, inv.Project+":"+string(inv.Role))
		}
		return nil

	case "grant":
		fs := flag.NewFlagSet("account grant", flag.ContinueOnError)
		db := dbFlag(fs)
		email := fs.String("email", "", "which account")
		project := fs.String("project", "MUS", "the project the role applies to")
		role := fs.String("role", "", "owner or reader")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if strings.TrimSpace(*email) == "" || strings.TrimSpace(*role) == "" {
			return fmt.Errorf("account grant needs --email and --role")
		}
		s, storeCtx, err := openStore(*db)
		if err != nil {
			return err
		}
		defer s.Close()
		accounts := account.New(s.DB())
		people, err := accounts.Accounts(storeCtx)
		if err != nil {
			return err
		}
		want := strings.ToLower(strings.TrimSpace(*email))
		for _, p := range people {
			if p.Email == want {
				if err := accounts.Grant(storeCtx, p.ID, *project, account.Role(*role), defaultActor()); err != nil {
					return err
				}
				fmt.Printf("%s is now %s on %s\n", p.Email, *role, *project)
				return nil
			}
		}
		return fmt.Errorf("no account for %s; invite them first", want)
	}
	return fmt.Errorf("account has no verb %q: invite, list, grant", verb)
}
