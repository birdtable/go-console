package measure

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestColumnPadsByDisplayCellsNotBytes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, in string
		want     int // cells the result should occupy
	}{
		{"ascii", "file", 10},
		// One more than it occupies: a column separates two things, and a label
		// that fills or overflows it has lost the alignment but not the gap.
		// Without this, `this sweep` in a column of ten printed `this sweep500`.
		{"already wider than the column", "quarantine!!", 13},
		{"multi-byte, one cell per rune", "café", 10},
		{"a wide rune fills two cells", "日本", 10},
		{"an escape sequence fills none", "\x1b[31mred\x1b[0m", 10},
	} {
		got := Column(tc.in, 10)
		if w := ansi.StringWidth(got); w != tc.want {
			t.Errorf("%s: Column(%q, 10) occupies %d cells, want %d", tc.name, tc.in, w, tc.want)
		}
	}
}

// A column separates two things. When the label filled the column exactly there
// was no padding left and the value ran into it, so `uploads reconcile` printed
// `this sweep500`, which reads as a word nobody wrote.
func TestAColumnAlwaysLeavesAGapAfterItsLabel(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		label string
		width int
		want  string
	}{
		{"uploads", 10, "uploads   "},
		{"this sweep", 10, "this sweep "},               // exactly the width
		{"a very long label", 10, "a very long label "}, // wider than it
		{"", 10, "          "},                          // a blank spacer column stays blank
		{"", 0, ""},                                     // and a blank in no column is nothing
	} {
		if got := Column(c.label, c.width); got != c.want {
			t.Errorf("Column(%q, %d) = %q, want %q", c.label, c.width, got, c.want)
		}
	}
}
