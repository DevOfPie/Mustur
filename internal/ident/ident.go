// Package ident holds Mustur's record identifier scheme.
//
// An identifier is PROJECT-ROLE-SERIAL: `MUS-D-0001`. The project prefix is
// three upper-case letters, so a second project onboarded later cannot collide
// with this one; the role letter says which StrucGu module role the record
// plays; the serial is zero-padded to four digits and unique within its
// project and role.
//
// Identifiers are permanent. The store is insert-only and records cite each
// other by identifier, so a scheme that allows renaming is a scheme that
// allows a citation to rot.
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
)

// Roles lists every role letter in the order records are presented.
var Roles = []Role{Milestone, WorkUnit, Question, Decision, Finding, Investigation, Repository, Machine, Project}

var roleNames = map[Role]string{
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

var pattern = regexp.MustCompile(`^([A-Z]{3})-([A-Z])-([0-9]{4})$`)

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
		return !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-'
	}) {
		if Valid(field) && !seen[field] {
			seen[field] = true
			out = append(out, field)
		}
	}
	return out
}
