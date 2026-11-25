//go:build !windows

package ui

import "github.com/birdtable/go-console/shell"

// hereDialect reads the parent chain, nearest first, up to 8 levels (parentChain: Linux and macOS, and an empty
// chain elsewhere), and lets dialectOfGroups choose.
func hereDialect() (shell.Dialect, error) { return dialectOfGroups(parentChain()) }
