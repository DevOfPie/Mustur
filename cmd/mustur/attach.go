package main

// `mustur image` — the bridge between a picture and the record that describes
// it.
//
// The owner's decision on attachments is that the description travels and the
// pixels do not: the bytes stay in the store and never reach the exported tree,
// which is committed to a public repository. That only works if something can
// read the picture and write down what it showed, and this is how an agent gets
// at one. `mustur image read <id> --out shot.png`, look at it, then
// `mustur amend <record> --data "Evidence=..."` with what it actually said.
//
// Deliberately not a viewer and not an editor. It lists what is attached and
// hands one file to whoever asked, which is all a bridge needs to be.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cmdImage(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("image needs a verb: list, read, forget")
	}
	verb, rest := args[0], args[1:]

	switch verb {
	case "list":
		fs := flag.NewFlagSet("image list", flag.ContinueOnError)
		db := dbFlag(fs)
		if err := fs.Parse(rest); err != nil {
			return err
		}
		s, ctx, err := openStore(*db)
		if err != nil {
			return err
		}
		defer s.Close()

		// Every record, because an attachment is looked for by whoever is
		// working through the queue rather than by somebody who already knows
		// which record carries one.
		all, err := s.List(ctx, "")
		if err != nil {
			return err
		}
		found := 0
		for _, r := range all {
			shots, err := s.Attachments(ctx, r.ID)
			if err != nil {
				return err
			}
			for _, a := range shots {
				found++
				fmt.Printf("%-14s %-12s %-10s %6d KB  %s\n",
					a.ID, a.RecordID, strings.TrimPrefix(a.MediaType, "image/"),
					(a.Size+1023)/1024, a.Created.Format("2006-01-02 15:04"))
			}
		}
		if found == 0 {
			fmt.Println("no images attached to anything")
		}
		return nil

	case "read":
		fs := flag.NewFlagSet("image read", flag.ContinueOnError)
		db := dbFlag(fs)
		out := fs.String("out", "", "write the image here; default is the identifier plus its extension")
		// Either order, like `get`. Go's flag package stops at the first
		// non-flag argument, so `image read <id> --db P` would otherwise read
		// the identifier and silently ignore the flag — which is exactly the
		// trap this helper exists for, and which caught the builder first time.
		id, err := parseWithPositional(fs, rest, "image read needs one image id; mustur image list shows them")
		if err != nil {
			return err
		}
		s, ctx, err := openStore(*db)
		if err != nil {
			return err
		}
		defer s.Close()

		a, data, err := s.Image(ctx, id)
		if err != nil {
			return err
		}
		path := *out
		if path == "" {
			path = a.ID + "." + strings.TrimPrefix(a.MediaType, "image/")
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return err
		}
		abs, _ := filepath.Abs(path)
		fmt.Printf("%s  %s  %d KB  attached to %s\n", abs, a.MediaType, (a.Size+1023)/1024, a.RecordID)
		fmt.Println("  look at it, then write what it showed into the record:")
		fmt.Printf("  mustur amend %s --title \"…\" --data \"Evidence=…\"\n", a.RecordID)
		// Said once, where somebody is about to write the description: the
		// record is public and the picture is not, so the description carries
		// what matters and nothing that did not need to travel.
		fmt.Println("  the record is exported and public; the image is neither")
		return nil

	case "forget":
		fs := flag.NewFlagSet("image forget", flag.ContinueOnError)
		db := dbFlag(fs)
		id, err := parseWithPositional(fs, rest, "image forget needs one image id; mustur image list shows them")
		if err != nil {
			return err
		}
		s, ctx, err := openStore(*db)
		if err != nil {
			return err
		}
		defer s.Close()

		a, _, err := s.Image(ctx, id)
		if err != nil {
			return err
		}
		if err := s.Forget(ctx, id); err != nil {
			return err
		}
		// The record is untouched, which is the point: the description an agent
		// wrote from the picture is the half that was meant to last.
		fmt.Printf("forgot %s; %s keeps what was written about it\n", a.ID, a.RecordID)
		return nil
	}
	return fmt.Errorf("image has no verb %q: list, read, forget", verb)
}
