package policy

import (
	"bytes"
	"context"
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/babarot/regomark/internal/input"
)

// askRego asks two questions about every document: one about its
// paragraphs, one about the whole document.
const askRego = `package docs

questions contains {"id": "para", "path": d.path, "line": 3, "blocks": [b | some b in d.blocks; b.kind == "paragraph"], "ask": "Is it short?"} if {
	some d in input.documents
}

questions contains {"id": "all", "path": d.path, "line": 1, "blocks": d.blocks, "ask": "One topic?"} if {
	some d in input.documents
}
`

func load(t *testing.T, files map[string]string) *Set {
	t.Helper()
	set, err := Load(write(t, files), "policy", "docs")
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestQuestions(t *testing.T) {
	set := load(t, map[string]string{"schema.rego": schemaRego, "ask.rego": askRego, "rules.rego": "package docs\ndeny contains 1 if false\n"})
	qs, err := set.Questions(context.Background(), testInput())
	if err != nil {
		t.Fatal(err)
	}
	blocks := testInput().Documents[0].Blocks
	want := []Question{
		{ID: "all", Path: "docs/a.md", Line: 1, Blocks: blocks, Ask: "One topic?"},
		{ID: "para", Path: "docs/a.md", Line: 3, Blocks: blocks[1:], Ask: "Is it short?"},
	}
	if !reflect.DeepEqual(qs, want) {
		t.Errorf("questions:\n got %+v\nwant %+v", qs, want)
	}
	wantJudgments := []input.Judgment{
		{ID: "all", Path: "docs/a.md", Line: 1, Status: input.StatusNotRun},
		{ID: "para", Path: "docs/a.md", Line: 3, Status: input.StatusNotRun},
	}
	if js := NotRun(qs); !reflect.DeepEqual(js, wantJudgments) {
		t.Errorf("NotRun:\n got %+v\nwant %+v", js, wantJudgments)
	}
}

func TestQuestionsNone(t *testing.T) {
	set := load(t, map[string]string{"schema.rego": schemaRego, "rules.rego": "package docs\ndeny contains 1 if false\n"})
	qs, err := set.Questions(context.Background(), testInput())
	if err != nil {
		t.Fatal(err)
	}
	if qs == nil || len(qs) != 0 {
		t.Errorf("questions = %#v, want an empty, non-nil slice", qs)
	}
}

func TestQuestionsErrors(t *testing.T) {
	// q builds a question with the fields replaced or, for a "-" value, left out
	q := func(fields map[string]string) string {
		f := map[string]string{
			"id": `"q"`, "path": `"docs/a.md"`, "line": "1", "blocks": "input.documents[0].blocks", "ask": `"Why?"`,
		}
		maps.Copy(f, fields)
		var parts []string
		for _, k := range []string{"id", "path", "line", "blocks", "ask", "extra"} {
			if v, ok := f[k]; ok && v != "-" {
				parts = append(parts, `"`+k+`": `+v)
			}
		}
		return "questions contains {" + strings.Join(parts, ", ") + "} if true\n"
	}
	tests := []struct {
		name, rules, want string
	}{
		{"not a set", `questions := {"a": 1}`, "data.docs.questions: must be a set, not object"},
		{"not an object", `questions contains "q" if true`, "a question must be an object"},
		{"unknown field", q(map[string]string{"extra": "1"}), `question "q" at "docs/a.md":1: unknown field "extra"`},
		{"missing field", q(map[string]string{"ask": "-"}), `question "q" at "docs/a.md":1: ask is missing`},
		{"missing id", q(map[string]string{"id": "-"}), "a question: id is missing"},
		{"uppercase id", q(map[string]string{"id": `"Q"`}), "id must be a string of lowercase letters"},
		{"not a document", q(map[string]string{"path": `"docs/b.md"`}), `path "docs/b.md" is not a document of the input`},
		{"line 0", q(map[string]string{"line": "0"}), "line must be an integer from 1"},
		{"empty ask", q(map[string]string{"ask": `" "`}), "ask must be a non-empty string"},
		{"no blocks", q(map[string]string{"blocks": "[]"}), "blocks must be a non-empty array"},
		{"blocks not an array", q(map[string]string{"blocks": "input.documents[0].blocks[0]"}), "blocks must be a non-empty array"},
		{"changed block", q(map[string]string{"blocks": `[object.union(input.documents[0].blocks[0], {"text": "B"})]`}), "blocks[0] is not a block of docs/a.md as the input has it"},
		{"block with an extra field", q(map[string]string{"blocks": `[object.union(input.documents[0].blocks[0], {"x": 1})]`}), "blocks[0] is not a block"},
		{"block of another line", q(map[string]string{"blocks": `[object.union(input.documents[0].blocks[1], {"line": 4})]`}), "blocks[0] is not a block"},
		{"same id, path and line", q(nil) + q(map[string]string{"ask": `"Why not?"`}), `more than one question "q" at docs/a.md:1`},
		{"input type error", `questions contains 1 if input.documents[_].titel`, "undefined ref"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := load(t, map[string]string{"schema.rego": schemaRego, "rules.rego": "package docs\n\ndeny contains 1 if false\n\n" + tt.rules})
			_, err := set.Questions(context.Background(), testInput())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestQuestionsWithJudgments(t *testing.T) {
	set := load(t, map[string]string{"schema.rego": schemaRego, "rules.rego": "package docs\ndeny contains 1 if false\n"})
	in := testInput()
	in.Judgments = []input.Judgment{{ID: "q", Path: "docs/a.md", Line: 1, Status: input.StatusNotRun}}
	if _, err := set.Questions(context.Background(), in); err == nil {
		t.Error("Questions accepted an input with judgments")
	}
}

// judgeRego reads the judgments in warn, and reports in deny whatever the
// documents have.
const judgeRego = `package docs

deny contains {"rule": "A", "path": d.path, "line": 1, "message": "deny"} if {
	some d in input.documents
}

warn contains {"rule": "B", "path": j.path, "line": j.evidence_line, "message": j.reason} if {
	some j in input.judgments
	j.status == "ok"
	j.verdict == "yes"
}

warn contains {"rule": "B", "path": j.path, "line": j.line, "message": "not run"} if {
	some j in input.judgments
	j.status == "not_run"
}

warn contains {"rule": "B", "path": "docs/a.md", "line": 1, "message": "printed"} if print("evaluated")
`

func TestEvalJudgments(t *testing.T) {
	set := load(t, map[string]string{"schema.rego": schemaRego, "rules.rego": judgeRego})
	judgments := []input.Judgment{
		{ID: "all", Path: "docs/a.md", Line: 1, Status: input.StatusOK, Verdict: "yes", Reason: "two topics", EvidenceLine: 3},
		{ID: "para", Path: "docs/a.md", Line: 3, Status: input.StatusNotRun},
	}
	var printed bytes.Buffer
	res, err := set.Eval(context.Background(), testInput(), judgments, &printed)
	if err != nil {
		t.Fatal(err)
	}
	want := []Violation{
		{Rule: "A", Severity: Deny, Path: "docs/a.md", Line: 1, Message: "deny"},
		{Rule: "B", Severity: Warn, Path: "docs/a.md", Line: 1, Message: "printed"},
		{Rule: "B", Severity: Warn, Path: "docs/a.md", Line: 3, Message: "not run"},
		{Rule: "B", Severity: Warn, Path: "docs/a.md", Line: 3, Message: "two topics"},
	}
	if !reflect.DeepEqual(res.Violations, want) {
		t.Errorf("violations:\n got %+v\nwant %+v", res.Violations, want)
	}
	// Evaluated twice, printed once
	if n := strings.Count(printed.String(), "evaluated"); n != 1 {
		t.Errorf("print output = %q, want it once", printed.String())
	}
}

func TestEvalDenyReadsJudgments(t *testing.T) {
	rules := `package docs

deny contains {"rule": "A", "path": j.path, "line": j.line, "message": "judged"} if {
	some j in input.judgments
}
`
	set := load(t, map[string]string{"schema.rego": schemaRego, "rules.rego": rules})
	judgments := []input.Judgment{{ID: "all", Path: "docs/a.md", Line: 1, Status: input.StatusNotRun}}
	_, err := set.Eval(context.Background(), testInput(), judgments, &bytes.Buffer{})
	want := "data.docs.deny depends on input.judgments: it reports different violations with them (1) than without them (0)"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v, want one containing %q", err, want)
	}

	// Without judgments there is nothing to compare, and deny sees none
	res, err := set.Eval(context.Background(), testInput(), nil, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Violations) != 0 {
		t.Errorf("violations = %+v, want none", res.Violations)
	}
}

func TestEvalJudgmentErrors(t *testing.T) {
	set := load(t, map[string]string{"schema.rego": schemaRego, "rules.rego": judgeRego})

	in := testInput()
	in.Judgments = []input.Judgment{{ID: "all", Path: "docs/a.md", Line: 1, Status: input.StatusNotRun}}
	if _, err := set.Eval(context.Background(), in, nil, &bytes.Buffer{}); err == nil {
		t.Error("Eval accepted an input with judgments")
	}

	bad := []input.Judgment{{ID: "all", Path: "docs/a.md", Line: 1, Status: "skipped"}}
	if _, err := set.Eval(context.Background(), testInput(), bad, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "/judgments/0/status") {
		t.Errorf("error = %v, want one about the status", err)
	}
}

func TestLoadQuestionRules(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string // a substring of the error; empty means Load succeeds
	}{
		{"questions in their own module", map[string]string{
			"schema.rego": schemaRego, "ask.rego": askRego, "rules.rego": "package docs\nwarn contains 1 if false\n",
		}, ""},
		{"questions only", map[string]string{"schema.rego": schemaRego, "ask.rego": askRego},
			"no module in policy defines deny or warn in package docs"},
		{"questions as a function", map[string]string{
			"schema.rego": schemaRego, "rules.rego": "package docs\ndeny contains 1 if false\n\nquestions(d) := 1 if d\n",
		}, "policy/rules.rego:4: questions must be a set of questions, not a function"},
		{"questions in a library", map[string]string{
			"rules.rego":    "package docs\nimport data.lib.util\ndeny contains 1 if util.x\n",
			"lib/util.rego": "package lib.util\nx := true\nquestions contains 1 if true\n",
		}, "policy/lib/util.rego: a library module in lib/ must not define questions"},
		{"misspelled questions", map[string]string{
			"schema.rego": schemaRego, "ask.rego": "package docs\nquestion contains 1 if true\n", "rules.rego": "package docs\ndeny contains 1 if false\n",
		}, "policy/ask.rego: defines none of deny, warn and questions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(write(t, tt.files), "policy", "docs")
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}
