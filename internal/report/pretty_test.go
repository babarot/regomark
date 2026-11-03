package report

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/regomark/internal/policy"
)

func TestPretty(t *testing.T) {
	vs := []policy.Violation{
		{Rule: "R1", Severity: policy.Deny, Path: "docs/a.md", Line: 3, Message: "broken: x.md"},
		{Rule: "R2", Severity: policy.Warn, Path: "docs/a.md", Line: 12, Message: "one\ntwo"},
		{Rule: "R3", Severity: policy.Warn, Path: "docs/b.md", Line: 1, Message: "これは日本語の長いメッセージで、端末の幅で折り返される。"},
	}
	var b strings.Builder
	if err := Pretty(&b, vs, 30); err != nil {
		t.Fatal(err)
	}
	want := `docs/a.md
   3  error    R1
      broken: x.md
  12  warning  R2
      one
      two

docs/b.md
  1  warning  R3
     これは日本語の長いメッセ
     ージで、端末の幅で折り返
     される。

✖ 3 problems (1 error, 2 warnings)
`
	if got := ansi.Strip(b.String()); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if !strings.Contains(b.String(), "\x1b[") {
		t.Error("no ANSI escape sequences")
	}
}

func TestPrettySummary(t *testing.T) {
	tests := []struct {
		vs   []policy.Violation
		want string
	}{
		{nil, "✔ no problems\n"},
		{[]policy.Violation{{Rule: "R", Severity: policy.Warn, Path: "a.md", Line: 1, Message: "m"}}, "⚠ 1 problem (0 errors, 1 warning)\n"},
	}
	for _, tt := range tests {
		var b strings.Builder
		if err := Pretty(&b, tt.vs, 0); err != nil {
			t.Fatal(err)
		}
		if got := ansi.Strip(b.String()); !strings.HasSuffix(got, tt.want) {
			t.Errorf("got %q, want it to end with %q", got, tt.want)
		}
	}
}
