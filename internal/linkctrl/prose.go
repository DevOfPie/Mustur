package linkctrl

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
)

var (
	adrTitle  = regexp.MustCompile(`^# ADR ([0-9]+): (.+)$`)
	adrStatus = regexp.MustCompile(`^Status: ([^,]+)(?:, (20[0-9]{2}-[0-9]{2}-[0-9]{2}))?`)
)

// Investigation reads one file from docs/adr: its number is its serial, and its
// Status line is the one key/value line in LinkCtrl's whole corpus.
func Investigation(name, text, today string) (record.Record, error) {
	lines := strings.Split(text, "\n")
	m := adrTitle.FindStringSubmatch(strings.TrimSpace(lines[0]))
	if m == nil {
		return record.Record{}, fmt.Errorf("%s: first line is not '# ADR <n>: <title>'", name)
	}
	n, _ := strconv.Atoi(m[1])
	rec := record.Record{
		ID:    ident.ID{Project: Prefix, Role: ident.Investigation, Serial: n}.String(),
		Kind:  "investigation",
		Title: m[2],
		At:    today,
	}
	rest := lines[1:]
	for i, l := range rest {
		if s := adrStatus.FindStringSubmatch(l); s != nil {
			rec.Data = append(rec.Data, record.Field{Key: "Status", Value: s[1]})
			if s[2] != "" {
				rec.At = s[2]
			}
			rest = append(rest[:i:i], rest[i+1:]...)
			break
		}
	}
	rec.Body = strings.TrimSpace(delink(strings.Join(rest, "\n")))
	rec.Data = append(rec.Data, record.Field{Key: "LinkCtrl", Value: "docs/adr/" + name})
	return rec, rec.Validate()
}

var answered = regexp.MustCompile(`\*\*Answered (20[0-9]{2}-[0-9]{2}-[0-9]{2})\*\*`)

// Questions reads upcoming-decisions.md: each ### entry under an "## Open"
// heading is one question, numbered in the order the file gives them, since
// LinkCtrl never numbered its questions. The template entry is not one.
func Questions(text, today string) ([]record.Record, error) {
	var out []record.Record
	var title string
	var body []string
	open := false
	flush := func() error {
		if title == "" {
			return nil
		}
		b := strings.TrimSpace(strings.Join(body, "\n"))
		rec := record.Record{
			ID:    ident.ID{Project: Prefix, Role: ident.Question, Serial: len(out) + 1}.String(),
			Kind:  "question",
			Title: delink(title),
			Body:  delink(b),
			At:    firstDate(b),
		}
		status := "open"
		if a := answered.FindStringSubmatch(b); a != nil {
			status, rec.At = "answered", a[1]
			// The answer is the paragraph holding the marker; LinkCtrl wraps it.
			for _, para := range strings.Split(b, "\n\n") {
				if strings.Contains(para, a[0]) {
					rec.Data = append(rec.Data, record.Field{Key: "Answer", Value: delink(strings.Join(strings.Fields(para), " "))})
					break
				}
			}
			// MUS-D-0126: an answer not given here says where it was given.
			rec.Data = append(rec.Data, record.Field{Key: "Relayed", Value: "imported from LinkCtrl's upcoming-decisions.md, where the answer is recorded; not answered in Mustur"})
		}
		if rec.At == "" {
			rec.At = today
			rec.Data = append(rec.Data, record.Field{Key: "Dated", Value: "on import: the entry carries no date"})
		}
		rec.Data = append(rec.Data, record.Field{Key: "Status", Value: status})
		title, body = "", nil
		if err := rec.Validate(); err != nil {
			return err
		}
		out = append(out, rec)
		return nil
	}
	for _, l := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(l, "## "):
			if err := flush(); err != nil {
				return nil, err
			}
			open = strings.HasPrefix(l, "## Open")
		case strings.HasPrefix(l, "### ") && open:
			if err := flush(); err != nil {
				return nil, err
			}
			if t := strings.TrimPrefix(l, "### "); !strings.HasPrefix(t, "<") {
				title = t
			}
		case title != "":
			if strings.TrimSpace(l) == "```" && len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
				continue // The stray fence closer after the template.
			}
			body = append(body, l)
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return out, nil
}
