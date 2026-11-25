package ui

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"
)

// fake is a table of processes by pid, read as the walk reads them.
func fake(procs map[int]proc) reader {
	return func(pid int) (proc, bool) { p, ok := procs[pid]; return p, ok }
}

// chain is the processes of a walk, the program first, each the parent of the one before; the numbers are start times,
// 0 for none, and the last has no parent.
func chain(starts ...int64) map[int]proc {
	names := []string{"program", "bash", "tmux", "fish", "login"}
	m := map[int]proc{}
	for i, s := range starts {
		p := proc{name: names[i], start: s}
		if i+1 < len(starts) {
			p.ppid = i + 2
		}
		m[i+1] = p
	}
	return m
}

// The walk up the parent chain with a fake table of processes, on every system: each system's judge ends it at
// its own sign of a reused pid, a pid that cannot be read ends it, and an equal start time passes.
//
// What it costs when this fails: on a system where a dead ancestor's pid is reused, the walk follows it into an
// unrelated tree and what it finds there, a fish or a cmd, decides the dialect of the line a person pastes.
func TestWalkChain(t *testing.T) {
	t.Parallel()
	gone := chain(100, 90, 80, 70)
	delete(gone, 3)
	unreadShell, unreadThird := chain(100, 90, 80, 70), chain(100, 90, 80, 70)
	for m, pid := range map[*map[int]proc]int{&unreadShell: 2, &unreadThird: 3} {
		p := (*m)[pid]
		p.unread = true
		(*m)[pid] = p
	}
	for _, c := range []struct {
		name    string
		procs   map[int]proc
		judge   func(reader, proc, proc) hop
		want    []string
		suspect bool
	}{
		{"linux: every parent older", chain(100, 90, 80, 70), endsWhenYounger, []string{"bash", "tmux", "fish"}, false},
		{"linux: equal times pass", chain(100, 100, 100, 100), endsWhenYounger, []string{"bash", "tmux", "fish"}, false},
		{"linux: the third is younger than the shell", chain(100, 90, 95, 70), endsWhenYounger, []string{"bash"}, false},
		{"linux: the shell is younger than the program", chain(100, 101, 80, 70), endsWhenYounger, nil, false},
		{"windows: every parent older", chain(100, 90, 80, 70), suspectWhenYounger, []string{"bash", "tmux", "fish"}, false},
		{"windows: equal times pass", chain(100, 100, 100, 100), suspectWhenYounger, []string{"bash", "tmux", "fish"}, false},
		{"windows: the third is younger, the chain is suspect", chain(100, 90, 95, 70), suspectWhenYounger, []string{"bash"}, true},
		{"windows: the shell is younger than the program", chain(1, 5, 4), suspectWhenYounger, nil, true},
		{"windows: an unread parent is suspect, no shell nearer", unreadShell, suspectWhenYounger, nil, true},
		{"windows: an unread third is suspect, the shell nearer is kept", unreadThird, suspectWhenYounger, []string{"bash"}, true},
		{"a pid that cannot be read", gone, endsWhenYounger, []string{"bash"}, false},
		{"the program cannot be read", map[int]proc{}, endsWhenYounger, nil, false},
		{"no parent", chain(1), endsWhenYounger, nil, false},
		{"macos: no time is compared", chain(1, 5, 9, 2), endsWhenReparented, []string{"bash", "tmux", "fish"}, false},
	} {
		got, suspect := walkChain(1, fake(c.procs), c.judge)
		if !slices.Equal(got, c.want) || suspect != c.suspect {
			t.Errorf("%s: chain %q suspect %v, want %q %v", c.name, got, suspect, c.want, c.suspect)
		}
	}
}

// On macOS a chain ends when a child's parent changed between the read of the child and the read of the parent.
//
// What it costs when this fails: a parent that died between the two reads, its pid reused, is followed into
// another tree and what it finds there decides the dialect.
func TestWalkChainEndsWhenReparented(t *testing.T) {
	t.Parallel()
	procs := chain(0, 0, 0, 0)
	reads := 0
	read := func(pid int) (proc, bool) {
		p := procs[pid]
		if pid == 1 {
			// The program's parent changes between the read of the program and the read of its parent.
			if reads++; reads > 1 {
				p.ppid = 9
			}
		}
		return p, true
	}
	got, suspect := walkChain(1, read, endsWhenReparented)
	if len(got) != 0 || suspect {
		t.Errorf("chain %q suspect %v, want none: the program's parent changed between reads", got, suspect)
	}
	if got, _ := walkChain(1, fake(procs), endsWhenReparented); !slices.Equal(got, []string{"bash", "tmux", "fish"}) {
		t.Errorf("a parent that does not change: chain %q, want bash tmux fish", got)
	}
}

// A second read of the program that fails ends the chain on macOS, as a changed ppid does.
//
// What it costs when this fails: a program that cannot be read again is trusted, and its parent is followed.
func TestWalkChainSecondReadFails(t *testing.T) {
	t.Parallel()
	procs := chain(0, 0, 0, 0)
	reads := 0
	read := func(pid int) (proc, bool) {
		if pid == 1 {
			reads++
		}
		p, ok := procs[pid]
		return p, ok && (pid != 1 || reads <= 1)
	}
	if got, suspect := walkChain(1, read, endsWhenReparented); len(got) != 0 || suspect {
		t.Errorf("chain %q suspect %v, want none: the program's second read failed", got, suspect)
	}
}

