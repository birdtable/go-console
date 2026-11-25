package ui

import "golang.org/x/sys/windows"

// splitPager splits $PAGER by the Windows command-line rules, so a quoted path with spaces is one word. A pager
// with a NUL in it is no command line, and gives no words.
func splitPager(pager string) []string {
	argv, err := windows.DecomposeCommandLine(pager)
	if err != nil {
		return nil
	}
	return argv
}
