package ui

// The palette's one testable claim: every role is readable on the ground it is
// painted against. The figures in style.go were written out before anything
// computed them, and three of the four were wrong.

import (
	"math"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/birdtable/go-console/measure"
)

// contrast reads two hex strings through the exported ratio, as a product with
// a hex string would: lipgloss.Color is a color.Color.
func contrast(a, b string) float64 {
	return ContrastRatio(lipgloss.Color(a), lipgloss.Color(b))
}

// 4.5:1 is the readable threshold for body text. Below it the amber "warn" row
// is the first thing to go, and that row is the one a person is meant to act on.
const readable = 4.5

func TestEveryRoleIsReadableOnBothGrounds(t *testing.T) {
	t.Parallel()
	checked := 0
	for role, p := range hexes() {
		for _, tc := range []struct {
			name, fg, bg string
		}{
			{role + " on the light ground", p.light, lightGround},
			{role + " on the dark ground", p.dark, darkGround},
		} {
			checked++
			if got := contrast(tc.fg, tc.bg); got < readable {
				t.Errorf("%s: %s on %s is %.2f:1, below the %.1f:1 that is readable",
					tc.name, tc.fg, tc.bg, got, readable)
			}
		}
	}
	if checked != 2*len(hexes()) {
		t.Fatalf("checked %d combinations for %d roles, so this proves nothing", checked, len(hexes()))
	}
}

// The figures written into style.go are what this computes. A drifting comment
// beside a passing test is how the last set came to be wrong.
func TestTheContrastTableInStyleGoIsCorrect(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		role, ground string
		want         float64
	}{
		{"bad", "light", 6.47}, {"bad", "dark", 6.03},
		{"attn", "light", 4.92}, {"attn", "dark", 9.99},
		{"good", "light", 5.02}, {"good", "dark", 9.57},
		{"action", "light", 5.36}, {"action", "dark", 9.22},
	} {
		p := hexes()[tc.role]
		fg, bg := p.light, lightGround
		if tc.ground == "dark" {
			fg, bg = p.dark, darkGround
		}
		if got := contrast(fg, bg); math.Abs(got-tc.want) > 0.005 {
			t.Errorf("%s on the %s ground is %.2f:1, and style.go says %.2f:1",
				tc.role, tc.ground, got, tc.want)
		}
	}
}

// The pair being readable is not the same claim as the one that gets painted
// being readable, and that gap is where this went wrong: `colourFor` passed the
// constant `false` to lipgloss.LightDark, so the light-ground half of every pair
// was the only half ever rendered. On the dark ground that is 2.58:1 for `bad`,
// against a threshold of 4.5, while the test above went on passing because it
// reads the table rather than the choice.
func TestTheColourActuallyChosenIsReadableOnTheGroundItIsChosenFor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		dark   bool
		ground string
	}{
		{"a dark terminal", true, darkGround},
		{"a light terminal", false, lightGround},
	} {
		for role, p := range hexes() {
			chosen := p.light
			if tc.dark {
				chosen = p.dark
			}
			if got := contrast(chosen, tc.ground); got < readable {
				t.Errorf("on %s, %s paints %s on %s, which is %.2f:1",
					tc.name, role, chosen, tc.ground, got)
			}
		}
	}
	// And the half drawn is the one the Terminal says: the dark half on a dark
	// ground, the light one on a light.
	if colourIn(Contrast(), nameBad, true) != lipgloss.Color(hexes()[nameBad].dark) || colourIn(Contrast(), nameBad, false) != lipgloss.Color(hexes()[nameBad].light) {
		t.Error("Contrast does not draw the half for the ground the Terminal says")
	}
}

// ui keeps the names the measures had before they moved to measure, and they
// answer as measure does: a colour, a pair and a label are each measured once.
func TestTheMeasuresKeepTheirNamesInUI(t *testing.T) {
	t.Parallel()
	a, b := lipgloss.Color("#767676"), lipgloss.Color("#ffffff")
	if got, want := ContrastRatio(a, b), measure.ContrastRatio(a, b); got != want {
		t.Errorf("ContrastRatio is %v, measure's is %v", got, want)
	}
	if got, want := RelativeLuminance(a), measure.RelativeLuminance(a); got != want {
		t.Errorf("RelativeLuminance is %v, measure's is %v", got, want)
	}
	if got, want := Column("this sweep", 10), measure.Column("this sweep", 10); got != want || got != "this sweep " {
		t.Errorf("Column is %q, measure's is %q", got, want)
	}
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
		got := contrast(tc.a, tc.b)
		if math.Abs(got-tc.want) > 0.005 {
			t.Errorf("%s: %.4f, want %.2f", tc.name, got, tc.want)
		}
		if swapped := contrast(tc.b, tc.a); swapped != got {
			t.Errorf("%s: swapping the arguments gives %v, not %v", tc.name, swapped, got)
		}
	}
}
