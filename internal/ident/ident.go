// Package ident holds Mustur's record identifier scheme.
//
// An identifier is PROJECT-ROLE-SERIAL: `MUS-D-0001`. The project prefix is
// three upper-case letters, so a second project onboarded later cannot collide
// with this one; the role letter says which StrucGu module role the record
// plays; the serial is zero-padded to four digits and unique within its
// project and role.
//
// **A prefix can also be reserved: an underscore and two upper-case letters,
// `_IB-F-0001`.** That form is for lists Mustur keeps for itself rather than for
// a project — the intake box, where a jot lands when nothing else will take it,
// is `_IB` (MUS-D-0192). A real project's prefix is three letters, so no
// project onboarded later can ever take a reserved one, and a record in
// Mustur's own list can never be mistaken for, or collide with, a record about
// a project. The
// underscore sorts after every upper-case letter, so a reserved list comes
// after every project in any listing ordered by Less.
//
// **The prefix says which project a record belongs to, not which store holds
// it.** The routing record names the prefix and intake uses it (MUS-D-0093).
// Before that, everything filed here was called MUS, and a jot in the intake
// box — then called the idea inbox — was indistinguishable at a glance from a
// record about Mustur itself. `MUS-F-0025` is the last one filed that way and
// keeps its identifier, because the permanence rule below is what makes
// citations safe. The box's jots were then filed under IDW, which is Idea
// Warehouse's prefix; the six filed that way were renamed to `_IB-F-0001`
// through `_IB-F-0006` in place, the one exception the permanence rule has
// (MUS-D-0192).
//
// Identifiers are permanent. The store is insert-only and records cite each
// other by identifier, so a scheme that allows renaming is a scheme that
// allows a citation to rot. MUS-D-0192 is the one exception, and the reserved
// form above is what keeps it from being needed again.
package ident

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Role is the single letter in the middle of an identifier.
type Role string

const (
	Decision      Role = "D"
	Question      Role = "Q" // Open, and the owner's. A decision is what some of them become.
	Finding       Role = "F"
	Investigation Role = "I"
	WorkUnit      Role = "W"
	Milestone     Role = "M"
	Repository    Role = "R"
	Machine       Role = "H" // H for host: M is already the milestone.
	Project       Role = "P"
	Phase         Role = "S" // S for stage: P is already the project (MUS-D-0173).
)

// Roles lists every role letter in the order records are presented. A phase
// comes first because it holds milestones.
var Roles = []Role{Phase, Milestone, WorkUnit, Question, Decision, Finding, Investigation, Repository, Machine, Project}

var roleNames = map[Role]string{
	Phase:         "phase",
	Decision:      "decision",
	Question:      "question",
	Finding:       "finding",
	Investigation: "investigation",
	WorkUnit:      "work-unit",
	Milestone:     "milestone",
	Repository:    "repository",
	Machine:       "machine",
	Project:       "project",
}

// Name is the record kind a role letter stands for.
func (r Role) Name() string { return roleNames[r] }

// KindNames lists every record kind, in the order records are presented.
//
// Anything enumerating the kinds calls this rather than writing the list out.
// A hardcoded copy is how the MCP tool call came to omit `question` the day a
// role letter was added: the index it returned was still described as "every
// record" and silently was not.
func KindNames() []string {
	out := make([]string, 0, len(Roles))
	for _, role := range Roles {
		out = append(out, role.Name())
	}
	return out
}

// RoleFor maps a kind name back to its letter. The second result is false for
// a name no role carries.
func RoleFor(name string) (Role, bool) {
	for role, n := range roleNames {
		if n == name {
			return role, true
		}
	}
	return "", false
}

// ProjectPattern is the regular-expression fragment a project prefix matches:
// three upper-case letters, or the reserved form of an underscore and two.
// Anything elsewhere that finds identifiers in text builds on this rather than
// spelling the shape out again, so the next change to the scheme is one line.
const ProjectPattern = `(?:[A-Z]{3}|_[A-Z]{2})`

var pattern = regexp.MustCompile(`^(` + ProjectPattern + `)-([A-Z])-([0-9]{4})$`)

// ID is a parsed identifier.
type ID struct {
	Project string
	Role    Role
	Serial  int
}

// String renders the identifier in its canonical form.
func (i ID) String() string {
	return fmt.Sprintf("%s-%s-%04d", i.Project, i.Role, i.Serial)
}

// Parse reads an identifier. It rejects anything the canonical form would not
// have produced, including a serial past four digits: widening the field later
// would resort every identifier written before it.
func Parse(s string) (ID, error) {
	m := pattern.FindStringSubmatch(s)
	if m == nil {
		return ID{}, fmt.Errorf("identifier %q is not PROJECT-ROLE-SERIAL, e.g. MUS-D-0001", s)
	}
	role := Role(m[2])
	if _, known := roleNames[role]; !known {
		return ID{}, fmt.Errorf("identifier %q carries unknown role letter %q", s, role)
	}
	serial, err := strconv.Atoi(m[3])
	if err != nil { // Unreachable while the pattern demands four digits.
		return ID{}, fmt.Errorf("identifier %q has an unreadable serial: %w", s, err)
	}
	if serial == 0 {
		return ID{}, fmt.Errorf("identifier %q has serial 0; serials start at 1", s)
	}
	return ID{Project: m[1], Role: role, Serial: serial}, nil
}

// ValidProject reports whether s is a well-formed project prefix on its own:
// three upper-case letters, or a reserved underscore and two. A routing record naming its own prefix is
// checked with this before anything is filed under it, so a typo in the
// registry produces a jot under the store's prefix rather than an identifier
// the scheme cannot parse.
func ValidProject(s string) bool {
	return projectPattern.MatchString(s)
}

var projectPattern = regexp.MustCompile(`^` + ProjectPattern + `$`)

// Valid reports whether s parses.
func Valid(s string) bool {
	_, err := Parse(s)
	return err == nil
}

// Less orders identifiers by project, then by the order roles are presented in,
// then by serial. Every listing in Mustur sorts with it, so two runs over the
// same records produce the same bytes.
func Less(a, b ID) bool {
	if a.Project != b.Project {
		return a.Project < b.Project
	}
	if a.Role != b.Role {
		return roleOrder(a.Role) < roleOrder(b.Role)
	}
	return a.Serial < b.Serial
}

func roleOrder(r Role) int {
	for i, role := range Roles {
		if role == r {
			return i
		}
	}
	return len(Roles)
}

// Cited pulls every identifier mentioned in a body of text. Used to check that
// a record's citations point at records that exist.
func Cited(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, field := range strings.FieldsFunc(text, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' && r != '_'
	}) {
		if id := citedIn(field); id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// citedIn finds the identifier a run of identifier characters spells, if any.
// The underscore has to be one of those characters or a reserved identifier
// could not be read, and that glues a three-letter one to whatever precedes it:
// `FOO_MUS-D-0001` is one run. So the run is tried whole, then from its last
// underscore (a reserved identifier), then after it (a project one) — which
// finds in such a run exactly what was found before the reserved form existed.
func citedIn(field string) string {
	if Valid(field) {
		return field
	}
	i := strings.LastIndex(field, "_")
	if i < 0 {
		return ""
	}
	for _, c := range []string{field[i:], field[i+1:]} {
		if Valid(c) {
			return c
		}
	}
	return ""
}
