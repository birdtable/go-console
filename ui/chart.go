package ui

import (
	"math"
	"strings"
)

// eighths are a cell filled from the left, an eighth at a time: eighth n is the nth
// rune, and none is "".
const eighths = "▏▎▍▌▋▊▉"

// eighth is a cell filled n eighths from the left, 0 to 7.
func eighth(n int) string {
	if n <= 0 {
		return ""
	}
	return string([]rune(eighths)[n-1])
}

// Charted reports whether a bar is drawn: at a terminal only, one a person is
// at or one that renders escapes. A pipe or a log gets the figures, and a width
// alone, as COLUMNS gives one, never turns a bar on.
func (s *Session) Charted() bool { return chartedOn(s.outStream()) }

func chartedOn(st stream) bool { return st.paints() || st.interactive }

// Meter is frac of width cells drawn as a bar, to an eighth of a cell, for a
// column of figures read at a glance, and "" where Charted says no bar is
// drawn. frac outside 0 to 1 is drawn as the nearer end.
func (s *Session) Meter(frac float64, width int) string { return meterOn(s.outStream(), frac, width) }

func meterOn(st stream, frac float64, width int) string {
	if !chartedOn(st) || width <= 0 || math.IsNaN(frac) {
		return ""
	}
	if st.ascii {
		return strings.Repeat("#", int(min(max(frac, 0), 1)*float64(width)+0.5))
	}
	n := int(min(max(frac, 0), 1)*float64(width*8) + 0.5)
	return strings.Repeat("█", n/8) + eighth(n%8)
}
