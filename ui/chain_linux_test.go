//go:build linux

package ui

import (
	"slices"
	"testing"
)

// The chain of a live process on Linux is not empty and names no blank.
//
// What it costs when this fails: HereDialect reads an empty chain under every shell and answers POSIX under fish.
func TestParentChainLive(t *testing.T) {
	t.Parallel()
	chain, own := parentChain()
	if len(chain) == 0 || len(chain) > 8 || len(own) != len(chain) {
		t.Fatalf("chain %q, own group %v, want 1 to 8 names and as many groups", chain, own)
	}
	for _, n := range chain {
		if n == "" {
			t.Errorf("chain %q holds a blank name", chain)
		}
	}
}

// Linux wires the judge that ends the chain at a younger parent, whatever the other judges would say.
//
// What it costs when this fails: Linux takes another system's judge, and a reused pid is followed into another tree,
// or a younger parent is trusted, so the line is quoted for the wrong shell.
func TestWalkParents(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		procs map[int]proc
		want  []string
	}{
		{"a younger parent ends the chain", chain(100, 90, 95, 70), []string{"bash"}},
		{"equal times pass", chain(100, 100, 100, 100), []string{"bash", "tmux", "fish"}},
	} {
		if got, _ := walkParents(1, fake(c.procs)); !slices.Equal(got, c.want) {
			t.Errorf("%s: chain %q, want %q", c.name, got, c.want)
		}
	}
}
