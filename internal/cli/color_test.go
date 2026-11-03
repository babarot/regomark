package cli

import (
	"bytes"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

func TestColoredNoColor(t *testing.T) {
	tests := []struct {
		noColor string
		want    colorprofile.Profile
	}{
		{"", colorprofile.ANSI},
		{"1", colorprofile.ASCII},
		{"yes", colorprofile.ASCII}, // any value but empty, which colorprofile alone ignores
	}
	for _, tt := range tests {
		// CLICOLOR_FORCE stands in for a terminal
		t.Setenv("CLICOLOR_FORCE", "1")
		t.Setenv("TERM", "xterm")
		t.Setenv("COLORTERM", "")
		t.Setenv("NO_COLOR", tt.noColor)
		if got := colored(&bytes.Buffer{}).Profile; got != tt.want {
			t.Errorf("NO_COLOR=%q: profile %v, want %v", tt.noColor, got, tt.want)
		}
	}
}

func TestColoredNotTerminal(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("NO_COLOR", "")
	var b bytes.Buffer
	note(&b, "regomark: %s", "x")
	if got := b.String(); got != "regomark: x\n" {
		t.Errorf("got %q", got)
	}
}
