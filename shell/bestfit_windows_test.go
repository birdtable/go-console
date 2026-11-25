package shell

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-console/internal/bestfit"
)

// TestTheBestFitTableIsWhatWindowsConvertsToday holds the checked-in table to the Windows the tests run on: it runs
// the scan tools/bestfit writes the table from and fails when the two differ, naming the runes only one side holds.
// A Windows update that changes a best-fit mapping turns the release-tier Windows run red here; the fix is to run
// go run ./tools/bestfit and read what changed.
func TestTheBestFitTableIsWhatWindowsConvertsToday(t *testing.T) {
	t.Parallel()
	scan, err := bestfit.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(scan) == 0 || len(bestFitRunes) == 0 {
		t.Fatalf("the scan holds %d runes and the table %d; neither may be empty", len(scan), len(bestFitRunes))
	}
	if !slices.IsSorted(bestFitRunes) || len(slices.Compact(slices.Clone(bestFitRunes))) != len(bestFitRunes) {
		t.Errorf("the table is not ascending with each rune once: %s", runeNames(bestFitRunes))
	}
	onlyTable, onlyScan := missingFrom(bestFitRunes, scan), missingFrom(scan, bestFitRunes)
	if len(onlyTable) > 0 || len(onlyScan) > 0 {
		t.Errorf("the table and this Windows differ: only in the table [%s], only in the scan [%s]; run go run ./tools/bestfit and read the change",
			runeNames(onlyTable), runeNames(onlyScan))
	}
	// The two the rule was written for: without them the table is not the one the refusal needs.
	for _, must := range []rune{0xFF02, 0xFF3C} {
		if !slices.Contains(bestFitRunes, must) {
			t.Errorf("the table does not hold U+%04X", must)
		}
	}
}

// missingFrom is the runes of a that b does not hold, in a's order.
func missingFrom(a, b []rune) []rune {
	var out []rune
	for _, r := range a {
		if !slices.Contains(b, r) {
			out = append(out, r)
		}
	}
	return out
}

// runeNames is runes as U+XXXX, never the runes themselves: some of them draw as a quote or as nothing.
func runeNames(runes []rune) string {
	names := make([]string, len(runes))
	for i, r := range runes {
		names[i] = fmt.Sprintf("U+%04X", r)
	}
	return strings.Join(names, " ")
}
