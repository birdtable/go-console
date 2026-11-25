package ui

import (
	"unsafe"

	"github.com/birdtable/go-console/shell"
	"golang.org/x/sys/windows"
)

// hereDialect reads the names of the parent chain, nearest first, up to 8 levels, and lets dialectOfChain choose.
// A failed snapshot is an empty chain, and so is a pid the snapshot does not name; a parent younger than its child, or
// one the snapshot names and OpenProcess refuses, ends the chain as one that cannot be trusted (dialectOfWalk).
func hereDialect() (shell.Dialect, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return dialectOfChain(nil)
	}
	defer windows.CloseHandle(snap) //nolint:errcheck // nothing to do about a failed close
	parent := map[int]int{}
	image := map[int]string{}
	e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		parent[int(e.ProcessID)] = int(e.ParentProcessID)
		image[int(e.ProcessID)] = windows.UTF16ToString(e.ExeFile[:])
	}
	return dialectOfParents(int(windows.GetCurrentProcessId()), func(pid int) (proc, bool) { return snapshotProc(parent, image, pid) })
}

// snapshotProc is one process of the snapshot: not ok when the snapshot does not name pid, unread when it does and
// the creation time cannot be read.
func snapshotProc(parent map[int]int, image map[int]string, pid int) (proc, bool) {
	ppid, ok := parent[pid]
	if !ok {
		return proc{}, false
	}
	start, got := creationTime(pid)
	return proc{name: image[pid], ppid: ppid, start: start, unread: !got}, true
}

// dialectOfParents is the walk Windows wires: a parent younger than its child, or one the snapshot names and
// OpenProcess refuses, cannot be trusted (suspectWhenYounger), and dialectOfWalk answers ErrNoDialect for it unless a
// known shell was found nearer.
func dialectOfParents(self int, read reader) (shell.Dialect, error) {
	chain, suspect := walkChain(self, read, suspectWhenYounger)
	return dialectOfWalk(chain, suspect)
}

// creationTime is when the process pid was created, as a FILETIME, through GetProcessTimes on a handle opened with
// PROCESS_QUERY_LIMITED_INFORMATION. A process that cannot be opened, another user's for example, has none.
func creationTime(pid int) (int64, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)) //nolint:gosec // a pid here came from a uint32
	if err != nil {
		return 0, false
	}
	defer windows.CloseHandle(h) //nolint:errcheck // nothing to do about a failed close
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(h, &created, &exited, &kernel, &user) != nil {
		return 0, false
	}
	return int64(created.HighDateTime)<<32 | int64(created.LowDateTime), true
}
