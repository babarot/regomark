package policy

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/topdown"

	"github.com/babarot/regomark/internal/input"
)

// Severities: which rule a violation came from.
const (
	Deny = "deny"
	Warn = "warn"
)

// Violation is one diagnostic a policy reported.
type Violation struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
}

// Result is what the policy set reported, with the rule IDs it declares.
type Result struct {
	RuleIDs    []string
	Violations []Violation
}

// Eval evaluates the policy set against the input. The output of print
// calls in the policies goes to printTo.
//
// The policy set must declare schema_version, equal to the version of the
// input, and rule_ids, a set of the IDs of its rules. Every violation must be
// an object with exactly rule (one of rule_ids), path, line and message.
func (s *Set) Eval(ctx context.Context, in *input.Input, printTo io.Writer) (*Result, error) {
	if err := s.checkSchemaVersion(ctx, in); err != nil {
		return nil, err
	}

	var schema any
	if err := json.Unmarshal(input.Schema(), &schema); err != nil {
		return nil, err
	}
	schemas := ast.NewSchemaSet()
	schemas.Put(ast.SchemaRootRef, schema)
	doc, err := s.eval(ctx, in, "data."+s.entrypoint,
		// Type-checks the policies' references to the input; it does not
		// check the input itself, which input.Validate has done
		rego.Schemas(schemas),
		rego.EnablePrintStatements(true),
		rego.PrintHook(topdown.NewPrintHook(printTo)),
	)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("data.%s is not defined", s.entrypoint)
	}
	pkg, ok := doc.Value.(ast.Object)
	if !ok {
		return nil, fmt.Errorf("data.%s is not a package", s.entrypoint)
	}

	ids, err := ruleIDs(pkg.Get(ast.StringTerm("rule_ids")))
	if err != nil {
		return nil, fmt.Errorf("data.%s.rule_ids: %w", s.entrypoint, err)
	}
	res := &Result{RuleIDs: ids}
	var errs []error
	for _, severity := range []string{Deny, Warn} {
		vs, err := violations(pkg.Get(ast.StringTerm(severity)), severity, ids)
		if err != nil {
			errs = append(errs, fmt.Errorf("data.%s.%s: %w", s.entrypoint, severity, err))
		}
		res.Violations = append(res.Violations, vs...)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	Sort(res.Violations)
	return res, nil
}

// checkSchemaVersion evaluates schema_version alone, before the rest of the
// policies, which may not even type-check against an input of another version.
func (s *Set) checkSchemaVersion(ctx context.Context, in *input.Input) error {
	v, err := s.eval(ctx, in, fmt.Sprintf("data.%s.schema_version", s.entrypoint))
	if err != nil {
		return err
	}
	if v == nil {
		return fmt.Errorf("the policy set does not define schema_version in package %s", s.entrypoint)
	}
	n, ok := v.Value.(ast.Number)
	if !ok {
		return fmt.Errorf("data.%s.schema_version is %v, not an integer", s.entrypoint, v)
	}
	if i, ok := n.Int(); !ok || i != input.SchemaVersion {
		return fmt.Errorf("the policy set is written for input schema version %v, and this regomark makes version %d", n, input.SchemaVersion)
	}
	return nil
}

// eval runs the query and returns its value as a rego term, keeping sets
// apart from arrays, or nil if the query is undefined.
func (s *Set) eval(ctx context.Context, in *input.Input, query string, opts ...func(*rego.Rego)) (*ast.Term, error) {
	opts = append(opts,
		rego.Query(query),
		rego.StrictBuiltinErrors(true),
		rego.GenerateJSON(func(t *ast.Term, _ *rego.EvalContext) (any, error) { return t, nil }),
	)
	for _, m := range s.modules {
		// The compiler rewrites what it gets; each query compiles a copy
		opts = append(opts, rego.ParsedModule(m.Copy()))
	}
	pq, err := rego.New(opts...).PrepareForEval(ctx)
	if err != nil {
		return nil, err
	}
	rs, err := pq.Eval(ctx, rego.EvalInput(in))
	if err != nil {
		return nil, err
	}
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return nil, nil
	}
	t, ok := rs[0].Expressions[0].Value.(*ast.Term)
	if !ok {
		return nil, fmt.Errorf("unexpected result %T", rs[0].Expressions[0].Value)
	}
	return t, nil
}

