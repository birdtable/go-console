package ui

import (
	"strings"
	"testing"
)

// Text is paged only at a terminal that renders, never with --plain, and never
// when it fits.
//
// What it costs when this fails: a script or a --plain run has its output held in
// a pager, waiting on a key nobody presses, or a person reading a long result at a
// terminal has it scroll past unpaged.
func TestTextIsPagedOnlyAtATerminalThatIsNotPlain(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a line\n", 500)
	if New().Pages(long) {
		t.Error("off a terminal, long text was paged")
	}
	scr := screenOpt(true, 0)
	if New(scr, Plain()).Pages(long) {
		t.Error("with --plain, long text was paged")
	}
	// A test has no screen to measure, so Pages alone is false here whatever
	// it is asked: give the screen a height, and the plain session is still
	// not paged while the other one is.
	terminal := stream{rich: true, interactive: true}
	if pages(terminal, long, 40, true) {
		t.Error("with --plain, long text was paged on a screen 40 lines high")
	}
	if !pages(terminal, long, 40, false) {
		t.Error("long text was not paged on a screen 40 lines high, so the cases above prove nothing")
	}
	if pages(terminal, "a line\n", 40, false) {
		t.Error("a line that fits was paged")
	}
}
