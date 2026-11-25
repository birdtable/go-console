package measure

import (
	"image/color"
	"math"
)

// RelativeLuminance is the WCAG 2.1 relative luminance of c: each of R, G and
// B as 8-bit sRGB over 255, linearised with the standard's own constants, then
// weighted 0.2126, 0.7152 and 0.0722. The answer runs from 0 for black to 1 for
// white. A colour is read as opaque: its alpha is not used.
func RelativeLuminance(c color.Color) float64 {
	n, ok := color.NRGBAModel.Convert(c).(color.NRGBA)
	if !ok {
		return 0
	}
	channel := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(n.R) + 0.7152*channel(n.G) + 0.0722*channel(n.B)
}

// ContrastRatio is the WCAG 2.1 contrast ratio of a and b, (L1+0.05)/(L2+0.05)
// with the lighter colour first, so the order of the arguments does not matter.
// The answer runs from 1 to 21. It holds no threshold: 4.5, 3 and 7 are the
// caller's to choose.
func ContrastRatio(a, b color.Color) float64 {
	x, y := RelativeLuminance(a), RelativeLuminance(b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}
