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
//   - a module outside lib/, other than one named schema.rego, defines deny
//     or warn, so that a misspelled rule name does not drop its rule silently;
//   - a module in lib/ is imported by another loaded module and defines
//     neither deny nor warn.
func Load(root, dir, entrypoint string) (*Set, error) {
	abs := filepath.Join(root, filepath.FromSlash(dir))
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real // WalkDir does not descend into a symbolic link
	}
	var mods []module
	err := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
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
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("policy: %s does not exist", dir)
		}
		return nil, fmt.Errorf("policy: %w", err)
	}
	if len(mods) == 0 {
		return nil, fmt.Errorf("policy: no .rego files in %s", dir)
	}
	slices.SortFunc(mods, func(a, b module) int { return strings.Compare(a.rel, b.rel) })
	if err := checkPackages(mods, dir, entrypoint); err != nil {
		return nil, err
	}
	s := &Set{entrypoint: entrypoint}
	for _, m := range mods {
		s.modules = append(s.modules, m.m)
	}
	return s, nil
}

func checkPackages(mods []module, dir, entrypoint string) error {
	want, err := ast.ParseRef("data." + entrypoint)
	if err != nil {
		return fmt.Errorf("entrypoint %q: %w", entrypoint, err)
	}
	var errs []error
	for _, m := range mods {
		name := path.Join(dir, m.rel)
		pkg := m.m.Package.Path
		defines := definesDiagnostics(m.m)
		if strings.HasPrefix(m.rel, "lib/") {
			if len(defines) > 0 {
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
		if len(defines) == 0 && path.Base(m.rel) != "schema.rego" {
			errs = append(errs, fmt.Errorf("%s: defines neither deny nor warn", name))
		}
	}
	return errors.Join(errs...)
}

// packageName returns a package path as a package clause names it.
func packageName(r ast.Ref) string {
	return strings.TrimPrefix(r.String(), "data.")
}

// definesDiagnostics returns which of deny and warn the module defines.
func definesDiagnostics(m *ast.Module) []string {
	var found []string
	for _, r := range m.Rules {
		ref := r.Head.Ref()
		if len(ref) == 0 {
			continue
		}
		if v, ok := ref[0].Value.(ast.Var); ok && (v == "deny" || v == "warn") && !slices.Contains(found, string(v)) {
			found = append(found, string(v))
		}
	}
	slices.Sort(found)
	return found
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
