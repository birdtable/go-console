package uitest

import (
	"errors"
	"io"
	"syscall"

	"github.com/charmbracelet/x/xpty"
)

// ptyOutput gives a ConPTY's reader its own copy of the output pipe's read end: the pty closes its end with
// itself and loses what is unread, and the copy reads on until the pipe ends, after the last frame.
func ptyOutput(p xpty.Pty) (io.ReadCloser, error) {
	c, ok := p.(*xpty.ConPty)
	if !ok {
		return io.NopCloser(p), nil
	}
	self, err := syscall.GetCurrentProcess()
	if err != nil {
		return nil, err //nolint:wrapcheck // syscall's words say it
	}
	var dup syscall.Handle
	if err = syscall.DuplicateHandle(self, syscall.Handle(c.OutPipeReadFd()), self, &dup, 0, false, syscall.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err //nolint:wrapcheck // syscall's words say it
	}
	return pipeEnd(dup), nil
}

type pipeEnd syscall.Handle

func (h pipeEnd) Read(b []byte) (n int, err error) {
	var read uint32
	if err = syscall.ReadFile(syscall.Handle(h), b, &read, nil); errors.Is(err, syscall.ERROR_BROKEN_PIPE) {
		err = io.EOF
	}
	return int(read), err
}

func (h pipeEnd) Close() error { return syscall.CloseHandle(syscall.Handle(h)) }