func ruleIDs(t *ast.Term) ([]string, error) {
	if t == nil {
		return nil, errors.New("not defined; the policy set must declare the IDs of its rules")
	}
	set, ok := t.Value.(ast.Set)
	if !ok {
		return nil, fmt.Errorf("must be a set of strings, not %s", ast.ValueName(t.Value))
	}
	var ids []string
	for _, e := range set.Slice() {
		s, ok := e.Value.(ast.String)
		if !ok || s == "" {
			return nil, fmt.Errorf("must be a set of non-empty strings; it has %v", e)
		}
		ids = append(ids, string(s))
	}
	slices.Sort(ids)
	return ids, nil
}

// violations converts what deny or warn holds. An undefined rule has none.
func violations(t *ast.Term, severity string, ids []string) ([]Violation, error) {
	if t == nil {
		return nil, nil
	}
	set, ok := t.Value.(ast.Set)
	if !ok {
		return nil, fmt.Errorf("must be a set, not %s", ast.ValueName(t.Value))
	}
	var vs []Violation
	var errs []error
	for _, e := range set.Slice() {
		v, err := violation(e, ids)
		if err != nil {
			errs = append(errs, fmt.Errorf("%v: %w", e, err))
			continue
		}
		v.Severity = severity
		vs = append(vs, v)
	}
	return vs, errors.Join(errs...)
}

// violationFields are the fields of a violation, all required.
var violationFields = []string{"rule", "path", "line", "message"}

func violation(t *ast.Term, ids []string) (Violation, error) {
	obj, ok := t.Value.(ast.Object)
	if !ok {
		return Violation{}, fmt.Errorf("a violation must be an object with %s, not %s", strings.Join(violationFields, ", "), ast.ValueName(t.Value))
	}
	var extra []string
	for _, k := range obj.Keys() {
		if s, ok := k.Value.(ast.String); !ok || !slices.Contains(violationFields, string(s)) {
			extra = append(extra, k.String())
		}
	}
	if len(extra) > 0 {
		return Violation{}, fmt.Errorf("unknown field %s; a violation has %s", strings.Join(extra, ", "), strings.Join(violationFields, ", "))
	}
	str := func(key string) (string, error) {
		v := obj.Get(ast.StringTerm(key))
		if v == nil {
			return "", fmt.Errorf("%s is missing", key)
		}
		s, ok := v.Value.(ast.String)
		if !ok || s == "" {
			return "", fmt.Errorf("%s must be a non-empty string", key)
		}
		return string(s), nil
	}
	var v Violation
	var err error
	if v.Rule, err = str("rule"); err != nil {
		return v, err
	}
	if v.Path, err = str("path"); err != nil {
		return v, err
	}
	if v.Message, err = str("message"); err != nil {
		return v, err
	}
	line := obj.Get(ast.StringTerm("line"))
	if line == nil {
		return v, errors.New("line is missing")
	}
	n, ok := line.Value.(ast.Number)
	if !ok {
		return v, errors.New("line must be an integer")
	}
	if v.Line, ok = n.Int(); !ok || v.Line < 1 {
		return v, errors.New("line must be an integer from 1")
	}
	if !slices.Contains(ids, v.Rule) {
		return v, fmt.Errorf("rule %q is not in rule_ids", v.Rule)
	}
	return v, nil
}

// Skip drops the violations of the rules in ids, each of which must be one of
// the rule IDs the policy set declares.
func (r *Result) Skip(ids []string) error {
	var unknown []string
	for _, id := range ids {
		if !slices.Contains(r.RuleIDs, id) {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("skip_rules: unknown rule ID %s; the policy set declares %s", strings.Join(unknown, ", "), strings.Join(r.RuleIDs, ", "))
	}
	r.Violations = slices.DeleteFunc(r.Violations, func(v Violation) bool { return slices.Contains(ids, v.Rule) })
	return nil
}

// Denied reports whether any violation is a deny.
func (r *Result) Denied() bool {
	return slices.ContainsFunc(r.Violations, func(v Violation) bool { return v.Severity == Deny })
}

// Sort orders violations by path, line, severity (deny first), rule and
// message, so that the output is the same on every run.
func Sort(vs []Violation) {
	slices.SortFunc(vs, func(a, b Violation) int {
		return cmp.Or(
			strings.Compare(a.Path, b.Path),
			cmp.Compare(a.Line, b.Line),
			strings.Compare(a.Severity, b.Severity),
			strings.Compare(a.Rule, b.Rule),
			strings.Compare(a.Message, b.Message),
		)
	})
}
