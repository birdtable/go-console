//go:build linux

package ui

import (
	"io/fs"
	"os"
	"strconv"
)

// parentChain reads the names of the parent chain, nearest first, up to 8 levels, and whether each is in the
// program's own process group, from /proc: the name, the ppid, the group and the start time all from /proc/<pid>/stat. A read that fails ends the chain; it is never an error.
func parentChain() ([]string, []bool) {
	root, err := os.OpenRoot("/proc")
	if err != nil {
		return nil, nil
	}
	defer root.Close() //nolint:errcheck // nothing to do about a failed close
	return walkParents(os.Getpid(), func(pid int) (proc, bool) {
		stat, err := fs.ReadFile(root.FS(), strconv.Itoa(pid)+"/stat")
		if err != nil {
			return proc{}, false
		}
		name, ppid, pgid, start, ok := statOf(string(stat))
		return proc{name: name, ppid: ppid, pgid: pgid, start: start}, ok
	})
}

// walkParents is the walk Linux wires: it ends at a parent that started later than its child (endsWhenYounger).
func walkParents(self int, read reader) ([]string, []bool) {
	chain, own, _ := walkGroups(self, read, endsWhenYounger)
	return chain, own
}
