package input

import "testing"

func TestResolve(t *testing.T) {
	tests := []struct {
		dest, target string
		external     bool
	}{
		{"b.md", "docs/adr/b.md", false},
		{"./b.md#L3", "docs/adr/b.md", false},
		{"b.md?plain=1#x", "docs/adr/b.md", false},
		{"../../main.go", "main.go", false},
		{"../", "docs", false},
		{"../..", ".", false},
		{"/README.md", "README.md", false},
		{"//README.md", "README.md", false},
		{"my%20file.md", "docs/adr/my file.md", false},
		{"a%23b.md#c", "docs/adr/a#b.md", false},
		{"100%.md", "docs/adr/100%.md", false},
		{"", "docs/adr/a.md", false},
		{"?x=1", "docs/adr/a.md", false},
		{"../../../x.md", "../x.md", false},
		{"https://example.com/a.md", "", true},
		{"mailto:a@example.com", "", true},
		{"#section", "", true},
	}
	for _, tt := range tests {
		target, external := resolve("docs/adr/a.md", tt.dest)
		if target != tt.target || external != tt.external {
			t.Errorf("resolve(%q) = %q, %v; want %q, %v", tt.dest, target, external, tt.target, tt.external)
		}
	}
}

func TestOutside(t *testing.T) {
	for p, want := range map[string]bool{"..": true, "../x": true, "..x": false, "x/..y": false, ".": false} {
		if got := outside(p); got != want {
			t.Errorf("outside(%q) = %v, want %v", p, got, want)
		}
	}
}
