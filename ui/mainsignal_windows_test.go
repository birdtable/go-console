//go:build windows

package ui

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Ctrl+Break, the stop a test can deliver to a child of its own on Windows, cancels the context ui.Main gives its
// commands, so a command that returns its context's error ends the run on the failed ending. The helper waits on its
// context and prints a line when it is cancelled; this test starts it in a process group of its own, sends that group
// Ctrl+Break once it is running, and reads the line and the failed ending's non-zero code back.
//
// What it costs when this fails: a program on Windows that is asked to stop keeps running, or stops without ending on
// the failed ending, so a person who presses Ctrl+Break and a script that reads the exit code are both told the wrong
// thing about a run that was interrupted.
func TestCtrlBreakCancelsMainsContextAndEndsOnTheFailedEnding(t *testing.T) {
	t.Parallel()
	exe := buildHelper(t, "ctxwait")
	var out, errOut syncBuffer
	// A process group of its own, so the Ctrl+Break reaches this child and not the test that sends it.
	cmd := exec.CommandContext(t.Context(), exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", exe, err)
	}
	waitForLine(t, &out, "READY") // the command is running and blocked on its context, before the stop is sent
	if err := ctrlBreak(cmd.Process.Pid); err != nil {
		t.Fatalf("GenerateConsoleCtrlEvent(CTRL_BREAK) to %d: %v (stderr %q)", cmd.Process.Pid, err, errOut.String())
	}
	err := cmd.Wait()
	if !strings.Contains(out.String(), "CANCELLED") {
		t.Errorf("after Ctrl+Break the program did not report its context cancelled; stdout %q, stderr %q", out.String(), errOut.String())
	}
	if code := exitCodeOf(err); code != 1 {
		t.Errorf("after Ctrl+Break the run ended with code %d, want 1 (the failed ending); stderr %q", code, errOut.String())
	}
}

// Stopped answers true with os.Interrupt, which Ctrl+Break is to a Go program on Windows, once it has ended the
// context of a running command, and a second Ctrl+Break while that command still runs neither ends the program nor
// changes the answer. The helper prints what Stopped says after the cancel, waits for a line on its standard input,
// and prints it again: this test sends the second Ctrl+Break and then the line, and reads both answers and the
// failed ending's code back.
//
// What it costs when this fails: a program on Windows cannot say that a stop was asked for, or a second Ctrl+Break
// kills it in the middle of the clean stop the first one asked for.
func TestStoppedNamesCtrlBreakAndASecondOneChangesNothing(t *testing.T) {
	t.Parallel()
	exe := buildHelper(t, "ctxwait")
	var out, errOut syncBuffer
	cmd := exec.CommandContext(t.Context(), exe, "again")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	cmd.Stdout, cmd.Stderr = &out, &errOut
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", exe, err)
	}
	waitForLine(t, &out, "READY")
	if err := ctrlBreak(cmd.Process.Pid); err != nil {
		t.Fatalf("GenerateConsoleCtrlEvent(CTRL_BREAK) to %d: %v (stderr %q)", cmd.Process.Pid, err, errOut.String())
	}
	waitForLine(t, &out, "STOPPED interrupt")
	if err := ctrlBreak(cmd.Process.Pid); err != nil {
		t.Fatalf("the second GenerateConsoleCtrlEvent(CTRL_BREAK) to %d: %v (stderr %q)", cmd.Process.Pid, err, errOut.String())
	}
	if _, err := in.Write([]byte("\n")); err != nil {
		t.Fatalf("the line to the helper after the second Ctrl+Break: %v (stdout %q, stderr %q)", err, out.String(), errOut.String())
	}
	waitForLine(t, &out, "AGAIN interrupt")
	if code := exitCodeOf(cmd.Wait()); code != 1 {
		t.Errorf("after two Ctrl+Breaks the run ended with code %d, want 1 (the failed ending); stdout %q, stderr %q", code, out.String(), errOut.String())
	}
}

// buildHelper builds a program of ui/testdata under its own name in a folder of its own and returns the path.
func buildHelper(t *testing.T, name string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), name+".exe")
	if out, err := exec.CommandContext(t.Context(), "go", "build", "-o", exe, "./testdata/"+name).CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v: %s", name, err, out)
	}
	return exe
}

// ctrlBreak sends CTRL_BREAK_EVENT to the process group whose id is pid, through kernel32, reached by the standard
// library's syscall and not golang.org/x/sys. GenerateConsoleCtrlEvent returns zero and sets the error on failure.
func ctrlBreak(pid int) error {
	gen := syscall.NewLazyDLL("kernel32.dll").NewProc("GenerateConsoleCtrlEvent")
	r, _, err := gen.Call(uintptr(syscall.CTRL_BREAK_EVENT), uintptr(pid))
	if r == 0 {
		return err
	}
	return nil
}

// waitForLine fails if want does not appear on buf within ten seconds, or if the test's context is cancelled first.
func waitForLine(t *testing.T, buf *syncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if strings.Contains(buf.String(), want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("waited for %q and it did not come; got %q", want, buf.String())
		}
		select {
		case <-t.Context().Done():
			t.Fatalf("the test's context ended while waiting for %q; got %q", want, buf.String())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// exitCodeOf is the process's exit code, or 0 when it left cleanly.
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	if ee := (*exec.ExitError)(nil); errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// syncBuffer is a bytes buffer a child's streams may be written to from the os/exec copying goroutine while the test
// reads it, each behind the same lock.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
