package uitest_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/birdtable/go-console/internal/english"
	"github.com/birdtable/go-console/ui"
	"github.com/birdtable/go-console/uitest"
)

// followScreen is a program's follow driver as a caller's test holds it.
func followScreen(d *ui.FollowDriver) uitest.Screen { return uitest.ModelScreen(d) }

// wider is the first row of frame that is wider than w cells, if there is one.
func wider(frame string, w int) (string, bool) {
	for row := range strings.SplitSeq(frame, "\n") {
		if ansi.StringWidth(row) > w {
			return row, true
		}
	}
	return "", false
}

// A program that follows a stream draws its viewport at the widths it supports
// and compares it with a golden, with no terminal: lines in, a resize, the end.
//
// What it costs when this fails: a program's viewport cannot be tested where
// it lives, and a line drawn past the screen ships unseen.
func TestAProgramDrawsItsFollowViewportAtTwoWidths(t *testing.T) { //nolint:paralleltest // serial: moved as it stands, not yet made parallel
	long := strings.Repeat("a very long line from a guest ", 8)
	for _, w := range []int{80, 120} {
		d := ui.NewFollowDriver(ui.New(ui.WithMessages(english.Messages())).NewFollow())
		s := followScreen(d)
		uitest.Run(s, w, 6)
		for i := 1; i <= 20; i++ {
			uitest.Feed(s, d.Line(fmt.Sprintf("line %02d of the stream", i)))
		}
		uitest.Feed(s, d.Line(long+"\x1b[31mred\x1b[0m"))
		res := uitest.Feed(s, d.Line("last line"))
		uitest.Golden(t, fmt.Sprintf("follow-%d", w), res.Frame)
		if row, over := wider(res.Frame, w); over {
			t.Errorf("width %d: a row is wider than the screen: %q", w, row)
		}
		if !strings.Contains(res.Frame, "last line") || !strings.Contains(res.Frame, "following") || strings.Contains(res.Frame, "\x1b") {
			t.Errorf("width %d:\n%s", w, res.Frame)
		}

		// A resize redraws at the new size, still at the bottom.
		res = uitest.Run(s, w/2, 4)
		if _, over := wider(res.Frame, w/2); over || !strings.Contains(res.Frame, "last line") {
			t.Errorf("width %d, resized to %d:\n%s", w, w/2, res.Frame)
		}

		// The end: the screen asks to leave, and a line after it is dropped.
		if res = uitest.Feed(s, d.Stop()); !res.Quit {
			t.Errorf("width %d: Stop did not end the viewport", w)
		}
		if res = uitest.Feed(s, d.Line("after the end")); strings.Contains(res.Frame, "after the end") {
			t.Errorf("width %d: a line after Stop was drawn", w)
		}
	}
}

// The width check itself fails on a row past the screen: the planted twin of a
// driver that draws past its width.
func TestARowPastTheWidthIsCaught(t *testing.T) { //nolint:paralleltest // serial: moved as it stands, not yet made parallel
	if _, over := wider("fits\n"+strings.Repeat("x", 81), 80); !over {
		t.Fatal("a row of 81 cells passed for 80")
	}
	if _, over := wider("fits\n"+strings.Repeat("x", 80), 80); over {
		t.Fatal("a row of 80 cells failed for 80")
	}
}
