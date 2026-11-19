package glob

import (
	"path"
	"testing"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"**/AGENTS.md", "AGENTS.md", true},
		{"**/AGENTS.md", "a/AGENTS.md", true},
		{"**/AGENTS.md", "a/b/c/AGENTS.md", true},
		{"**/AGENTS.md", "a/xAGENTS.md", false},
		{"**/AGENTS.md", "AGENTS.md/x", false},
		{"docs/**/*.md", "docs/a.md", true},
		{"docs/**/*.md", "docs/x/y/a.md", true},
		{"docs/**/*.md", "docs/x/a.txt", false},
		{"docs/**/*.md", "other/a.md", false},
		{"docs/**/*.md", "a.md", false},
		{"docs/**", "docs/x/y.md", true},
		{"**", "a/b.md", true},
		{"a/**/**/b.md", "a/b.md", true},
		{"a/**/b/**/c.md", "a/x/b/y/z/c.md", true},
		{"a/**/b/**/c.md", "a/x/y/c.md", false},
		{"**/[ab].md", "x/b.md", true},
		// ** inside a segment is two *, as in path.Match
		{"docs/**.md", "docs/a.md", true},
		{"docs/**.md", "docs/x/a.md", false},
		{"docs/000-*.md", "docs/000-a.md", true},
		{"docs/000-*.md", "docs/sub/000-a.md", false},
		{"[", "[", false},
		{"**/[", "a/[", false},
	}
	for _, tt := range tests {
		if got := Match(tt.pattern, tt.name); got != tt.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}

// A pattern without a ** segment matches what path.Match matches
func TestMatchAsPathMatch(t *testing.T) {
	names := []string{"docs/a.md", "docs/000-a.md", "docs/x/a.md", "a.md", "docs/[x].md", "docs/b.md"}
	for _, pattern := range []string{"docs/*.md", "docs/000-*.md", "*", "*/*", "docs/[ab].md", "docs/[^a].md", `docs/\[x\].md`, "docs/?.md", "docs/**.md"} {
		for _, name := range names {
			want, _ := path.Match(pattern, name)
			if got := Match(pattern, name); got != want {
				t.Errorf("Match(%q, %q) = %v, path.Match says %v", pattern, name, got, want)
			}
		}
	}
}

func TestValidate(t *testing.T) {
	for pattern, valid := range map[string]bool{
		"docs/*.md":    true,
		"**/AGENTS.md": true,
		"docs/**/*.md": true,
		"[":            false,
		"**/[":         false,
		"[/**":         false,
		`docs/\`:       false,
	} {
		if err := Validate(pattern); (err == nil) != valid {
			t.Errorf("Validate(%q) = %v, want valid %v", pattern, err, valid)
		}
	}
}

func TestHasMeta(t *testing.T) {
	for s, want := range map[string]bool{"docs": false, "docs/adr": false, "*.md": true, "a?": true, "[ab]": true} {
		if got := HasMeta(s); got != want {
			t.Errorf("HasMeta(%q) = %v, want %v", s, got, want)
		}
	}
}
