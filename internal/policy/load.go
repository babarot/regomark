// Package policy loads the rego policies, checks how their packages are laid
// out, and evaluates them against the input.
package policy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"
)

// Set is the loaded policy set.
type Set struct {
	entrypoint string
	modules    []*ast.Module // sorted by file name
}

// module is a parsed policy file with where it sits in the policy directory.
type module struct {
	rel string // relative to the policy directory, with forward slashes
	m   *ast.Module
}

// Load reads every .rego file under dir (a root-relative directory of the
// repository at root), except the _test.rego ones, and checks the package
// rules:
//
//   - a module outside lib/ is in the entrypoint package itself, not a child
//     of it, whose rules no query would collect;
//   - a module outside lib/ defines deny, warn or questions, so that a
//     misspelled rule name does not drop its rule silently, unless it
//     defines nothing but schema_version and rule_ids;
//   - at least one module defines deny or warn;
//   - a module in lib/ is imported by another loaded module and defines
//     none of deny, warn and questions.
func Load(root, dir, entrypoint string) (*Set, error) {
	abs := filepath.Join(root, filepath.FromSlash(dir))
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real // WalkDir does not descend into a symbolic link
	}
	if _, err := os.Stat(abs); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("policy: %s does not exist", dir)
		}
		return nil, fmt.Errorf("policy: %w", err)
	}
	var mods []module
	err := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			rel, _ := filepath.Rel(abs, p)
			name := path.Join(dir, filepath.ToSlash(rel))
			info, err := os.Stat(p)
			if err != nil {
				// Whatever it pointed to, a policy may be missing with it
				return fmt.Errorf("%s is a symbolic link whose target cannot be read: %w", name, unwrapPath(err))
			}
			if info.IsDir() {
				// WalkDir would not descend into it, so its modules would be
				// neither loaded nor checked; refuse it rather than follow it
				return fmt.Errorf("%s is a symbolic link to a directory; put the modules in the policy directory itself", name)
			}
		}
		if d.IsDir() || !strings.HasSuffix(p, ".rego") || strings.HasSuffix(p, "_test.rego") {
			return nil
		}
		rel, err := filepath.Rel(abs, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		// Named by its root-relative path, which OPA's messages then show
		m, err := ast.ParseModuleWithOpts(path.Join(dir, rel), string(src), ast.ParserOptions{RegoVersion: ast.RegoV1})
		if err != nil {
			return err
		}
		mods = append(mods, module{rel: rel, m: m})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("policy: %w", err)
	}
	if len(mods) == 0 {
		return nil, fmt.Errorf("policy: no .rego files in %s", dir)
	}
	slices.SortFunc(mods, func(a, b module) int { return strings.Compare(a.rel, b.rel) })
	if err := checkPackages(mods, dir, entrypoint); err != nil {
		return nil, err
	}
	if err := checkDataRefs(mods); err != nil {
		return nil, err
	}
	s := &Set{entrypoint: entrypoint}
	for _, m := range mods {
		s.modules = append(s.modules, m.m)
	}
	return s, nil
}

// unwrapPath drops the absolute path from a file system error, whose
// message names the file already.
func unwrapPath(err error) error {
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		return pe.Err
	}
	return err
}

