package ui

import (
	"strconv"
	"strings"
)

// statOf is the name, the parent pid, the process group (field 5) and the start time (field 22, in clock ticks
// since boot) in a /proc/<pid>/stat line, "pid (comm) state ppid pgrp ...". A group that is no number is 0. The name is what lies between the first opening parenthesis and the last
// closing one, and may hold spaces and parentheses, so the fields are counted from the last closing parenthesis.
func statOf(stat string) (name string, ppid, pgid int, start int64, ok bool) {
	before, after, found := strings.CutLast(stat, ")")
	if !found {
		return "", 0, 0, 0, false
	}
	_, name, found = strings.Cut(before, "(")
	if !found {
		return "", 0, 0, 0, false
	}
	fields := strings.Fields(after)
	if len(fields) < 20 {
		return "", 0, 0, 0, false
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return "", 0, 0, 0, false
	}
	pgid, _ = strconv.Atoi(fields[2]) //nolint:errcheck // a group that is no number is 0, which is never the program's own
	start, err = strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return "", 0, 0, 0, false
	}
	return name, ppid, pgid, start, true
}
