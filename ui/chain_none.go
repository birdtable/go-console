//go:build !linux && !darwin && !windows

package ui

// parentChain is empty on a system whose parent chain is not read here. Windows has its own walk.
func parentChain() ([]string, []bool) { return nil, nil }