// Eight ancestors at most, and a cycle of pids ends at eight.
//
// What it costs when this fails: a pid cycle makes the walk read forever.
func TestWalkChainStopsAtEight(t *testing.T) {
	t.Parallel()
	loop := func(pid int) (proc, bool) { return proc{name: "sh", ppid: pid%3 + 1, start: 5}, true }
	if got, _ := walkChain(1, loop, endsWhenYounger); len(got) != 8 {
		t.Errorf("a cycle gave %d names, want 8", len(got))
	}
}

// Where the rule decides the dialect: program, node, a younger pid, fish. On Linux the chain ends at node and
// the default POSIX answers, not fish's refusal; on Windows the chain cannot be trusted and no dialect answers.
// With a known shell nearer than the younger hop, the shell decides, and the shell's dialect is the answer.
//
// What it costs when this fails: a fish behind a reused pid decides the dialect on Linux, or a cmd behind a
// reused pid or a clock set back is quoted for as PowerShell, whose single quotes cmd does not read.
func TestWalkChainDecidesTheDialect(t *testing.T) {
	t.Parallel()
	names := func(s ...string) map[int]proc {
		m := map[int]proc{}
		starts := []int64{100, 90, 95, 70}
		for i, n := range s {
			m[i+1] = proc{name: n, start: starts[i]}
			if i+1 < len(s) {
				m[i+1] = proc{name: n, ppid: i + 2, start: starts[i]}
			}
		}
		return m
	}
	linux, _ := walkChain(1, fake(names("program", "node", "x", "fish")), endsWhenYounger)
	if d, err := dialectOfChainOr(linux, shell.POSIX, false); d != shell.POSIX || err != nil {
		t.Errorf("linux chain %q: %v, %v; want posix, nil (the default, not fish's refusal)", linux, d, err)
	}
	win, suspect := walkChain(1, fake(names("program", "node", "x", "fish")), suspectWhenYounger)
	if d, err := dialectOfWalk(win, suspect); d != 0 || !errors.Is(err, ErrNoDialect) {
		t.Errorf("windows chain %q suspect %v: %v, %v; want 0 and ErrNoDialect", win, suspect, d, err)
	}
	// A known shell nearer than the younger hop still decides.
	cmd, suspect := walkChain(1, fake(names("program", "cmd.exe", "x", "fish")), suspectWhenYounger)
	if d, err := dialectOfWalk(cmd, suspect); d != shell.Cmd || err != nil {
		t.Errorf("windows chain %q suspect %v: %v, %v; want cmd, nil", cmd, suspect, d, err)
	}
	// program 1s, cmd 5s, explorer 4s: the shell is younger than the program, no shell found before it.
	young := map[int]proc{1: {name: "program.exe", ppid: 2, start: 1}, 2: {name: "cmd.exe", ppid: 3, start: 5}, 3: {name: "explorer.exe", start: 4}}
	if c, suspect := walkChain(1, fake(young), suspectWhenYounger); true {
		if d, err := dialectOfWalk(c, suspect); d != 0 || !errors.Is(err, ErrNoDialect) {
			t.Errorf("program 1s, cmd 5s, explorer 4s: %q %v: %v, %v; want ErrNoDialect", c, suspect, d, err)
		}
	}
	// A trusted chain with no shell is the default, PowerShell.
	if d, err := dialectOfWalk([]string{"explorer.exe"}, false); d != shell.PowerShell || err != nil {
		t.Errorf("a trusted chain with no shell: %v, %v; want powershell, nil", d, err)
	}
}

// grouped is the processes of a walk, the program first, each the parent of the one before, as "name:pgid".
func grouped(t *testing.T, procs ...string) map[int]proc {
	t.Helper()
	m := map[int]proc{}
	for i, p := range procs {
		name, group, _ := strings.Cut(p, ":")
		pgid, err := strconv.Atoi(group)
		if err != nil {
			t.Fatalf("%q: %v", p, err)
		}
		m[i+1] = proc{name: name, pgid: pgid, ppid: i + 2}
	}
	last := m[len(procs)]
	last.ppid = 0
	m[len(procs)] = last
	return m
}

