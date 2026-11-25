//go:build !windows

package ui

import "strings"

// splitPager splits $PAGER on spaces, as sh does for git and less here.
func splitPager(pager string) []string { return strings.Fields(pager) }
