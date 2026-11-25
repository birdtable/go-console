// Package measure holds the pure measures of text and colour: how many cells a
// string fills and the column it is padded to, and the WCAG 2.1 relative
// luminance of a colour and the contrast ratio of two. It is built on the
// standard library and github.com/charmbracelet/x/ansi (StringWidth), adds the
// column with its gap and the two WCAG formulas, and leaves every threshold, width
// and colour to the caller. It does no I/O: it reads no environment, file or
// terminal, at init or at any other time, and imports nothing of Charm but ansi,
// so a layer that must stay pure takes it without taking ui.
package measure

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Column pads s to n display cells, where a cell is what ansi.StringWidth
// counts: not a byte and not a rune, so a wide rune fills two and an escape
// sequence none.
//
// A label that fills its column still gets one space after it. Without it a
// label exactly n wide ran into its own value and printed `this sweep500`,
// which reads as a word nobody wrote. A label wider than the column has already
// lost the alignment; it has not lost the space between two things. An empty s
// is padded to n and no further, so a blank spacer column stays blank.
func Column(s string, n int) string {
	if pad := n - ansi.StringWidth(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	if s == "" {
		return s
	}
	return s + " "
}
