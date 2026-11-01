// Package report writes the violations in the formats regomark check offers.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/babarot/regomark/internal/policy"
)

// Formats are the names --format accepts.
var Formats = []string{"text", "github", "json"}

// Write writes the violations, already sorted, in the format.
func Write(w io.Writer, format string, vs []policy.Violation) error {
	switch format {
	case "text":
		return text(w, vs)
	case "github":
		return github(w, vs)
	case "json":
		return jsonFormat(w, vs)
	default:
		return fmt.Errorf("unknown format %q (want one of %s)", format, strings.Join(Formats, ", "))
	}
}

// text writes a line per violation, as compilers do:
//
//	path:line: [rule] message
//	path:line: warning: [rule] message
func text(w io.Writer, vs []policy.Violation) error {
	for _, v := range vs {
		prefix := ""
		if v.Severity == policy.Warn {
			prefix = "warning: "
		}
		if _, err := fmt.Fprintf(w, "%s:%d: %s[%s] %s\n", v.Path, v.Line, prefix, v.Rule, v.Message); err != nil {
			return err
		}
	}
	return nil
}

// github writes workflow commands, which GitHub Actions shows as annotations
// on the lines.
func github(w io.Writer, vs []policy.Violation) error {
	for _, v := range vs {
		cmd := "error"
		if v.Severity == policy.Warn {
			cmd = "warning"
		}
		if _, err := fmt.Fprintf(w, "::%s file=%s,line=%d,title=%s::%s\n",
			cmd, escapeProperty(v.Path), v.Line, escapeProperty(v.Rule), escapeData("["+v.Rule+"] "+v.Message)); err != nil {
			return err
		}
	}
	return nil
}

// escapeData escapes the message of a workflow command.
func escapeData(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}

// escapeProperty escapes a property value of a workflow command.
func escapeProperty(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C").Replace(s)
}

// jsonFormat writes the violations as a JSON array.
func jsonFormat(w io.Writer, vs []policy.Violation) error {
	if vs == nil {
		vs = []policy.Violation{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(vs)
}
