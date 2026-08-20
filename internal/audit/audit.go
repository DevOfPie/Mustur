package audit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// State is one of the five a checker must distinguish and must never fold into
// another. A run reporting only ok and finding is hiding something.
type State string

const (
	OK            State = "ok"
	Finding       State = "finding"
	Skip          State = "skip"
	Waived        State = "waived"
	NeedsJudgment State = "judgment"
)

// States is the order counts are reported in.
var States = []State{OK, Finding, Skip, Waived, NeedsJudgment}

// Result is one check's or one judgment entry's outcome.
type Result struct {
	Module   string
	ID       string // Check id, or judgment entry id.
	State    State
	Detail   string   // Why: the finding, the reason it skipped, the deviation's reason.
	Paths    []string // Repository-relative, sorted. What was read, or what failed.
	Question string   // Judgment entries only.
	Evidence string   // Judgment entries only.
}

// Report is one run.
type Report struct {
	Root    string
	Catalog string
	Results []Result
	Notices []string // Version drift. Not findings: they are about the catalog, not the tree.
	Counts  map[State]int
}

// Run audits root against the modules its adoption record names, resolved from
// cat. It writes nothing to root.
//
// An error is a run that could not start — an unparseable adoption record, a
// module the catalog does not hold, a version pinned loosely. Findings are
// output, not failure, which is why they come back in the report rather than
// as an error.
func Run(root string, cat *Catalog, now time.Time) (*Report, error) {
	adoption, err := LoadAdoption(root)
	if err != nil {
		return nil, err
	}
	t, err := openTree(root)
	if err != nil {
		return nil, err
	}
	rep := &Report{Root: t.root, Catalog: cat.Root, Counts: map[State]int{}}

	// Roles resolve per module, but a check may name a role belonging to a
	// prerequisite — findings-queue asks whether the queue links to the triage
	// document, and triage_doc is triage-rule's role. Resolving every adopted
	// module's roles up front is what lets that cross the boundary, and a role
	// whose module is not adopted stays absent from this map, which is what
	// makes such a check skip rather than fail.
	byModule := map[string]map[string]target{}
	for _, id := range adoption.AdoptedIDs() {
		adopted := adoption.Modules[id]
		module, ok := cat.Modules[id]
		if !ok {
			return nil, fmt.Errorf("%s adopts module %q, which the catalog at %s does not hold", root, id, cat.Root)
		}
		if notice := driftNotice(id, adopted.Version, module.Version); notice != "" {
			rep.Notices = append(rep.Notices, notice)
		}
		roles := map[string]target{}
		for _, role := range module.Roles {
			tg, err := t.resolve(module, adopted, adoption, role.ID)
			if err != nil {
				return nil, err
			}
			roles[role.ID] = tg
		}
		byModule[id] = roles
	}

	for _, id := range adoption.AdoptedIDs() {
		adopted := adoption.Modules[id]
		module := cat.Modules[id]
		targets := byModule[id]
		for _, check := range module.Checks {
			rep.add(t.evaluate(module, adopted, adoption, targets, lookup(byModule, id), check, now))
		}
		for _, j := range module.Judgment {
			res := judgmentResult(module, targets, j)
			res.Paths = t.relative(res.Paths)
			rep.add(res)
		}
	}
	return rep, nil
}

func (r *Report) add(res Result) {
	r.Results = append(r.Results, res)
	r.Counts[res.State]++
}

// Total is how many results the run produced.
func (r *Report) Total() int { return len(r.Results) }

// judgmentResult emits one line per judgment entry, always. A purely
// mechanical run must never look complete, so the entry is emitted whether or
// not anyone is present and whether or not the roles it names resolve — but a
// person handed the output must not be sent to a document that is not there,
// so an unreadable role is annotated rather than hidden.
func judgmentResult(m Module, targets map[string]target, j Judgment) Result {
	res := Result{Module: m.ID, ID: j.ID, State: NeedsJudgment, Question: j.Question, Evidence: j.Evidence}
	var missing []string
	for _, role := range j.Read {
		tg := targets[role]
		if len(tg.files) == 0 {
			missing = append(missing, fmt.Sprintf("%s (%s)", role, reasonFor(tg)))
			continue
		}
		res.Paths = append(res.Paths, tg.files...)
	}
	if len(missing) > 0 {
		res.Detail = "nothing to read for " + strings.Join(missing, ", ")
	}
	return res
}

func reasonFor(tg target) string {
	if tg.note != "" {
		return tg.note
	}
	return "no readable target"
}

