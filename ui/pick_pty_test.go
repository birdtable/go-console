package ui

// A menu on a real terminal under load: the screen a Pick draws before a key, and the
// screen it leaves after ctrl-c, are the same on every run. The test binary ptyprog runs on a pty,
// told PTYPROG=menu; the test waits for the description, waits for 200 ms of quiet, reads the screen
// as an emulator draws it, sends ctrl-c, and reads the screen again once the program has gone.

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-console/uitest"
)

// pickFrames runs ptyprog's menu on a pty cols wide, and returns the screen once the menu has been quiet
// for 200 ms and the screen after ctrl-c, when the program has exited and the pty has been read to its end.
func pickFrames(t *testing.T, cols int) (before, after string) {
	t.Helper()
	exe, err := ptyProgram()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), exe)
	cmd.Env = []string{"TERM=xterm-256color", "LANG=C.UTF-8", "PTYPROG=menu"}
	p, err := uitest.StartPty(t.Context(), cmd, cols, 50, false)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	before, ok := p.WaitFor(20*time.Second, func(s string, idle time.Duration) bool {
		return strings.Contains(s, "What now?") && strings.Contains(s, "--pick") && idle >= 200*time.Millisecond
	})
	if !ok {
		t.Fatal("the menu was not drawn, and quiet, in 20 s")
	}
	_ = p.Send("\x03")
	p.Wait()
	return before, p.Screen()
}

// burn keeps every CPU of the machine more than busy until the test ends: the load a menu is drawn
// under on a busy machine, which is what shows a draw that depends on timing.
func burn(t *testing.T) {
	t.Helper()
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	for range 3 * runtime.NumCPU() {
		go func() {
			sum := 0
			for {
				select {
				case <-stop:
					return
				default:
				}
				for j := range 1_000_000 {
					sum += j
				}
			}
		}()
	}
}

// A Pick with a title and a description, at 40 columns, the narrowest, draws the same screen on every run
// while the machine is loaded: the frame before a key, and the screen after ctrl-c. Each is a golden.
//
// What it costs when this fails: a person on a busy machine sees the menu erased on one run and left on
// the screen on the next, or a row short, and a program's own pty tests of its menus pass and fail by
// the load of the machine.
func TestAPickDrawsTheSameScreensWhateverTheLoad(t *testing.T) {
	t.Parallel()
	burn(t)
	for _, cols := range []int{40} {
		t.Run(strconv.Itoa(cols), func(t *testing.T) {
			t.Parallel()
			before, after := pickFrames(t, cols)
			uitest.Golden(t, "pick-before-"+strconv.Itoa(cols), before)
			uitest.Golden(t, "pick-after-"+strconv.Itoa(cols), after)
		})
	}
}
