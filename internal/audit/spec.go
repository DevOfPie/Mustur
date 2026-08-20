// Package audit implements StrucGu's check vocabulary against a repository
// that has adopted it.
//
// StrucGu ships no runner, deliberately, which makes its SPEC.md "the only
// thing that makes two independent implementations agree". This is one
// implementation. Where it had to choose, the choice is a comment naming what
// the specification left open — those are the places two correct-looking
// checkers diverge, and the specification says an ambiguity is a missing
// fixture rather than a defect in either.
//
// It writes nothing to the repository it audits.
package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

// Module is one module's manifest — the machine-readable half of a module.
// module.md is normative where the two disagree, and a disagreement is a
// defect to report rather than a choice to make, so nothing here silently
// prefers this file.
type Module struct {
	Schema   string     `yaml:"schema"`
	ID       string     `yaml:"id"`
	Version  string     `yaml:"version"`
	Title    string     `yaml:"title"`
	Base     bool       `yaml:"base"`
	Roles    []Role     `yaml:"roles"`
	Checks   []Check    `yaml:"checks"`
	Judgment []Judgment `yaml:"judgment"`
}

// Role is a logical artifact name. Never a path: the adoption record is the
// only place a path appears.
type Role struct {
	ID                string            `yaml:"id"`
	Cardinality       string            `yaml:"cardinality"`
	CardinalityByForm map[string]string `yaml:"cardinality_by_form"`
	Exclude           []string          `yaml:"exclude"`
	Of                string            `yaml:"of"`
}

// Check is one observable test of an obligation.
type Check struct {
	ID                    string   `yaml:"id"`
	Obligation            string   `yaml:"obligation"`
	Nature                string   `yaml:"nature"`
	Kind                  string   `yaml:"kind"`
	In                    RoleList `yaml:"in"`
	To                    string   `yaml:"to"` // role_referenced only: the role the target must link to.
	Patterns              []string `yaml:"patterns"`
	RequiresEffectiveFrom bool     `yaml:"requires_effective_from"`
	Finding               string   `yaml:"finding"`
	PairedWith            string   `yaml:"paired_with"`
}

// Judgment is everything a check cannot decide. One line is emitted per entry
// on every run, whether or not anyone is there to judge it, so that a purely
// mechanical run can never look complete.
type Judgment struct {
	ID          string   `yaml:"id"`
	Obligation  string   `yaml:"obligation"`
	Question    string   `yaml:"question"`
	Read        RoleList `yaml:"read"`
	Evidence    string   `yaml:"evidence"`
	DoNotReport string   `yaml:"do_not_report"`
}

// RoleList is one role or several. The manifest writes `in: decision_log` and
// `in: [decision_log, decision_index]` interchangeably, so both decode here.
type RoleList []string

// UnmarshalYAML accepts a scalar or a sequence.
func (r *RoleList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var one string
		if err := node.Decode(&one); err != nil {
			return err
		}
		*r = RoleList{one}
		return nil
	case yaml.SequenceNode:
		var many []string
		if err := node.Decode(&many); err != nil {
			return err
		}
		*r = many
		return nil
	default:
		return fmt.Errorf("a role reference must be a name or a list of names")
	}
}

// Role returns a module's role by id.
func (m Module) Role(id string) (Role, bool) {
	for _, r := range m.Roles {
		if r.ID == id {
			return r, true
		}
	}
	return Role{}, false
}

// Catalog is a checkout of the modules a repository adopts.
type Catalog struct {
	Root    string
	Modules map[string]Module
}

// LoadCatalog reads every module manifest under root/modules.
func LoadCatalog(root string) (*Catalog, error) {
	dir := filepath.Join(root, "modules")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read module catalog at %s: %w", dir, err)
	}
	cat := &Catalog{Root: root, Modules: map[string]Module{}}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name(), "module.yaml")
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		var m Module
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if m.ID != e.Name() {
			return nil, fmt.Errorf("%s declares id %q in a directory named %q", path, m.ID, e.Name())
		}
		cat.Modules[m.ID] = m
	}
	if len(cat.Modules) == 0 {
		return nil, fmt.Errorf("no modules under %s", dir)
	}
	return cat, nil
}

// Adoption is the consuming repository's hand-edited claim about itself.
type Adoption struct {
	Schema     string                   `yaml:"schema"`
	Modules    map[string]AdoptedModule `yaml:"modules"`
	Exclude    []string                 `yaml:"exclude"`
	Deviations []Deviation              `yaml:"deviations"`
}

// AdoptedModule is one module's adoption: a pinned version and a role map.
type AdoptedModule struct {
	Version       string             `yaml:"version"`
	Adopted       string             `yaml:"adopted"`
	Form          string             `yaml:"form"`
	EffectiveFrom string             `yaml:"effective_from"`
	Roles         map[string]*string `yaml:"roles"` // nil value means `~`: deliberately unmapped.
}

// Deviation is a finding the owner accepted. It is echoed every run rather
// than suppressed, and it expires.
type Deviation struct {
	Check    string `yaml:"check"`
	Title    string `yaml:"title"`
	Accepted string `yaml:"accepted"`
	By       string `yaml:"by"`
	Reason   string `yaml:"reason"`
	Scope    string `yaml:"scope"`
	ReviewBy string `yaml:"review_by"`
	Upstream string `yaml:"upstream"`
}

var exactVersion = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// LoadAdoption reads and validates the adoption record at root/strucgu.yaml.
// Everything it rejects is a run that cannot start rather than a finding: a
// record a checker cannot parse says nothing about the repository's conformance
// and reporting it as a failure would claim otherwise.
func LoadAdoption(root string) (*Adoption, error) {
	path := filepath.Join(root, "strucgu.yaml")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var a Adoption
	if err := yaml.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if a.Schema != "strucgu/adoption@1" {
		return nil, fmt.Errorf("%s declares schema %q, not strucgu/adoption@1", path, a.Schema)
	}
	if len(a.Modules) == 0 {
		return nil, fmt.Errorf("%s adopts no modules", path)
	}
	for id, m := range a.Modules {
		if !exactVersion.MatchString(m.Version) {
			return nil, fmt.Errorf("%s pins %s at %q, which is not an exact x.y.z version", path, id, m.Version)
		}
		if m.Adopted == "" {
			return nil, fmt.Errorf("%s adopts %s with no date", path, id)
		}
	}
	return &a, nil
}

// AdoptedIDs returns the adopted module ids in a stable order, so two runs of
// the same audit produce the same report.
func (a *Adoption) AdoptedIDs() []string {
	ids := make([]string, 0, len(a.Modules))
	for id := range a.Modules {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