// evaluate runs one check and returns its single state. A check reports one
// state for a role even when the role covers many files: finding if any file
// fails, ok only if every file passes.
// driftNotice is the whole of StrucGu's push surface: a checker that reads a
// module newer than the pin prints one line pointing at that module's
// changelog. There is no bot, no pull request and no notification, and
// propagation is pull-only.
//
// It is emphatically not a refusal. A checker evaluates the module as it reads
// it — nothing in a module lets it evaluate a version other than the one on
// disk — so the pin records what the adopter agreed to and reports drift
// rather than constraining what runs. An earlier draft of this package refused
// to run on a mismatch, which would have made every fixture in the catalog
// unauditable and, worse, hidden the drift it was trying to flag.
func driftNotice(id, pinned, read string) string {
	if pinned == read {
		return ""
	}
	notice := fmt.Sprintf("%s: the catalog holds %s and this repository pins %s — see modules/%s/CHANGELOG.md",
		id, read, pinned, id)
	if majorOf(read) > majorOf(pinned) {
		// A new check never ships in a minor version, so only a major bump can
		// have added one since the adopter agreed.
		notice += ". That is a major ahead, so checks may have been added since the adoption"
	}
	return notice
}

func majorOf(version string) int {
	major, _, _ := strings.Cut(version, ".")
	n := 0
	for _, r := range major {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// roleFinder resolves a role that may belong to another adopted module.
type roleFinder func(role string) (target, bool)

// lookup searches the check's own module first, then every other adopted one.
func lookup(byModule map[string]map[string]target, own string) roleFinder {
	return func(role string) (target, bool) {
		if tg, ok := byModule[own][role]; ok {
			return tg, true
		}
		ids := make([]string, 0, len(byModule))
		for id := range byModule {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if tg, ok := byModule[id][role]; ok {
				return tg, true
			}
		}
		return target{}, false
	}
}

func (t *tree) evaluate(m Module, adopted AdoptedModule, a *Adoption, targets map[string]target, find roleFinder, c Check, now time.Time) Result {
	res := Result{Module: m.ID, ID: c.ID}

	var read []target
	for _, role := range c.In {
		if tg, ok := targets[role]; ok && len(tg.files) > 0 {
			read = append(read, tg)
		}
	}
	if len(read) == 0 && c.Kind == "path_exists" {
		// path_exists is the check that observes absence, so absence is its
		// finding rather than its excuse. The rule it must not break is the
		// next one down: when it fails, every *other* check bound only to that
		// role skips, because reporting five findings for one missing file
		// tells a reader the repository is five times more broken than it is.
		//
		// An unmapped role is still a skip even here. Absence of a declaration
		// is not evidence of anything, and the specification says so in as many
		// words.
		mapped := false
		var why []string
		for _, role := range c.In {
			tg := targets[role]
			if tg.mapped {
				mapped = true
			}
			why = append(why, fmt.Sprintf("%s: %s", role, reasonFor(tg)))
		}
		if mapped {
			res.State, res.Detail = Finding, strings.Join(why, "; ")
			if dev, ok := a.deviation(c.ID, nil, now); ok {
				res.State = Waived
				res.Detail = fmt.Sprintf("%s — accepted %s by %s, review by %s: %s",
					res.Detail, dev.Accepted, dev.By, dev.ReviewBy, oneLine(dev.Reason))
			}
			return res
		}
		res.State, res.Detail = Skip, strings.Join(why, "; ")
		return res
	}

	if len(read) == 0 {
		// A check bound to more than one role skips only when none resolved:
		// having read two of three, reporting skip would claim it could not
		// tell when it could.
		res.State = Skip
		var why []string
		for _, role := range c.In {
			why = append(why, fmt.Sprintf("%s: %s", role, reasonFor(targets[role])))
		}
		res.Detail = strings.Join(why, "; ")
		return res
	}

	if c.Kind == "history_deletions" && adopted.EffectiveFrom == "" {
		// Running it unbounded produces four-figure findings on a mature
		// repository, and an audit that reports four thousand findings on day
		// one is deleted the same day.
		res.State = Skip
		res.Detail = "the adoption record declares no effective_from, and history is not read without one"
		return res
	}

	state, detail, paths := t.observe(m, adopted, find, c, read)
	res.State, res.Detail, res.Paths = state, detail, t.relative(paths)

	if res.State == Finding {
		if dev, ok := a.deviation(c.ID, res.Paths, now); ok {
			res.State = Waived
			res.Detail = fmt.Sprintf("%s — accepted %s by %s, review by %s: %s",
				res.Detail, dev.Accepted, dev.By, dev.ReviewBy, oneLine(dev.Reason))
		}
	}
	return res
}

// relative renders paths against the audited root, deduplicated. Two roles
// mapped to the same file — which is ordinary under a single-log decision
// record — would otherwise list it twice and read as two files.
func (t *tree) relative(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		rel := p
		if r, err := filepath.Rel(t.root, p); err == nil {
			rel = filepath.ToSlash(r)
		}
		if !seen[rel] {
			seen[rel] = true
			out = append(out, rel)
		}
	}
	sort.Strings(out)
	return out
}

// deviation finds an accepted deviation covering a check and the paths it
// failed on. A deviation past its review date does not apply: detection
// reasserts itself without anyone having to enforce anything.
func (a *Adoption) deviation(checkID string, paths []string, now time.Time) (Deviation, bool) {
	for _, d := range a.Deviations {
		if d.Check != checkID {
			continue
		}
		if d.ReviewBy != "" {
			expiry, err := time.Parse("2006-01-02", d.ReviewBy)
			if err == nil && now.After(expiry) {
				continue
			}
		}
		if d.Scope != "" && len(paths) > 0 {
			covered := true
			for _, p := range paths {
				if !matchAdoptionGlob(d.Scope, p) {
					covered = false
					break
				}
			}
			if !covered {
				continue
			}
		}
		return d, true
	}
	return Deviation{}, false
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// observe is the check vocabulary. Closed: adding a kind is a change to
// StrucGu's specification, not to this file.
func (t *tree) observe(m Module, adopted AdoptedModule, find roleFinder, c Check, read []target) (State, string, []string) {
	switch c.Kind {
	case "path_exists":
		// Reaching here means the role resolved to at least one readable file,
		// which is what the check observes.
		return OK, "", filesOf(read)

	case "heading_present":
		return t.everyPatternMatches(c, read, func(text string) string {
			return strings.Join(headings(text), "\n")
		})

	case "pattern_present":
		// Deliberately not stripCode. The specification lists "a pattern
		// matching inside a code block, a quotation, or an example" as a known
		// false positive of this kind, which makes it a match rather than a
		// bug — and the fence rule it does state is about links alone. A
		// checker that skips fences here reports a finding against a triage
		// document whose boundary is written as a block.
		return t.everyPatternMatches(c, read, func(text string) string { return text })

	case "pattern_absent":
		var failed []string
		for _, tg := range read {
			for _, f := range tg.files {
				text, err := readFile(f)
				if err != nil {
					return Skip, err.Error(), nil
				}
				body := text // Same reason as pattern_present: code is not exempt here.
				for _, p := range c.Patterns {
					re, err := compile(p)
					if err != nil {
						return Skip, err.Error(), nil
					}
					if re.MatchString(body) {
						failed = append(failed, fmt.Sprintf("%s matches /%s/", filepath.Base(f), p))
					}
				}
			}
		}
		if len(failed) > 0 {
			return Finding, strings.Join(failed, "; "), filesOf(read)
		}
		return OK, "", filesOf(read)

	case "links_resolve":
		return t.linksResolve(read)

	case "role_referenced":
		return t.roleReferenced(m, find, c, read)

	case "history_deletions":
		return t.historyDeletions(adopted, read)
	}
	// An unknown kind is a run that cannot answer, not a passing check.
	return Skip, fmt.Sprintf("check kind %q is not in the vocabulary this checker implements", c.Kind), nil
}

func filesOf(read []target) []string {
	var out []string
	for _, tg := range read {
		out = append(out, tg.files...)
	}
	return out
}

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(b), nil
}

// compile builds a case-insensitive extended regular expression, which is what
// the specification says patterns are.
func compile(pattern string) (*regexp.Regexp, error) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, fmt.Errorf("pattern /%s/ does not compile: %w", pattern, err)
	}
	return re, nil
}

