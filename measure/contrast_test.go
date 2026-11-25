package measure

import (
	"image/color"
	"math"
	"strconv"
	"testing"
)

// hex is a "#rrggbb" string as an opaque colour.
func hex(t *testing.T, s string) color.Color {
	t.Helper()
	n, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil || len(s) != 7 {
		t.Fatalf("%q is not #rrggbb", s)
	}
	return color.NRGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 0xff}
}

func TestContrastRatioRows(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		a, b string
		want float64
	}{
		{"black on white", "#000000", "#ffffff", 21},
		{"a colour against itself", "#b91c1c", "#b91c1c", 1},
		{"just above 4.5", "#767676", "#ffffff", 4.54},
		{"just below 4.5", "#777777", "#ffffff", 4.48},
	} {
		got := ContrastRatio(hex(t, tc.a), hex(t, tc.b))
		if math.Abs(got-tc.want) > 0.005 {
			t.Errorf("%s: %.4f, want %.2f", tc.name, got, tc.want)
		}
		if swapped := ContrastRatio(hex(t, tc.b), hex(t, tc.a)); swapped != got {
			t.Errorf("%s: swapping the arguments gives %v, not %v", tc.name, swapped, got)
		}
	}
}

func TestRelativeLuminanceEnds(t *testing.T) {
	t.Parallel()
	if got := RelativeLuminance(color.Black); got != 0 {
		t.Errorf("black is %v, want 0", got)
	}
	if got := RelativeLuminance(color.White); math.Abs(got-1) > 1e-9 {
		t.Errorf("white is %v, want 1", got)
	}
}

func TestAlphaIsNotUsed(t *testing.T) {
	t.Parallel()
	opaque := color.NRGBA{R: 0xb9, G: 0x1c, B: 0x1c, A: 0xff}
	half := color.NRGBA{R: 0xb9, G: 0x1c, B: 0x1c, A: 0x80}
	white := color.White
	if a, b := ContrastRatio(opaque, white), ContrastRatio(half, white); a != b {
		t.Errorf("alpha 0x80 gives %v, opaque gives %v", b, a)
	}
	if a, b := RelativeLuminance(opaque), RelativeLuminance(half); a != b {
		t.Errorf("luminance with alpha 0x80 is %v, opaque is %v", b, a)
	}
}
