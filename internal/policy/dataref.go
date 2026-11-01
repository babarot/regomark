package policy

import (
	"errors"
	"fmt"
	"slices"

	"github.com/open-policy-agent/opa/v1/ast"
)

// checkDataRefs reports every reference into data that no loaded module
// defines. Rego takes such a reference as undefined, not as an error, so a
// misspelled library constant (lib.h.alowed for lib.h.allowed) would make
// the rule using it never fire. regomark loads no data besides the modules,
// so a reference none of them defines cannot be anything else.
//
// The modules are compiled first, which rewrites import aliases and names of
// the same package into full data paths. If they do not compile, the check
// is left to the evaluation, which reports the compile errors.
func checkDataRefs(mods []module) error {
	compiled := make(map[string]*ast.Module, len(mods))
	for _, m := range mods {
		compiled[m.rel] = m.m.Copy()
	}
	c := ast.NewCompiler()
	c.Compile(compiled)
	if c.Failed() {
		return nil
	}

	var errs []error
	seen := map[string]bool{}
	for _, m := range mods {
		cm := c.Modules[m.rel]
		if cm == nil {
			continue
		}
		for _, r := range cm.Rules {
			ast.WalkRefs(r, func(ref ast.Ref) bool {
				if !ref.HasPrefix(ast.DefaultRootRef) || defined(c, ref.ConstantPrefix()) {
					return false
				}
				loc := r.Location
				if ref[0].Location != nil {
					loc = ref[0].Location
				}
				msg := fmt.Sprintf("%s:%d: %s is not defined by any loaded module", loc.File, loc.Row, ref)
				if !seen[msg] {
					seen[msg] = true
					errs = append(errs, errors.New(msg))
				}
				return false
			})
		}
	}
	return errors.Join(errs...)
}

// defined reports whether some rule or package of the compiled modules is
// at, under or above the ground reference.
func defined(c *ast.Compiler, ref ast.Ref) bool {
	// The constant prefix of a data reference keeps only ground terms; cut
	// it at the first one that is not a string, which no package path holds
	if i := slices.IndexFunc(ref[1:], func(t *ast.Term) bool {
		_, ok := t.Value.(ast.String)
		return !ok
	}); i >= 0 {
		ref = ref[:i+1]
	}
	if len(ref) <= 1 || len(c.GetRules(ref)) > 0 {
		return true
	}
	for _, m := range c.Modules {
		if m.Package.Path.HasPrefix(ref) {
			return true
		}
	}
	return false
}