// everyPatternMatches is the shape both present-checks share: every listed
// pattern must match somewhere in the role, and the role passes only if all of
// them do.
func (t *tree) everyPatternMatches(c Check, read []target, prepare func(string) string) (State, string, []string) {
	var missing []string
	for _, tg := range read {
		for _, f := range tg.files {
			text, err := readFile(f)
			if err != nil {
				return Skip, err.Error(), nil
			}
			body := prepare(text)
			for _, p := range c.Patterns {
				re, err := compile(p)
				if err != nil {
					return Skip, err.Error(), nil
				}
				if !re.MatchString(body) {
					missing = append(missing, fmt.Sprintf("%s has nothing matching /%s/", filepath.Base(f), p))
				}
			}
		}
	}
	if len(missing) > 0 {
		return Finding, strings.Join(missing, "; "), filesOf(read)
	}
	return OK, "", filesOf(read)
}

func (t *tree) linksResolve(read []target) (State, string, []string) {
	var broken []string
	for _, tg := range read {
		for _, f := range tg.files {
			text, err := readFile(f)
			if err != nil {
				return Skip, err.Error(), nil
			}
			for _, l := range links(text) {
				targetPath := f
				if l.path != "" {
					targetPath = filepath.Join(filepath.Dir(f), filepath.FromSlash(l.path))
					info, statErr := os.Stat(targetPath)
					if statErr != nil {
						broken = append(broken, fmt.Sprintf("%s -> %s (no such file)", filepath.Base(f), l.raw))
						continue
					}
					if info.IsDir() {
						continue // A link to a directory resolves; there is no heading inside one.
					}
				}
				if l.anchor == "" {
					continue
				}
				body, readErr := readFile(targetPath)
				if readErr != nil {
					broken = append(broken, fmt.Sprintf("%s -> %s (unreadable)", filepath.Base(f), l.raw))
					continue
				}
				if !slugs(body)[l.anchor] {
					broken = append(broken, fmt.Sprintf("%s -> %s (no such heading)", filepath.Base(f), l.raw))
				}
			}
		}
	}
	if len(broken) > 0 {
		return Finding, strings.Join(broken, "; "), filesOf(read)
	}
	return OK, "", filesOf(read)
}

