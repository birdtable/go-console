//go:build darwin

package ui

import (
	"os"

	"golang.org/x/sys/unix"
)

// parentChain reads the names of the parent chain, nearest first, up to 8 levels, through the kern.proc.pid
// sysctl: P_comm, Eproc.Ppid and Eproc.Pgid, which says whether each is in the program's own process group, and ends the chain when a child's parent changed between the two reads (endsWhenReparented).
// A read that fails ends the chain; it is never an error.
func parentChain() ([]string, []bool) {
	return walkParents(os.Getpid(), func(pid int) (proc, bool) {
		kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
		if err != nil {
			return proc{}, false
		}
		return proc{name: unix.ByteSliceToString(kp.Proc.P_comm[:]), ppid: int(kp.Eproc.Ppid), pgid: int(kp.Eproc.Pgid)}, true
	})
}

// walkParents is the walk macOS wires: it ends when a child's parent changed between the reads (endsWhenReparented).
func walkParents(self int, read reader) ([]string, []bool) {
	chain, own, _ := walkGroups(self, read, endsWhenReparented)
	return chain, own
}
