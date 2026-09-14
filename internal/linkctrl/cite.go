package linkctrl

import (
	"regexp"
	"strconv"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
)

var citation = regexp.MustCompile(`\b(?:D([0-9]+)|F([0-9]+)|LNK-M-([0-9]{4}))\b`)

// Cite turns the citations in every imported record into refs: a D or F
// number, or a milestone identifier Rewrite has already placed, becomes a
// "cites" ref when the import holds a record under it. Text is left as it is.
// A W number is LinkCtrl's workflow change, which has no kind here, and a
// number the import holds nothing for stays text. It runs after Rewrite.
func Cite(sources []Source) int {
	have := map[string]bool{}
	for _, src := range sources {
		for _, r := range src.Records {
			have[r.ID] = true
		}
	}
	added := 0
	for si := range sources {
		for ri := range sources[si].Records {
			r := &sources[si].Records[ri]
			seen := map[string]bool{r.ID: true}
			for _, f := range r.Refs {
				seen[f.Value] = true
			}
			collect := func(s string) string {
				for _, m := range citation.FindAllStringSubmatch(s, -1) {
					id := ""
					switch {
					case m[1] != "":
						id = serialID(ident.Decision, m[1])
					case m[2] != "":
						id = serialID(ident.Finding, m[2])
					default:
						id = serialID(ident.Milestone, m[3])
					}
					if id == "" || !have[id] || seen[id] {
						continue
					}
					seen[id] = true
					r.Refs = append(r.Refs, record.Field{Key: "cites", Value: id})
					added++
				}
				return s
			}
			outsideCode(r.Title, collect)
			outsideCode(r.Body, collect)
			for _, f := range r.Data {
				if f.Key != "LinkCtrl" {
					outsideCode(f.Value, collect)
				}
			}
		}
	}
	return added
}

func serialID(role ident.Role, digits string) string {
	n, err := strconv.Atoi(digits)
	if err != nil || n < 1 || n > 9999 {
		return ""
	}
	return ident.ID{Project: Prefix, Role: role, Serial: n}.String()
}