// roleReferenced tests that the target links to another role's path. Not that
// any anchor on it resolves — a link to the right document with a rotted
// anchor passes here and is reported by links_resolve, which is where it
// belongs.
func (t *tree) roleReferenced(m Module, find roleFinder, c Check, read []target) (State, string, []string) {
	if c.To == "" {
		return Skip, "role_referenced names no `to` role", nil
	}
	wanted, declared := find(c.To)
	if !declared {
		// Its module is not adopted. Skip, not a finding: this is what makes
		// the kind safe in a repository that adopts one module and not another.
		return Skip, fmt.Sprintf("role %s belongs to a module this repository does not adopt", c.To), nil
	}
	if !wanted.mapped || len(wanted.files) == 0 {
		// Safe in a repository that adopts one module and not another.
		return Skip, fmt.Sprintf("%s: %s", wanted.role, reasonFor(wanted)), nil
	}
	want := map[string]bool{}
	for _, f := range wanted.files {
		want[filepath.Clean(f)] = true
	}
	for _, tg := range read {
		if tg.role == wanted.role {
			continue
		}
		for _, f := range tg.files {
			text, err := readFile(f)
			if err != nil {
				return Skip, err.Error(), nil
			}
			for _, l := range links(text) {
				if l.path == "" {
					continue
				}
				resolved := filepath.Clean(filepath.Join(filepath.Dir(f), filepath.FromSlash(l.path)))
				if want[resolved] {
					return OK, "", filesOf(read)
				}
			}
		}
	}
	return Finding, fmt.Sprintf("no link resolves to role %s at %s", wanted.role, wanted.rawPath), filesOf(read)
}

// historyDeletions reports commits, not lines, and never reads history earlier
// than effective_from. The bound is exclusive: the named commit is the last one
// out of scope.
func (t *tree) historyDeletions(adopted AdoptedModule, read []target) (State, string, []string) {
	if !t.isRepo {
		return Skip, "the audited root is not a git repository", nil
	}
	paths := filesOf(read)
	args := []string{"-C", t.root, "log", "--format=%h %s", "--diff-filter=M", "--numstat"}
	if commit, ok := t.resolves(adopted.EffectiveFrom); ok {
		args = append(args, commit+"..HEAD")
	} else {
		args = append(args, "--since="+adopted.EffectiveFrom)
	}
	args = append(args, "--")
	args = append(args, paths...)

	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return Skip, fmt.Sprintf("git log over %s failed", adopted.EffectiveFrom), nil
	}
	var commits []string
	current := ""
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 3 && isCount(fields[0]) && isCount(fields[1]) {
			if fields[1] != "0" && current != "" {
				commits = append(commits, current)
				current = ""
			}
			continue
		}
		current = line
	}
	if len(commits) > 0 {
		return Finding, fmt.Sprintf("%d commit(s) removed lines: %s", len(commits), strings.Join(commits, "; ")), paths
	}
	return OK, "", paths
}

func isCount(s string) bool {
	if s == "-" {
		return true // A binary file's numstat.
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// resolves reports whether git knows a commit by that name — the specification
// says effective_from is a commit if git resolves it and a date otherwise.
func (t *tree) resolves(ref string) (string, bool) {
	if ref == "" {
		return "", false
	}
	out, err := exec.Command("git", "-C", t.root, "rev-parse", "--verify", "--quiet", ref+"^{commit}").Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}
