package cli

import (
	"fmt"
	"io"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
)

// terminal returns the width of w in cells if w is a terminal.
func terminal(w io.Writer) (width int, ok bool) {
	f, isFile := w.(*os.File)
	if !isFile || !term.IsTerminal(f.Fd()) {
		return 0, false
	}
	width, _, err := term.GetSize(f.Fd())
	if err != nil {
		return 0, true
	}
	return width, true
}

// colored returns a writer to w that passes on as much of the ANSI escape
// sequences as w and the environment take: none unless w is a terminal, and
// no colors if NO_COLOR is set to anything but empty (https://no-color.org).
// colorprofile reads NO_COLOR as a boolean, so NO_COLOR=yes would not stop
// it alone.
func colored(w io.Writer) *colorprofile.Writer {
	cw := colorprofile.NewWriter(w, os.Environ())
	if os.Getenv("NO_COLOR") != "" && cw.Profile > colorprofile.ASCII {
		cw.Profile = colorprofile.ASCII
	}
	return cw
}

var faint = lipgloss.NewStyle().Faint(true)

// note writes a line of progress to w, faint at a terminal so that it stands
// back from the violations.
func note(w io.Writer, format string, args ...any) {
	fmt.Fprintln(colored(w), faint.Render(fmt.Sprintf(format, args...)))
}