func checkPackages(mods []module, dir, entrypoint string) error {
	want, err := ast.ParseRef("data." + entrypoint)
	if err != nil {
		return fmt.Errorf("entrypoint %q: %w", entrypoint, err)
	}
	var errs []error
	diagnostics := 0 // modules of the entrypoint that define deny or warn
	for _, m := range mods {
		name := path.Join(dir, m.rel)
		pkg := m.m.Package.Path
		for _, f := range collectedFunctions(m.m) {
			// A function is left out of the package document, so what it
			// yields would never be collected
			what := "violations"
			if v, _ := f.Head.Ref()[0].Value.(ast.Var); v == questionsRule {
				what = "questions"
			}
			errs = append(errs, fmt.Errorf("%s:%d: %s must be a set of %s, not a function", name, f.Location.Row, f.Head.Ref(), what))
		}
		if strings.HasPrefix(m.rel, "lib/") {
			if defines := definesRules(m.m, collectedRules, true); len(defines) > 0 {
				errs = append(errs, fmt.Errorf("%s: a library module in lib/ must not define %s", name, strings.Join(defines, " or ")))
			}
			if !imported(m, mods) {
				errs = append(errs, fmt.Errorf("%s: no other module imports the library package %s", name, packageName(pkg)))
			}
			continue
		}
		if !pkg.Equal(want) {
			errs = append(errs, fmt.Errorf("%s: package %s must be %s: modules outside lib/ belong to the entrypoint package", name, packageName(pkg), packageName(want)))
			continue
		}
		switch {
		case len(definesRules(m.m, diagnosticRules, false)) > 0:
			diagnostics++
		case len(definesRules(m.m, []string{questionsRule}, false)) > 0:
			// Asks questions whose answers another module's warn reads
		case !declarationsOnly(m.m):
			errs = append(errs, fmt.Errorf("%s: defines none of deny, warn and questions", name))
		}
	}
	if diagnostics == 0 {
		// A policy set without its rules would pass every document
		errs = append(errs, fmt.Errorf("no module in %s defines deny or warn in package %s", dir, entrypoint))
	}
	return errors.Join(errs...)
}

// packageName returns a package path as a package clause names it.
func packageName(r ast.Ref) string {
	return strings.TrimPrefix(r.String(), "data.")
}

// The rules regomark collects from the entrypoint package: the violations,
// and the questions for the judge.
const questionsRule = "questions"

var (
	diagnosticRules = []string{"deny", "warn"}
	collectedRules  = []string{"deny", questionsRule, "warn"}
)

// definesRules returns which of names the module defines rules for, with
// defaults left out when countDefaults is false. A default never yields
// anything, so it does not make a module a policy (beside a misspelled rule
// it would make the module look like one); it still makes a library define
// the rule.
func definesRules(m *ast.Module, names []string, countDefaults bool) []string {
	var found []string
	for _, r := range m.Rules {
		if r.Default && !countDefaults {
			continue
		}
		ref := r.Head.Ref()
		if len(ref) == 0 {
			continue
		}
		if v, ok := ref[0].Value.(ast.Var); ok && slices.Contains(names, string(v)) && !slices.Contains(found, string(v)) {
			found = append(found, string(v))
		}
	}
	slices.Sort(found)
	return found
}

// declarationsOnly reports whether every rule the module defines is
// schema_version or rule_ids: what the policy set declares about itself,
// which is the one thing a module outside lib/ may hold instead of deny or
// warn. The exemption goes by content, not by file name, so that a
// misspelled rule cannot hide in such a module.
func declarationsOnly(m *ast.Module) bool {
	if len(m.Rules) == 0 {
		return false
	}
	for _, r := range m.Rules {
		ref := r.Head.Ref()
		if len(ref) != 1 {
			return false
		}
		if v, ok := ref[0].Value.(ast.Var); !ok || (v != "schema_version" && v != "rule_ids") {
			return false
		}
	}
	return true
}

// collectedFunctions returns the rules that define deny, warn or questions
// as a function.
func collectedFunctions(m *ast.Module) []*ast.Rule {
	var fs []*ast.Rule
	for _, r := range m.Rules {
		ref := r.Head.Ref()
		if len(ref) == 0 || len(r.Head.Args) == 0 {
			continue
		}
		if v, ok := ref[0].Value.(ast.Var); ok && slices.Contains(collectedRules, string(v)) {
			fs = append(fs, r)
		}
	}
	return fs
}

// imported reports whether another module imports lib's package, a package
// above it, or something in it.
func imported(lib module, mods []module) bool {
	pkg := lib.m.Package.Path
	for _, m := range mods {
		if m.rel == lib.rel {
			continue
		}
		for _, imp := range m.m.Imports {
			ref, ok := imp.Path.Value.(ast.Ref)
			if !ok || len(ref) < 2 || !ref.HasPrefix(ast.DefaultRootRef) {
				continue
			}
			if pkg.HasPrefix(ref) || ref.HasPrefix(pkg) {
				return true
			}
		}
	}
	return false
}
