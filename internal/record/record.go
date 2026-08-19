// Package record is the shape of everything Mustur stores.
//
// One shape carries every StrucGu module role and every routing row. The
// alternative — a table per kind — buys typed columns and costs a second
// insert path, a second export path and a second audit path for each kind
// added, which is the cost this project is least able to pay while its
// requirements are still discoverable only by use.
package record

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/DevOfPie/Mustur/internal/ident"
)

// Field is one named value on a record. Fields are ordered, not a map: the
// order is the author's and the export renders it, so two exports of the same
// record produce the same bytes without needing an ordering rule invented at
// render time.
type Field struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Record is one addressable thing Mustur holds.
type Record struct {
	ID    string  `json:"id"`
	Kind  string  `json:"kind"`
	Title string  `json:"title"`
	At    string  `json:"at"`             // YYYY-MM-DD. When the record's content was true, not when it was inserted.
	Body  string  `json:"body,omitempty"` // Markdown. May be empty for a record whose fields carry everything.
	Refs  []Field `json:"refs,omitempty"` // Named citations: {"supersedes", "MUS-D-0005"}.
	Data  []Field `json:"data,omitempty"` // Kind-specific values, rendered in this order.
}

// Ident parses the record's identifier.
func (r Record) Ident() (ident.ID, error) { return ident.Parse(r.ID) }

// Validate checks everything about a record that does not need the store: that
// the identifier parses, that its role letter agrees with its kind, that a
// date is present in the one form the export renders, and that no field is
// nameless.
func (r Record) Validate() error {
	id, err := ident.Parse(r.ID)
	if err != nil {
		return err
	}
	if id.Role.Name() != r.Kind {
		return fmt.Errorf("record %s: role letter %s means kind %q, record says %q", r.ID, id.Role, id.Role.Name(), r.Kind)
	}
	if strings.TrimSpace(r.Title) == "" {
		return fmt.Errorf("record %s: no title", r.ID)
	}
	if !isDate(r.At) {
		return fmt.Errorf("record %s: date %q is not YYYY-MM-DD", r.ID, r.At)
	}
	for _, f := range append(append([]Field{}, r.Refs...), r.Data...) {
		if strings.TrimSpace(f.Key) == "" {
			return fmt.Errorf("record %s: a field has no name", r.ID)
		}
	}
	return nil
}

func isDate(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i, c := range s {
		if i == 4 || i == 7 {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Cites returns every identifier this record mentions, in its refs or in its
// prose, sorted and deduplicated.
func (r Record) Cites() []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		for _, id := range ident.Cited(s) {
			if id != r.ID && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	add(r.Title)
	add(r.Body)
	for _, f := range append(append([]Field{}, r.Refs...), r.Data...) {
		add(f.Value)
	}
	sort.Strings(out)
	return out
}

// Get returns the first data field with the given name.
func (r Record) Get(key string) (string, bool) {
	for _, f := range r.Data {
		if f.Key == key {
			return f.Value, true
		}
	}
	return "", false
}

// Sort orders records the way every Mustur listing orders them.
func Sort(rs []Record) {
	sort.SliceStable(rs, func(i, j int) bool {
		a, errA := ident.Parse(rs[i].ID)
		b, errB := ident.Parse(rs[j].ID)
		if errA != nil || errB != nil { // Unparseable identifiers sort last, by string, deterministically.
			return rs[i].ID < rs[j].ID
		}
		return ident.Less(a, b)
	})
}

// MarshalPayload renders the record as the JSON the event log stores.
func (r Record) MarshalPayload() ([]byte, error) { return json.Marshal(r) }

// UnmarshalPayload reads a record back out of the event log.
func UnmarshalPayload(b []byte) (Record, error) {
	var r Record
	err := json.Unmarshal(b, &r)
	return r, err
}
