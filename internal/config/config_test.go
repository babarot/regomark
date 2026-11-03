package config

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	c, err := Parse([]byte(`
documents: ./docs/adr/
policy: .regomark/policy
entrypoint: adr
skip_rules: [ADR001]
exclude: [docs/adr/000-*.md]
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Documents != "docs/adr" || c.Policy != ".regomark/policy" || c.Entrypoint != "adr" {
		t.Errorf("got %+v", c)
	}
	if len(c.SkipRules) != 1 || c.SkipRules[0] != "ADR001" {
		t.Errorf("skip_rules = %v", c.SkipRules)
	}
	if !c.Excluded("docs/adr/000-template.md") || c.Excluded("docs/adr/001-a.md") {
		t.Errorf("exclude does not match as a pattern")
	}
}

func TestParseErrors(t *testing.T) {
	base := "documents: docs\npolicy: policy\nentrypoint: adr\n"
	tests := []struct {
		name, yaml, want string
	}{
		{"unknown key", base + "skip_rule: [A]\n", "field skip_rule is not a known key"},
		{"wrong type", base + "skip_rules: A\n", "cannot unmarshal"},
		{"empty", "", "documents: must be set"},
		{"no entrypoint", "documents: docs\npolicy: policy\n", "entrypoint: must be set"},
		{"entrypoint not a package", "documents: docs\npolicy: policy\nentrypoint: adr-x\n", `"adr-x" is not a package name`},
		{"absolute", "documents: /docs\npolicy: policy\nentrypoint: adr\n", "must be relative to the repository root"},
		{"outside", "documents: docs/../..\npolicy: policy\nentrypoint: adr\n", "must be inside the repository"},
		{"backslash", "documents: docs\\adr\npolicy: policy\nentrypoint: adr\n", "use forward slashes"},
		{"bad pattern", base + "exclude: ['[']\n", "is not a valid pattern"},
		{"empty skip rule", base + "skip_rules: ['']\n", "skip_rules: an entry is empty"},
		{"unknown judge key", base + "judge:\n  enabled: true\n", "field enabled is not a known key"},
		{"empty command", base + "judge:\n  command: []\n", "judge.command: must name a command"},
		{"command not a list", base + "judge:\n  command: claude -p\n", "cannot unmarshal"},
		{"negative parallel", base + "judge:\n  parallel: -1\n", "judge.parallel: must be at least 1"},
		{"bad timeout", base + "judge:\n  timeout: 2m30\n", `judge.timeout: "2m30" is not a positive duration`},
		{"zero timeout", base + "judge:\n  timeout: 0s\n", "judge.timeout"},
		{"negative max questions", base + "judge:\n  max_questions: -5\n", "judge.max_questions: must be at least 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestParseDottedEntrypoint(t *testing.T) {
	if _, err := Parse([]byte("documents: .\npolicy: p\nentrypoint: docs.adr\n")); err != nil {
		t.Error(err)
	}
}

func TestParseJudge(t *testing.T) {
	base := "documents: docs\npolicy: policy\nentrypoint: adr\n"
	c, err := Parse([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	j := c.Judge
	if j.Command != nil || j.Parallel != DefaultParallel || j.TimeoutDuration != DefaultTimeout || j.MaxQuestions != DefaultMaxQuestions || j.Cache != "" {
		t.Errorf("defaults = %+v", j)
	}
	c, err = Parse([]byte(base + "judge:\n  command: [sh, -c, 'claude -p']\n  parallel: 2\n  timeout: 30s\n  cache: .cache/regomark\n  max_questions: 10\n"))
	if err != nil {
		t.Fatal(err)
	}
	j = c.Judge
	if len(j.Command) != 3 || j.Command[2] != "claude -p" || j.Parallel != 2 || j.TimeoutDuration.String() != "30s" || j.Cache != ".cache/regomark" || j.MaxQuestions != 10 {
		t.Errorf("judge = %+v", j)
	}
}
