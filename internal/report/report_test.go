package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/babarot/regomark/internal/policy"
)

var violations = []policy.Violation{
	{Rule: "R1", Severity: policy.Deny, Path: "docs/a.md", Line: 3, Message: "broken: x.md"},
	{Rule: "R2", Severity: policy.Warn, Path: "docs/a,b.md", Line: 7, Message: "100% sure\nreally"},
}

func TestWrite(t *testing.T) {
	tests := []struct {
		format string
		vs     []policy.Violation
		want   string
	}{
		{"text", violations, "docs/a.md:3: [R1] broken: x.md\ndocs/a,b.md:7: warning: [R2] 100% sure\nreally\n"},
		{"github", violations, "::error file=docs/a.md,line=3,title=R1::[R1] broken: x.md\n" +
			"::warning file=docs/a%2Cb.md,line=7,title=R2::[R2] 100%25 sure%0Areally\n"},
		{"json", violations[:1], `[
  {
    "rule": "R1",
    "severity": "deny",
    "path": "docs/a.md",
    "line": 3,
    "message": "broken: x.md"
  }
]
`},
		{"json", nil, "[]\n"},
		{"text", nil, ""},
	}
	for _, tt := range tests {
		var b bytes.Buffer
		if err := Write(&b, tt.format, nil, tt.vs, 0); err != nil {
			t.Fatal(err)
		}
		if b.String() != tt.want {
			t.Errorf("%s:\n got %q\nwant %q", tt.format, b.String(), tt.want)
		}
	}
}

func TestWriteUnknownFormat(t *testing.T) {
	if err := Write(&bytes.Buffer{}, "xml", nil, nil, 0); err == nil {
		t.Error("Write accepted xml")
	}
}

func TestPathURI(t *testing.T) {
	tests := []struct{ path, want string }{
		{"docs/a.md", "docs/a.md"},
		{"docs/a b%.md", "docs/a%20b%25.md"},
		{"docs/#1?.md", "docs/%231%3F.md"},
		{"a:b.md", "./a:b.md"},
		{"docs/日本.md", "docs/%E6%97%A5%E6%9C%AC.md"},
	}
	for _, tt := range tests {
		if got := pathURI(tt.path); got != tt.want {
			t.Errorf("pathURI(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestSARIFEscapesBraces(t *testing.T) {
	var b bytes.Buffer
	vs := []policy.Violation{{Rule: "R1", Severity: policy.Deny, Path: "docs/a.md", Line: 1, Message: "want {0} and }{"}}
	if err := Write(&b, "sarif", []string{"R1"}, vs, 0); err != nil {
		t.Fatal(err)
	}
	if want := `"text": "want {{0}} and }}{{"`; !strings.Contains(b.String(), want) {
		t.Errorf("got\n%s\nwant it to contain %s", b.String(), want)
	}
}
