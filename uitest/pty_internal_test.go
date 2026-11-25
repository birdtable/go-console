package uitest

import (
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/charmbracelet/x/xpty"
)

// stuck is a pty whose program has stopped reading its input: a write waits for release.
type stuck struct {
	xpty.Pty
	out     *io.PipeReader
	release chan struct{}
}

func (s *stuck) Start(*exec.Cmd) error      { return nil }
func (s *stuck) Read(b []byte) (int, error) { return s.out.Read(b) }
func (s *stuck) Close() error               { return nil }
func (s *stuck) Write(b []byte) (int, error) {
	<-s.release
	return len(b), nil
}

// The screen answers a query of the program, and the answer waits to be written in a queue of its own: a
// program that has stopped reading its input must not hold the screen's lock, so WaitFor returns at its deadline
// and Send returns. Before, the answer was written to the pty by the same call that drew the screen, under the
// lock, and WaitFor hung there past any deadline, with none of a test's own failure messages printed.
//
// What it costs when this fails: a pty test of ui hangs for minutes under load, and never says what it waited for.
func TestAProgramThatReadsNoAnswersDoesNotHoldTheDriver(t *testing.T) {
	t.Parallel()
	out, in := io.Pipe()
	s := &stuck{out: out, release: make(chan struct{})}
	defer close(s.release)
	defer in.Close()
	p, err := startOn(t.Context(), exec.CommandContext(t.Context(), "none"), s, 20, false)
	if err != nil {
		t.Fatal(err)
	}
	// Two device attributes queries, which the screen answers. The first answer is taken by the reader of the
	// screen's replies before its write at the pty blocks; the second waits in the screen, and the call that draws
	// it holds the lock until something reads it. One query is not enough to show it. They are
	// written from a goroutine of their own: where the lock is held, the second write waits as well.
	go func() {
		for range 2 {
			if _, err := io.WriteString(in, "\x1b[c"); err != nil {
				return
			}
		}
	}()
	done := make(chan bool, 1)
	go func() {
		_, ok := p.WaitFor(500*time.Millisecond, func(string, time.Duration) bool { return false })
		_ = p.Send("a key")
		done <- ok
	}()
	select {
	case ok := <-done:
		if ok {
			t.Error("WaitFor said yes to a condition that is never true")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("WaitFor or Send did not return 9 s after a 0.5 s deadline: a program that reads no answers holds the driver")
	}
}
