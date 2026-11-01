package policy

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/babarot/regomark/internal/input"
)

const schemaRego = `package docs

schema_version := 1

rule_ids := {"A", "B"}
`

// write lays out a policy directory "policy" in a new root.
func write(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, "policy", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func testInput() *input.Input {
	return &input.Input{SchemaVersion: input.SchemaVersion, Root: ".", Documents: []input.Document{{
		Path: "docs/a.md", Name: "a.md",
		Blocks: []input.Block{{Kind: "heading", Level: 1, Line: 1, Text: "A"}, {Kind: "paragraph", Line: 3, Text: "x"}},
		Links:  []input.Link{},
	}}}
}

func TestLoadPackageRules(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string // substrings of the error; none means Load succeeds
	}{
		{"valid", map[string]string{
			"schema.rego":      schemaRego,
			"rules.rego":       "package docs\nimport data.lib.util\nwarn contains 1 if util.x\n",
			"lib/util.rego":    "package lib.util\nimport data.lib.base\nx := base.y\n",
			"lib/base.rego":    "package lib.base\ny := true\n",
			"rules_test.rego":  "package other_test\ntest_x if true\n",
			"sub/schema.rego":  "package docs\nz := 1\n",
			"sub/more.rego":    "package docs\ndeny contains 1 if false\n",
			"README.md":        "not rego",
			"lib/x_test.rego":  "package lib.unused_test\n",
			"lib/sub/top.rego": "package lib.util.sub\nq := 1\n",
		}, nil},
		{"child package", map[string]string{"schema.rego": schemaRego, "x.rego": "package docs.x\ndeny contains 1 if true\n"},
			[]string{"policy/x.rego: package docs.x must be docs"}},
		{"other package", map[string]string{"schema.rego": "package adr\nschema_version := 1\n"},
			[]string{"policy/schema.rego: package adr must be docs"}},
		{"neither deny nor warn", map[string]string{"schema.rego": schemaRego, "x.rego": "package docs\ndenies contains 1 if true\n"},
			[]string{"policy/x.rego: defines neither deny nor warn"}},
		{"library defines deny and warn", map[string]string{
			"rules.rego":    "package docs\nimport data.lib.util\ndeny contains 1 if util.x\n",
			"lib/util.rego": "package lib.util\nx := true\ndeny contains 1 if true\nwarn contains 1 if true\n",
		}, []string{"policy/lib/util.rego: a library module in lib/ must not define deny or warn"}},
		{"library imported by nobody", map[string]string{
			"rules.rego":    "package docs\ndeny contains 1 if true\n",
			"lib/util.rego": "package lib.util\nimport data.lib.util\nx := true\n",
		}, []string{"policy/lib/util.rego: no other module imports the library package lib.util"}},
		{"deny as a function", map[string]string{"schema.rego": schemaRego, "x.rego": "package docs\n\ndeny(doc) := 1 if doc\n"},
			[]string{"policy/x.rego:3: deny must be a set of violations, not a function"}},
		{"every error at once", map[string]string{
			"a.rego": "package a\n", "b.rego": "package docs\n",
		}, []string{"package a must be docs", "b.rego: defines neither"}},
		{"no files", map[string]string{"x_test.rego": "package docs_test\n"}, []string{"no .rego files in policy"}},
		{"syntax error", map[string]string{"x.rego": "package docs\ndeny contains if\n"}, []string{"policy/x.rego"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(write(t, tt.files), "policy", "docs")
			if len(tt.want) == 0 {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("Load succeeded")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error = %v, want one containing %q", err, w)
				}
			}
		})
	}
}

func TestLoadMissingDirectory(t *testing.T) {
	if _, err := Load(t.TempDir(), "policy", "docs"); err == nil || !strings.Contains(err.Error(), "policy does not exist") {
		t.Errorf("error = %v", err)
	}
}

func TestLoadSymlinkedDirectory(t *testing.T) {
	root := write(t, map[string]string{"schema.rego": schemaRego, "rules.rego": "package docs\ndeny contains 1 if false\n"})
	if err := os.Symlink("policy", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, "link", "docs"); err != nil {
		t.Error(err)
	}
}

