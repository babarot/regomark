package report

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/regomark/internal/policy"
)

var (
	pathStyle    = lipgloss.NewStyle().Bold(true)
	faintStyle   = lipgloss.NewStyle().Faint(true)
	denyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Red)
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Green)
	summaryStyle = lipgloss.NewStyle().Bold(true)
)

// minWrap is the narrowest a message is wrapped to, however narrow the
// terminal.
const minWrap = 20

// Pretty writes the violations for a person at a terminal: grouped by file,
// a line, the severity and the rule above each message, which is wrapped to
// fit width cells (not at all if width is 0), and a count at the end.
//
//	docs/a.md
//	   3  error    R1
//	      broken: x.md
//
//	✖ 1 problem (1 error, 0 warnings)
//
// It colors them with ANSI escape sequences; w is to drop them where colors
// are not wanted.
func Pretty(w io.Writer, vs []policy.Violation, width int) error {
	var b strings.Builder
	if len(vs) == 0 {
		b.WriteString(okStyle.Render("✔") + " no problems\n")
		_, err := io.WriteString(w, b.String())
		return err
	}
	denies := 0
	for i := 0; i < len(vs); {
		// The violations are sorted, so those of a file are together
		j := i
		for j < len(vs) && vs[j].Path == vs[i].Path {
			j++
		}
		writeFile(&b, vs[i:j], width)
		b.WriteString("\n")
		for _, v := range vs[i:j] {
			if v.Severity != policy.Warn {
				denies++
			}
		}
		i = j
	}
	mark := denyStyle.Render("✖")
	if denies == 0 {
		mark = warnStyle.Render("⚠")
	}
	fmt.Fprintf(&b, "%s %s\n", mark, summaryStyle.Render(fmt.Sprintf("%s (%s, %s)",
		plural(len(vs), "problem"), plural(denies, "error"), plural(len(vs)-denies, "warning"))))
	_, err := io.WriteString(w, b.String())
	return err
}

// writeFile writes the violations of one file.
func writeFile(b *strings.Builder, vs []policy.Violation, width int) {
	b.WriteString(pathStyle.Render(vs[0].Path) + "\n")
	lw := 0
	for _, v := range vs {
		lw = max(lw, len(strconv.Itoa(v.Line)))
	}
	indent := strings.Repeat(" ", 2+lw+2)
	for _, v := range vs {
		sev := denyStyle.Render("error  ")
		if v.Severity == policy.Warn {
			sev = warnStyle.Render("warning")
		}
		fmt.Fprintf(b, "  %s  %s  %s\n", faintStyle.Render(fmt.Sprintf("%*d", lw, v.Line)), sev, faintStyle.Render(v.Rule))
		msg := v.Message
		if width > 0 {
			msg = ansi.Wrap(msg, max(width-len(indent), minWrap), "")
		}
		for line := range strings.Lines(msg) {
			b.WriteString(indent + strings.TrimRight(line, " \n") + "\n")
		}
	}
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
