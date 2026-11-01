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