func TestEval(t *testing.T) {
	root := write(t, map[string]string{
		"schema.rego": schemaRego,
		"rules.rego": `package docs

deny contains {"rule": "A", "path": d.path, "line": b.line, "message": b.text} if {
	some d in input.documents
	some b in d.blocks
}

warn contains {"rule": "B", "path": "docs/a.md", "line": 1, "message": "w"} if print("hello")
`,
	})
	set, err := Load(root, "policy", "docs")
	if err != nil {
		t.Fatal(err)
	}
	var printed bytes.Buffer
	res, err := set.Eval(context.Background(), testInput(), &printed)
	if err != nil {
		t.Fatal(err)
	}
	want := []Violation{
		{Rule: "A", Severity: Deny, Path: "docs/a.md", Line: 1, Message: "A"},
		{Rule: "B", Severity: Warn, Path: "docs/a.md", Line: 1, Message: "w"},
		{Rule: "A", Severity: Deny, Path: "docs/a.md", Line: 3, Message: "x"},
	}
	if !reflect.DeepEqual(res.Violations, want) {
		t.Errorf("violations:\n got %+v\nwant %+v", res.Violations, want)
	}
	if !reflect.DeepEqual(res.RuleIDs, []string{"A", "B"}) {
		t.Errorf("rule IDs = %v", res.RuleIDs)
	}
	if !res.Denied() {
		t.Error("Denied = false")
	}
	if !strings.Contains(printed.String(), "hello") {
		t.Errorf("print output = %q", printed.String())
	}

	if err := res.Skip([]string{"A"}); err != nil {
		t.Fatal(err)
	}
	if len(res.Violations) != 1 || res.Denied() {
		t.Errorf("after skipping A: %+v", res.Violations)
	}
	if err := res.Skip([]string{"C"}); err == nil || !strings.Contains(err.Error(), "unknown rule ID C") {
		t.Errorf("Skip(C) error = %v", err)
	}
}

func TestEvalErrors(t *testing.T) {
	tests := []struct {
		name, schema, rules, want string
	}{
		{"schema version mismatch", "package docs\nschema_version := 2\nrule_ids := {\"A\"}\n", "", "written for input schema version 2"},
		{"schema version not a number", "package docs\nschema_version := \"1\"\nrule_ids := {\"A\"}\n", "", "not an integer"},
		{"no schema version", "package docs\nrule_ids := {\"A\"}\n", "", "does not define schema_version"},
		{"no rule IDs", "package docs\nschema_version := 1\n", "", "rule_ids: not defined"},
		{"rule IDs not strings", "package docs\nschema_version := 1\nrule_ids := {1}\n", "", "must be a set of non-empty strings"},
		{"not an object", schemaRego, `deny contains "x" if true`, "must be an object"},
		{"missing message", schemaRego, `deny contains {"rule": "A", "path": "a", "line": 1} if true`, "message is missing"},
		{"empty path", schemaRego, `deny contains {"rule": "A", "path": "", "line": 1, "message": "m"} if true`, "path must be a non-empty string"},
		{"line 0", schemaRego, `deny contains {"rule": "A", "path": "a", "line": 0, "message": "m"} if true`, "line must be an integer from 1"},
		{"fractional line", schemaRego, `deny contains {"rule": "A", "path": "a", "line": 1.5, "message": "m"} if true`, "line must be an integer from 1"},
		{"severity field", schemaRego, `warn contains {"rule": "A", "path": "a", "line": 1, "message": "m", "severity": "warn"} if true`, `unknown field "severity"`},
		{"unknown rule", schemaRego, `warn contains {"rule": "Z", "path": "a", "line": 1, "message": "m"} if true`, `rule "Z" is not in rule_ids`},
		{"warn not a set", schemaRego, `warn := {"a": 1}`, "data.docs.warn: must be a set, not object"},
		{"input type error", schemaRego, `deny contains 1 if input.documents[_].titel`, "undefined ref"},
		{"builtin error", schemaRego, `deny contains {"rule": "A", "path": "a", "line": to_number("x"), "message": "m"} if true`, "to_number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := tt.rules
			if rules == "" {
				rules = "deny contains 1 if false"
			}
			root := write(t, map[string]string{"schema.rego": tt.schema, "rules.rego": "package docs\n\n" + rules + "\n"})
			set, err := Load(root, "policy", "docs")
			if err != nil {
				t.Fatal(err)
			}
			_, err = set.Eval(context.Background(), testInput(), &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestSort(t *testing.T) {
	vs := []Violation{
		{Rule: "B", Severity: Warn, Path: "b.md", Line: 1, Message: "m"},
		{Rule: "A", Severity: Warn, Path: "a.md", Line: 2, Message: "m"},
		{Rule: "B", Severity: Deny, Path: "a.md", Line: 2, Message: "m"},
		{Rule: "A", Severity: Deny, Path: "a.md", Line: 10, Message: "m"},
		{Rule: "A", Severity: Deny, Path: "a.md", Line: 2, Message: "n"},
		{Rule: "A", Severity: Deny, Path: "a.md", Line: 2, Message: "m"},
	}
	Sort(vs)
	want := []Violation{
		{Rule: "A", Severity: Deny, Path: "a.md", Line: 2, Message: "m"},
		{Rule: "A", Severity: Deny, Path: "a.md", Line: 2, Message: "n"},
		{Rule: "B", Severity: Deny, Path: "a.md", Line: 2, Message: "m"},
		{Rule: "A", Severity: Warn, Path: "a.md", Line: 2, Message: "m"},
		{Rule: "A", Severity: Deny, Path: "a.md", Line: 10, Message: "m"},
		{Rule: "B", Severity: Warn, Path: "b.md", Line: 1, Message: "m"},
	}
	if !reflect.DeepEqual(vs, want) {
		t.Errorf("sorted:\n got %+v\nwant %+v", vs, want)
	}
}
