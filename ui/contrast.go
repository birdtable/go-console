package ui

import (
	"image/color"

	"github.com/birdtable/go-console/measure"
)

// RelativeLuminance is measure.RelativeLuminance: the WCAG 2.1 relative luminance of c, 0 for black to 1 for white.
func RelativeLuminance(c color.Color) float64 { return measure.RelativeLuminance(c) }

// ContrastRatio is measure.ContrastRatio: the WCAG 2.1 contrast ratio of a and b, 1 to 21, with no threshold.
func ContrastRatio(a, b color.Color) float64 { return measure.ContrastRatio(a, b) }
