package ui

import (
	"strconv"
	"strings"
	"testing"
)

// stat is a /proc stat line for the name comm, the parent ppid, the group pgid and the start time: field 5 is the
// group and field 22 the start time. The session, field 6, is another number, so a read of the wrong field shows.
func stat(comm string, ppid, pgid, start int) string {
	return "42 (" + comm + ") S " + strconv.Itoa(ppid) + " " + strconv.Itoa(pgid) + " 77" + strings.Repeat(" 0", 15) + " " + strconv.Itoa(start) + " 1 2"
}

// The name, the parent pid, the process group and the start time out of a /proc stat line, whatever the name holds:
// the group is the third field after the last closing parenthesis, a short line is not ok, and a group that is no
// number is 0, which is never the program's own group.
//
// What it costs when this fails: a shell named with a space or a parenthesis ends the chain early or reads the
// wrong ancestor, a reused pid is followed into another tree, or another field is taken for the group and a shell is
// passed over or kept by chance, so the line is quoted for the wrong shell.
func TestStatOf(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, stat string
		comm       string
		ppid, pgid int
		start      int64
		ok         bool
	}{
		{"plain", stat("bash", 7, 31, 900), "bash", 7, 31, 900, true},
		{"space in the name", stat("my shell", 9, 32, 12), "my shell", 9, 32, 12, true},
		{"parenthesis in the name", stat("a) b", 11, 33, 5), "a) b", 11, 33, 5, true},
		{"spaces and parentheses in the name", stat("a (b) c) 9 8 7", 11, 34, 5), "a (b) c) 9 8 7", 11, 34, 5, true},
		{"opening parenthesis in the name", stat("(a (b", 11, 35, 5), "(a (b", 11, 35, 5, true},
		{"no parenthesis", "42 bash S 7", "", 0, 0, 0, false},
		{"truncated before the start time", "42 (bash) S 7 42 42 0 -1", "", 0, 0, 0, false},
		{"truncated before the group", "42 (bash) S 7", "", 0, 0, 0, false},
		{"group not a number", "42 (bash) S 7 x" + strings.Repeat(" 0", 16) + " 5", "bash", 7, 0, 5, true},
		{"ppid not a number", "42 (bash) S x" + strings.Repeat(" 0", 17) + " 5", "", 0, 0, 0, false},
		{"start not a number", "42 (bash) S 7" + strings.Repeat(" 0", 17) + " x", "", 0, 0, 0, false},
	} {
		comm, ppid, pgid, start, ok := statOf(c.stat)
		if comm != c.comm || ppid != c.ppid || pgid != c.pgid || start != c.start || ok != c.ok {
			t.Errorf("%s: %q, %d, %d, %d, %v; want %q, %d, %d, %d, %v", c.name, comm, ppid, pgid, start, ok, c.comm, c.ppid, c.pgid, c.start, c.ok)
		}
	}
}
