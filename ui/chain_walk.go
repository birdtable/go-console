package ui

// maxChain is how many ancestors the walk names.
const maxChain = 8

// proc is what a system says of one process: its pid, its image name, its parent's pid and, where the system
// gives one, when it started, in units that compare on that system (clock ticks since boot, a FILETIME).
type proc struct {
	pid   int
	name  string
	ppid  int
	start int64
	// pgid is the process group, where the system has one and it could be read, and 0 otherwise.
	pgid int
	// unread is true when the system names the process but its start time could not be read (Windows, a process
	// that cannot be opened).
	unread bool
}

// reader reads one process, and says false when it cannot.
type reader func(pid int) (proc, bool)

// hop is what a judge says of a child and the parent read for it: the parent is taken (hopTaken), the chain ends
// before it (hopEnd), or ends before it and cannot be trusted (hopSuspect).
type hop int

const (
	hopTaken hop = iota
	hopEnd
	hopSuspect
)

// walkChain is walkGroups for a caller that reads no process group: Windows, which has none.
func walkChain(self int, read reader, judge func(read reader, child, parent proc) hop) (chain []string, suspect bool) {
	chain, _, suspect = walkGroups(self, read, judge)
	return chain, suspect
}

// walkGroups is the names of the ancestors of the process self, nearest first, up to 8, read by read and judged
// hop by hop by judge. A pid that read cannot read ends the chain, and so does a parent the judge does not take.
// own says of each ancestor whether it is in the group of self: its pgid is greater than 0 and equals the one read
// for self, so a group that could not be read is never own. suspect is true when the judge said so: the chain
// ended at a hop that cannot be trusted.
func walkGroups(self int, read reader, judge func(read reader, child, parent proc) hop) (chain []string, own []bool, suspect bool) {
	child, ok := read(self)
	child.pid = self
	group := child.pgid
	for ok && child.ppid > 0 && len(chain) < maxChain {
		parent, ok := read(child.ppid)
		parent.pid = child.ppid
		if !ok {
			break
		}
		if h := judge(read, child, parent); h != hopTaken {
			return chain, own, h == hopSuspect
		}
		chain = append(chain, parent.name)
		own = append(own, group > 0 && parent.pgid == group)
		child = parent
	}
	return chain, own, false
}

// endsWhenYounger is Linux's judge: a parent that started later than its child cannot be its parent, so its pid
// was reused, and the chain ends. Start times count from boot there, which no clock step moves; equal times pass.
func endsWhenYounger(_ reader, child, parent proc) hop {
	if parent.start > child.start {
		return hopEnd
	}
	return hopTaken
}

// suspectWhenYounger is Windows's judge: the creation time is the only tool there, and a parent that reads
// younger than its child is a reused pid or a clock set back, and a parent that is named but cannot be opened is
// not checked, so the chain cannot be trusted.
func suspectWhenYounger(_ reader, child, parent proc) hop {
	if child.unread || parent.unread || parent.start > child.start {
		return hopSuspect
	}
	return hopTaken
}

// endsWhenReparented is macOS's judge: the kernel gives an orphan a new parent, so a ppid names a live process,
// and the only gap is between reading the child and reading the parent. It reads the child again, and the chain
// ends when its parent changed. No time is compared.
func endsWhenReparented(read reader, child, _ proc) hop {
	if again, ok := read(child.pid); !ok || again.ppid != child.ppid {
		return hopEnd
	}
	return hopTaken
}
