package audit

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// target is a role resolved against a tree: the files a check bound to it
// reads, and why there are none when there are none.
type target struct {
	role    string
	mapped  bool     // The adoption record names a path (`~` is mapped=false).
	rawPath string   // As written in the adoption record.
	isDir   bool     // From the role's cardinality, resolved through the declared form.
	files   []string // Absolute paths, sorted. Markdown only for a dir role.
	exists  bool     // The path itself is present.
	tracked bool     // Git knows it, where the root is a repository.
	note    string   // Why files is empty, for a skip that has to say what it could not tell.
}

// tree is one audited root, plus what git can say about it.
type tree struct {
	root      string
	isRepo    bool
	trackedBy map[string]bool // Repository-relative slash paths git has.
}

func openTree(root string) (*tree, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	t := &tree{root: abs, trackedBy: map[string]bool{}}

	// Read-only git, and only at the audited root. The specification is
	// explicit that a checker must not walk up to find a repository: an
	// adoption record vendored inside a larger one would otherwise be audited
	// against history its owner does not control.
	cmd := exec.Command("git", "-C", abs, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return t, nil // Not a repository. Trackedness has no truth value here.
	}
	if strings.TrimSpace(string(out)) != abs {
		return t, nil // A repository, but rooted elsewhere. Same answer.
	}
	t.isRepo = true
	listed, err := exec.Command("git", "-C", abs, "ls-files").Output()
	if err != nil {
		return nil, fmt.Errorf("list tracked files in %s: %w", abs, err)
	}
	for _, line := range strings.Split(string(listed), "\n") {
		if line != "" {
			t.trackedBy[line] = true
		}
	}
	return t, nil
}

// cardinality resolves a role's shape, following cardinality_by_form through
// the form the adoption record declares.
func cardinality(role Role, adopted AdoptedModule) (string, error) {
	if role.Cardinality != "by_form" {
		return role.Cardinality, nil
	}
	if adopted.Form == "" {
		// Not a finding: a record a checker cannot resolve says nothing about
		// the repository, and the specification calls this a run that cannot
		// start.
		return "", fmt.Errorf("role %s varies by form and the adoption record declares none", role.ID)
	}
	c, ok := role.CardinalityByForm[adopted.Form]
	if !ok {
		return "", fmt.Errorf("role %s declares no shape for form %q", role.ID, adopted.Form)
	}
	return c, nil
}

// resolve maps one role to the files a check bound to it reads.
func (t *tree) resolve(m Module, adopted AdoptedModule, a *Adoption, roleID string) (target, error) {
	tg := target{role: roleID}
	role, ok := m.Role(roleID)
	if !ok {
		return tg, fmt.Errorf("module %s binds a check to role %q, which it does not declare", m.ID, roleID)
	}
	raw, declared := adopted.Roles[roleID]
	if !declared || raw == nil || strings.TrimSpace(*raw) == "" {
		tg.note = "unmapped in the adoption record"
		return tg, nil
	}
	tg.mapped = true
	tg.rawPath = *raw

	shape, err := cardinality(role, adopted)
	if err != nil {
		return tg, err
	}
	tg.isDir = shape == "dir"

	abs := filepath.Join(t.root, filepath.FromSlash(strings.TrimSuffix(tg.rawPath, "/")))
	info, err := os.Stat(abs)
	if os.IsNotExist(err) {
		tg.note = fmt.Sprintf("%s does not exist", tg.rawPath)
		return tg, nil
	}
	if err != nil {
		return tg, err
	}
	tg.exists = true

	if !tg.isDir {
		if info.IsDir() {
			tg.note = fmt.Sprintf("%s is a directory and the role is a file", tg.rawPath)
			return tg, nil
		}
		tg.tracked = t.tracks(abs)
		if t.isRepo && !tg.tracked {
			// Exists for its author and for nobody who clones.
			tg.note = fmt.Sprintf("%s is not tracked", tg.rawPath)
			return tg, nil
		}
		tg.files = []string{abs}
		return tg, nil
	}

	if !info.IsDir() {
		tg.note = fmt.Sprintf("%s is a file and the role is a directory", tg.rawPath)
		return tg, nil
	}
	files, err := t.markdownUnder(abs, role.Exclude, a.Exclude)
	if err != nil {
		return tg, err
	}
	tg.files = files
	tg.tracked = len(files) > 0
	if len(files) == 0 {
		tg.note = fmt.Sprintf("%s holds no markdown that survives exclusions", tg.rawPath)
	}
	return tg, nil
}

func (t *tree) tracks(abs string) bool {
	if !t.isRepo {
		return true // No truth value; do not let it decide anything.
	}
	rel, err := filepath.Rel(t.root, abs)
	if err != nil {
		return false
	}
	return t.trackedBy[filepath.ToSlash(rel)]
}

// markdownUnder lists the markdown files a dir role covers: everything under
// it and below, minus the role's own excludes matched against each basename at
// any depth, and minus the adoption record's, matched against the path relative
// to the repository root.
func (t *tree) markdownUnder(dir string, roleExclude, adoptionExclude []string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(p), ".md") {
			return nil
		}
		for _, pattern := range roleExclude {
			if ok, _ := path.Match(pattern, d.Name()); ok {
				return nil
			}
		}
		rel, relErr := filepath.Rel(t.root, p)
		if relErr != nil {
			return relErr
		}
		slash := filepath.ToSlash(rel)
		for _, pattern := range adoptionExclude {
			if matchAdoptionGlob(pattern, slash) {
				return nil
			}
		}
		if !t.tracks(p) {
			return nil
		}
		out = append(out, p)
		return nil
	})
	sort.Strings(out)
	return out, err
}

// matchAdoptionGlob matches an adoption-record exclusion against a
// repository-relative path.
//
// The specification does not fix the glob dialect, and this is one of the
// places two correct-looking checkers disagree. The reading here: `**` crosses
// directory separators and a lone `*` does not — the shell's rule, and the one
// under which `records/*.md` means the six files in `records/` rather than
// everything beneath it. Mustur's own adoption record was written against the
// other reading, silently excluded a whole role's records, and now lists paths
// literally instead. Where the answer matters, list paths.
func matchAdoptionGlob(pattern, target string) bool {
	if strings.Contains(pattern, "**") {
		prefix, suffix, _ := strings.Cut(pattern, "**")
		if !strings.HasPrefix(target, prefix) {
			return false
		}
		suffix = strings.TrimPrefix(suffix, "/")
		if suffix == "" {
			return true
		}
		rest := strings.TrimPrefix(target, prefix)
		for {
			if ok, _ := path.Match(suffix, rest); ok {
				return true
			}
			_, after, found := strings.Cut(rest, "/")
			if !found {
				return false
			}
			rest = after
		}
	}
	ok, _ := path.Match(pattern, target)
	return ok
}
