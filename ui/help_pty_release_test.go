//go:build release

package ui

// The release tier's pty test: it runs at `task release`, with -tags release, and not at a push.
// A pty that answers no query makes fang and lipgloss wait 2 s for each of two (4.2 s a run), which
// is theirs and not go-console's, and too long for a push.

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// Help fits the terminal at 80, 60 and 40 columns under a pty, every row of it, and returns
// within a bound where the pty answers no query.
func TestHelpFitsAPty(t *testing.T) {
	t.Parallel()
	const bound = 10 * time.Second
	for _, cols := range []int{80, 60, 40} {
		t.Run(strconv.Itoa(cols), func(t *testing.T) {
			t.Parallel()
			got, took := helpRun(t, cols, []string{"TERM=xterm-256color"}, false, "--help")
			if took > bound {
				t.Errorf("help took %v at a pty that answers no query, over %v", took, bound)
			}
			for i, l := range strings.Split(got, "\n") {
				if w := ansi.StringWidth(strings.TrimRight(l, "\r ")); w > cols {
					t.Errorf("row %d is %d columns wide on a %d-column screen: %q", i+1, w, cols, l)
				}
			}
		})
	}
}
