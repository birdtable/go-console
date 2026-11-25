//go:build darwin

package ui

import (
	"slices"
	"testing"
)

// The chain of a live process on macOS is not empty and names no blank.
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

// macOS wires the judge that ends the chain when a child's parent changed between the reads, and compares no time.
//
// What it costs when this fails: macOS takes another system's judge, so a parent that died between the two reads is
// followed into another tree, or a clock step ends a chain of live parents.
func TestWalkParents(t *testing.T) {
	t.Parallel()
	// A younger time with an unchanged ppid passes: the shell reads as younger than the program.
	if got, _ := walkParents(1, fake(chain(1, 5, 4, 3))); !slices.Equal(got, []string{"bash", "tmux", "fish"}) {
		t.Errorf("a younger time, an unchanged ppid: chain %q, want bash tmux fish", got)
	}
	// A ppid that changes between the reads ends the chain.
	procs := chain(0, 0, 0, 0)
	reads := 0
	changing := func(pid int) (proc, bool) {
		p := procs[pid]
		if pid == 1 {
			if reads++; reads > 1 {
				p.ppid = 9
			}
		}
		return p, true
	}
	if got, _ := walkParents(1, changing); len(got) != 0 {
		t.Errorf("a changed ppid: chain %q, want none", got)
	}
}
