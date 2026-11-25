package ui

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"charm.land/huh/v2"

	"github.com/birdtable/go-console/uitest"
)

// wrappingChoices are n rows whose blurbs are longer than a 40 column screen,
// each ending in a marker word that is on screen only if the row is.
func wrappingChoices(n int) []Choice {
	cs := make([]Choice, n)
	for i := range cs {
		cs[i] = Choice{
			Label: fmt.Sprintf("row%d", i),
			Blurb: fmt.Sprintf("a blurb long enough to wrap on a narrow screen and take three lines or more before it ends: end%d", i),
			Value: fmt.Sprintf("v%d", i),
			Gap:   i%3 == 2,
		}
	}
	return cs
}

type listCase struct {
	name string
	form func(cs []Choice) *huh.Form
	// wraps is whether huh wraps a row's blurb onto the lines below. A multi
	// select does not: huh v2 clips a long blurb to the first lines of the row,
	// so only the row's name is held to be in view there.
	wraps bool
}

func listCases() []listCase {
	return []listCase{
		{"Pick", func(cs []Choice) *huh.Form {
			var got string
			return huh.NewForm(huh.NewGroup(pickField(stream{}, houseMessages(), "Which?", cs, "--k", &got)))
		}, true},
	}
}

// inView is what shows row i: its blurb's last word where huh wraps the blurb,
// else its name.
func (c listCase) inView(i int) string {
	if c.wraps {
		return fmt.Sprintf("end%d", i)
	}
	return fmt.Sprintf("row%d", i)
}

func TestEveryRowOfAMenuIsInViewWhateverTheWidth(t *testing.T) {
	t.Parallel()
	var ran atomic.Int32
	t.Cleanup(func() { // after the parallel subtests
		if n := ran.Load(); n != 3 {
			t.Errorf("the property ran %d times, not 3", n)
		}
	})
	for _, c := range listCases() {
		for _, w := range []int{40, 60, 80} {
			t.Run(fmt.Sprintf("%s at %d", c.name, w), func(t *testing.T) {
				t.Parallel()
				cs := wrappingChoices(5)
				frame := uitest.Run(uitest.FormScreen(c.form(cs)), w, 80).Frame
				for i := range cs {
					if !strings.Contains(frame, c.inView(i)) {
						t.Errorf("row %d is out of view at %d columns:\n%s", i, w, frame)
					}
				}
				ran.Add(1)
			})
		}
	}
}

func TestALongMenuScrollsAtTheCapAndReachesItsLastRow(t *testing.T) {
	t.Parallel()
	const n = 30
	var ran atomic.Int32
	t.Cleanup(func() { // after the parallel subtests
		if n := ran.Load(); n != 3 {
			t.Errorf("the property ran %d times, not 3", n)
		}
	})
	for _, c := range listCases() {
		for _, w := range []int{40, 60, 80} {
			t.Run(fmt.Sprintf("%s at %d", c.name, w), func(t *testing.T) {
				t.Parallel()
				s := uitest.FormScreen(c.form(wrappingChoices(n)))
				first := uitest.Run(s, w, 80).Frame
				// Not the multi select: huh does not wrap its rows, so the frame
				// wraps them after the viewport has counted them, and the lines
				// on screen run past what it was given.
				if lines := strings.Count(first, "\n") + 1; c.wraps && lines > maxListHeight+4 {
					t.Errorf("a menu of %d rows takes %d lines, past its cap of %d:\n%s", n, lines, maxListHeight, first)
				}
				if strings.Contains(first, c.inView(n-1)) {
					t.Errorf("the last of %d rows is in view at once, so the cap cut nothing:\n%s", n, first)
				}
				keys := make([]string, n-1)
				for i := range keys {
					keys[i] = "down"
				}
				last := uitest.Run(s, w, 80, keys...).Frame
				if !strings.Contains(last, c.inView(n-1)) {
					t.Errorf("moving to the last row does not bring it into view at %d columns:\n%s", w, last)
				}
				ran.Add(1)
			})
		}
	}
}

// Before a width arrives, a menu of more rows than the cap already takes no more than the cap, and
// its last row is not in view: the cap is held by the height the field is given, and not only by the
// height it is given again at a width.
//
// What it costs when this fails: a menu drawn before the terminal's size is known fills the screen
// and pushes its title off the top.
func TestALongMenuIsCappedBeforeAWidthArrives(t *testing.T) {
	t.Parallel()
	const n = 30
	var got string
	f := pickField(stream{}, houseMessages(), "Which?", wrappingChoices(n), "--k", &got)
	f.Focus()
	view := f.View()
	if lines := strings.Count(view, "\n") + 1; lines > maxListHeight {
		t.Errorf("a menu of %d rows takes %d lines before a width, past its cap of %d:\n%s", n, lines, maxListHeight, view)
	}
	if strings.Contains(view, fmt.Sprintf("end%d", n-1)) {
		t.Errorf("the last of %d rows is in view before a width, so the cap cut nothing:\n%s", n, view)
	}
}
