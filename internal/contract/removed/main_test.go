package main

import (
	"strings"
	"testing"
)

// What it costs when this fails: a promise the last tag made is taken back and nothing says so, and a product that leaned
// on it learns at its next build.
func TestALineTakenBackSinceTheLastTagIsRedUnlessListed(t *testing.T) {
	t.Parallel()
	const a, b = "ui\tTestA\tall\tA.", "ui\tTestB\tall\tB."
	for _, c := range []struct {
		name               string
		old, cur, declared string
		want               string
	}{
		{"nothing changed", a + "\n" + b + "\n", a + "\n" + b + "\n", "", ""},
		{"a line taken back and not listed", a + "\n" + b + "\n", a + "\n", "", b},
		{"a line taken back and listed", a + "\n" + b + "\n", a + "\n", b + "\n", ""},
		{"a line added", a + "\n", a + "\n" + b + "\n", "", ""},
		{"a line listed that is also kept", a + "\n", a + "\n", a + "\n", ""},
		{"a line reworded counts as taken back", a + "\n", "ui\tTestA\tall\tChanged.\n", "", a},
		{"no index at the base", "", a + "\n", "", ""},
	} {
		got := strings.Join(lost(c.old, c.cur, c.declared), "\n")
		if got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
