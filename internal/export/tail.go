package export

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/DevOfPie/Mustur/internal/record"
)

// TailMarker opens the generated half of a hand-written document. Everything
// above it is prose somebody wrote; everything below it is replaced on every
// export.
//
// The marker carries its own boundary — `from=MUS-D-0121` — rather than this
// package holding one. The hand-written half stopped where it stopped for
// reasons that are in the document, and a number in Go would be a second place
// to keep them in step.
const TailMarker = "<!-- mustur:generated"

// Tail rewrites everything below the marker in path with every decision from
// the marker's `from=` identifier onward.
//
// This exists because decisions.md fell seventeen decisions behind the store
// and nothing noticed (MUS-F-0073). The owner's answer on MUS-Q-0069 was to
// generate the tail rather than to write it twice or to hand the file over to
// a pointer: the file keeps its prose, its index and its append-only rule down
// to the marker, and below the marker nobody writes anything by hand.
//
// It is deliberately not part of Write. Write owns a directory it may prune;
// this edits one file it must not otherwise touch, and the running service
// exports records/ from a unit whose filesystem is read-only everywhere else.
// Keeping them apart is what stops a daemon writing into the checkout's root.
func Tail(path string, records []record.Record) error {
	existing, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	head, from, err := split(existing)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	var out bytes.Buffer
	out.Write(head)
	for _, r := range tailRecords(records, from) {
		out.WriteString("\n")
		out.WriteString(OneUnder(r, 3))
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

// split returns everything up to and including the marker line, and the
// identifier the marker names as the first generated record.
func split(content []byte) ([]byte, string, error) {
	lines := bytes.SplitAfter(content, []byte("\n"))
	for i, ln := range lines {
		if !bytes.Contains(ln, []byte(TailMarker)) {
			continue
		}
		from := fromOf(string(ln))
		if from == "" {
			return nil, "", fmt.Errorf("the %s marker names no `from=` identifier, so nothing can be generated under it", TailMarker)
		}
		return bytes.Join(lines[:i+1], nil), from, nil
	}
	return nil, "", fmt.Errorf("no %s marker, so there is no generated half to write", TailMarker)
}

func fromOf(line string) string {
	i := strings.Index(line, "from=")
	if i < 0 {
		return ""
	}
	rest := line[i+len("from="):]
	return strings.TrimSpace(strings.Fields(strings.TrimSuffix(rest, "-->"))[0])
}

// tailRecords is every decision at or after from, in identifier order.
//
// Compared as strings, which is right because identifiers are fixed-width and
// zero-padded: MUS-D-0099 sorts before MUS-D-0121 as text exactly as it does as
// a number, and a comparison that parsed the serial would have to decide what
// to do with a prefix it does not recognise.
func tailRecords(records []record.Record, from string) []record.Record {
	var out []record.Record
	for _, r := range records {
		if r.Kind == "decision" && r.ID >= from {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