// Off Windows an ancestor in the program's own process group is passed over only when its name answers POSIX: read
// through the walk of Linux and of macOS with a fake table that carries groups, and chosen by dialectOfGroups. A
// name that answers PowerShell or ErrNoDialect decides wherever it sits, a POSIX name in another group answers
// POSIX, a group of 0 is never the program's own, so every answer is the one the names alone give, and a passed-over
// ancestor is one of the 8.
//
// What it costs when this fails: a wrapper script's sh in front of fish or csh answers POSIX, and a backslash or a
// ! in the pasted line reaches the person's shell as code; or a pwsh or a fish that shares the program's group is
// passed over, and a refusal or a PowerShell answer becomes POSIX and nil.
func TestDialectOfGroups(t *testing.T) {
	t.Parallel()
	const refused = shell.Dialect(0)
	eight := []string{"program:7", "sh:7", "sh:7", "sh:7", "sh:7", "sh:7", "sh:7", "sh:7", "sh:7", "fish:3"}
	rows := []struct {
		name  string
		procs []string
		want  shell.Dialect
		names string
	}{
		{"sh own-group in front of fish", []string{"program:7", "sh:7", "fish:3"}, refused, "fish"},
		{"sh own-group in front of pwsh", []string{"program:7", "sh:7", "pwsh:3"}, shell.PowerShell, ""},
		{"sh own-group in front of pwsh-preview", []string{"program:7", "sh:7", "pwsh-preview:3"}, shell.PowerShell, ""},
		{"sh own-group in front of zsh in another group", []string{"program:7", "sh:7", "zsh:3"}, shell.POSIX, ""},
		{"sh own-group and nothing behind", []string{"program:7", "sh:7"}, shell.POSIX, ""},
		{"sh own-group, an unknown name, then csh", []string{"program:7", "sh:7", "node:7", "csh:3"}, refused, "csh"},
		{"sh in another group in front of fish", []string{"program:7", "sh:4", "fish:3"}, shell.POSIX, ""},
		{"sh in a group shared with a node in front of it, not the program's, in front of fish in a third", []string{"program:7", "node:3", "sh:3", "fish:5"}, shell.POSIX, ""},
		{"fish own-group in front of bash", []string{"program:7", "fish:7", "bash:3"}, refused, "fish"},
		{"pwsh own-group", []string{"program:7", "pwsh:7"}, shell.PowerShell, ""},
		{"pwsh own-group in front of bash", []string{"program:7", "pwsh:7", "bash:3"}, shell.PowerShell, ""},
		{"pwsh-lts own-group in front of fish", []string{"program:7", "pwsh-lts:7", "fish:3"}, shell.PowerShell, ""},
		{"cmd own-group is no shell here, then sh own-group, then tcsh", []string{"program:7", "cmd:7", "sh:7", "tcsh:7"}, refused, "tcsh"},
		{"eight own-group sh, then fish: the 8 still bound the walk", eight, shell.POSIX, ""},
		{"seven own-group sh, then fish", eight[1:], refused, "fish"},
	}
	for _, n := range []string{"bash", "sh", "zsh", "dash", "ash", "ksh", "ksh93", "mksh", "busybox", "busybox-1.37", "BASH", "sh.exe"} {
		rows = append(rows, struct {
			name  string
			procs []string
			want  shell.Dialect
			names string
		}{n + " own-group in front of elvish", []string{"program:7", n + ":7", "elvish:3"}, refused, "elvish"})
	}
	judges := map[string]func(reader, proc, proc) hop{"linux": endsWhenYounger, "macos": endsWhenReparented}
	for _, c := range rows {
		for system, judge := range judges {
			chain, own, _ := walkGroups(1, fake(grouped(t, c.procs...)), judge)
			got, err := dialectOfGroups(chain, own)
			if got != c.want || (c.want == refused) != errors.Is(err, ErrNoDialect) || (c.want != refused && err != nil) || (err != nil && !strings.Contains(err.Error(), c.names)) {
				t.Errorf("%s: %s: %d, %v, want %d and an error only for a refusal, naming %q", system, c.name, got, err, c.want, c.names)
			}
			// With no group read, on every entry, the answer is the one the names alone give, row for row.
			zeroed := make([]string, len(c.procs))
			for i, p := range c.procs {
				name, _, _ := strings.Cut(p, ":")
				zeroed[i] = name + ":0"
			}
			chain, own, _ = walkGroups(1, fake(grouped(t, zeroed...)), judge)
			got, err = dialectOfGroups(chain, own)
			want, wantErr := dialectOfChainOr(chain, shell.POSIX, false)
			if slices.Contains(own, true) || got != want || (err == nil) != (wantErr == nil) {
				t.Errorf("%s: %s with no group read: own %v, %d, %v, want none own and %d, %v", system, c.name, own, got, err, want, wantErr)
			}
		}
	}
	// Only the program's group could not be read: nothing is own-group.
	chain, own, _ := walkGroups(1, fake(grouped(t, "program:0", "sh:7", "fish:7")), endsWhenYounger)
	if got, err := dialectOfGroups(chain, own); got != shell.POSIX || err != nil || slices.Contains(own, true) {
		t.Errorf("the program's group is 0: own %v, %d, %v, want none own and POSIX, nil", own, got, err)
	}
	// Windows's walk reads no group and its chooser passes nothing over.
	if chain, _ := walkChain(1, fake(grouped(t, "program.exe:7", "sh.exe:7", "fish.exe:3")), suspectWhenYounger); !slices.Equal(chain, []string{"sh.exe", "fish.exe"}) {
		t.Errorf("windows: chain %q, want sh.exe fish.exe", chain)
	} else if got, err := dialectOfWalk(chain, false); got != shell.POSIX || err != nil {
		t.Errorf("windows: sh.exe in front of fish.exe: %d, %v, want POSIX, nil", got, err)
	}
}
