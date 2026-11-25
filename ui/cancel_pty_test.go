package ui

// A form cancelled with ctrl-c keeps its last frame on the screen, deterministically.
// huh's own cancel is tea.Interrupt, which skips the final render and flush, so what
// stayed depended on what had been flushed. The form path cancels with tea.Quit, which renders once more
// every time, and still ends as huh.ErrUserAborted: the program's closing word is "declined".

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-console/uitest"
)

// cancelledForm runs ptyprog in mode on a pty under load, waits for the form to be drawn and quiet, sends
// ctrl-c, and returns the screen before, the screen after the program has gone, and its exit code.
func cancelledForm(t *testing.T, mode, title string) (before, after string, code int) {
	t.Helper()
	exe, err := ptyProgram()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), exe)
	cmd.Env = []string{"TERM=xterm-256color", "LANG=C.UTF-8", "PTYPROG=" + mode}
	p, err := uitest.StartPty(t.Context(), cmd, 60, 50, false)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	before, ok := p.WaitFor(20*time.Second, func(s string, idle time.Duration) bool {
		return strings.Contains(s, title) && idle >= 200*time.Millisecond
	})
	if !ok {
		t.Fatalf("the form was not drawn, and quiet, in 20 s:\n%s", p.Screen())
	}
	_ = p.Send("\x03")
	code = p.Wait()
	return before, p.Screen(), code
}

// The screen after ctrl-c holds every row of the form the screen held before it, the last included, with
// the closing word on a row of its own under them, and the program ends declined, the
// ending ErrUserAborted gives. Plant: put huh's tea.Interrupt back as the cancel in
// runKept. On the Mac that stayed green in 20 of 20 runs, so the proof it can fail is the 100 runs on
// Linux, not this machine.
func TestACancelledFormKeepsItsLastFrame(t *testing.T) {
	t.Parallel()
	burn(t)
	for _, tc := range []struct{ name, mode, title string }{
		{"Session.Ask", "ask", "Which region?"},
		{"Pick", "menu", "What now?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before, after, code := cancelledForm(t, tc.mode, tc.title)
			for row := range strings.SplitSeq(strings.TrimRight(before, "\n "), "\n") {
				if row = strings.TrimSpace(row); row != "" && !strings.Contains(after, row) {
					t.Errorf("the row %q was on the screen before ctrl-c and is not after:\n%s", row, after)
				}
			}
			if !strings.Contains(after, "declined") {
				t.Errorf("the cancel did not end declined:\n%s", after)
			}
			if code != 6 {
				t.Errorf("exit %d, want 6 (declined)", code)
			}
		})
	}
}

// A key that changes the view and ctrl-c in one write: the cursor goes down and the form is cancelled before
// the frame that shows it is drawn. The kept frame must show the cursor where it moved, which a cancel that
// skips the last render (huh's tea.Interrupt) would not draw. The error is still
// ErrUserAborted: the closing word is declined, exit 6. Plant: tea.Interrupt as the cancel in runKept.
func TestACancelledFormKeepsTheCursorMovedInTheSameWrite(t *testing.T) {
	t.Parallel()
	burn(t)
	for _, tc := range []struct{ name, mode, title, moved string }{
		{"Session.Ask", "ask", "Which region?", "> south"},
		{"Pick", "menu", "What now?", "> Stay quiet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			exe, err := ptyProgram()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(t.Context(), exe)
			cmd.Env = []string{"TERM=xterm-256color", "LANG=C.UTF-8", "PTYPROG=" + tc.mode}
			p, err := uitest.StartPty(t.Context(), cmd, 60, 50, false)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			if _, ok := p.WaitFor(20*time.Second, func(s string, idle time.Duration) bool {
				return strings.Contains(s, tc.title) && idle >= 200*time.Millisecond
			}); !ok {
				t.Fatalf("the form was not drawn, and quiet, in 20 s:\n%s", p.Screen())
			}
			_ = p.Send("\x1b[B\x03")
			code := p.Wait()
			after := p.Screen()
			if !strings.Contains(after, tc.moved) {
				t.Errorf("the cursor moved in the same write as ctrl-c is not on the kept frame (%q):\n%s", tc.moved, after)
			}
			if !strings.Contains(after, "declined") || code != 6 {
				t.Errorf("the cancel did not end declined (exit %d):\n%s", code, after)
			}
		})
	}
}

// An Ask whose last row is an option (west, after north and south) keeps its options when it is cancelled: the closing word is printed
// on a fresh row under the kept view, and does not take the option's place. Plant: take
// the empty row off the kept view in keptModel.View, so the renderer's erase of the last row takes the
// option.
func TestACancelledAskKeepsItsLastOption(t *testing.T) {
	t.Parallel()
	burn(t)
	_, after, _ := cancelledForm(t, "ask", "Which region?")
	rows := strings.Split(strings.TrimRight(after, "\n "), "\n")
	n := len(rows)
	if n < 3 || !strings.Contains(rows[n-1], "declined") || !strings.Contains(rows[n-2], "west") || !strings.Contains(after, "south") {
		t.Errorf("want the options south and west kept, west on the row above the closing word:\n%s", after)
	}
}
