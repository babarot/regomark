package report

import (
	"bytes"
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
		if err := Write(&b, tt.format, tt.vs); err != nil {
			t.Fatal(err)
		}
		if b.String() != tt.want {
			t.Errorf("%s:\n got %q\nwant %q", tt.format, b.String(), tt.want)
		}
	}
}

func TestWriteUnknownFormat(t *testing.T) {
	if err := Write(&bytes.Buffer{}, "xml", nil); err == nil {
		t.Error("Write accepted xml")
	}
}
