//go:build release

package ui

import "testing"

// A child that exits at once is read whole in 50 runs in a row, the stress a push runs 5 times of.
// It runs at `task release`, with -tags release.
//
// What it costs when this fails: the same as TestAChildThatExitsAtOnceIsReadWhole, on a rarer run.
func TestAChildThatExitsAtOnceIsReadWholeFifty(t *testing.T) {
	t.Parallel()
	childExitsAtOnce(t, 50)
}
