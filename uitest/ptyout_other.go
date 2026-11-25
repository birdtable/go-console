//go:build !windows

package uitest

import (
	"io"

	"github.com/charmbracelet/x/xpty"
)

// ptyOutput is the pty itself: its output ends at the mark Wait writes at the slave.
func ptyOutput(p xpty.Pty) (io.ReadCloser, error) { return io.NopCloser(p), nil }
