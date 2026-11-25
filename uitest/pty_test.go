package uitest_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-console/uitest"
)

// The driver at a pty, on the program ui's pty tests build: its help is read as a screen, WaitFor waits for
// the quiet it is asked for, and Wait gives the exit once all the output is read.
//
// What it costs when this fails: a pty test of ui reads a half-drawn screen, or none, or hangs.
func TestAPtyReadsTheScreenWaitsForQuietAndGivesTheExit(t *testing.T) {
	t.Parallel()
	exe := filepath.Join(t.TempDir(), "tool.exe")
	if out, err := exec.CommandContext(t.Context(), "go", "build", "-o", exe, "../ui/testdata/ptyprog").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	cmd := exec.CommandContext(t.Context(), exe, "--help")
	cmd.Env = []string{"TERM=xterm-256color"}
	p, err := uitest.StartPty(t.Context(), cmd, 60, 50, false)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, ok := p.WaitFor(20*time.Second, func(s string, _ time.Duration) bool { return strings.Contains(s, "--loud") }); !ok {
		t.Fatalf("the help never showed --loud: %q", p.Screen())
	}
	const quiet = 300 * time.Millisecond
	start := time.Now()
	if _, ok := p.WaitFor(20*time.Second, func(_ string, idle time.Duration) bool { return idle >= quiet }); !ok || time.Since(start) < quiet-100*time.Millisecond {
		t.Errorf("WaitFor for %v of quiet gave %v after %v", quiet, ok, time.Since(start))
	}
	if code := p.Wait(); code != 0 {
		t.Errorf("the program exited %d, want 0", code)
	}
	screen := p.Screen()
	if !strings.Contains(screen, "--yes") || strings.HasSuffix(screen, "\n") {
		t.Errorf("the screen after the exit is %q, want the whole help with no blank row after it", screen)
	}
	for i, r := range strings.Split(screen, "\n") {
		if r != strings.TrimRight(r, " ") {
			t.Errorf("row %d keeps its trailing spaces: %q", i+1, r)
		}
	}
}

// The scrollback of a pty is the rows that scrolled off the top, kept up to the count the caller named and none
// without it: a program whose help is taller than the pty has its first rows in Scrollback and its last in Screen.
//
// What it costs when this fails: a program that reads a screen that has lost the top of a long output, or a test keeps
// rows it never asked for.
func TestAPtyScrollbackKeepsTheRowsOffTheTopUpToTheCallersCount(t *testing.T) {
	t.Parallel()
	exe := filepath.Join(t.TempDir(), "tool.exe")
	if out, err := exec.CommandContext(t.Context(), "go", "build", "-o", exe, "../ui/testdata/ptyprog").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	run := func(rows int, opts ...uitest.PtyOption) *uitest.Pty {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), exe, "--help")
		cmd.Env = []string{"TERM=xterm-256color"}
		p, err := uitest.StartPty(t.Context(), cmd, 60, rows, false, opts...)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		if code := p.Wait(); code != 0 {
			t.Fatalf("the program exited %d, want 0", code)
		}
		return p
	}
	whole := run(50).Screen()
	all := strings.Split(whole, "\n")
	const rows = 5
	if len(all) <= rows+2 {
		t.Fatalf("the help is %d rows, too short to scroll", len(all))
	}
	big := run(rows, uitest.PtyScrollback(1000))
	wantOff := all[:len(all)-len(strings.Split(big.Screen(), "\n"))]
	if got := big.Scrollback() + "\n" + big.Screen(); got != whole {
		t.Errorf("Scrollback and Screen give %q, want the whole help %q", got, whole)
	}
	if got := strings.Split(big.Scrollback(), "\n"); len(got) != len(wantOff) {
		t.Errorf("Scrollback is %d rows, want %d", len(got), len(wantOff))
	}
	if got, want := run(rows, uitest.PtyScrollback(2)).Scrollback(), strings.Join(wantOff[len(wantOff)-2:], "\n"); got != want {
		t.Errorf("a count of 2 gives %q, want the last two rows off the top, %q", got, want)
	}
	if got := run(rows).Scrollback(); got != "" {
		t.Errorf("with no PtyScrollback, Scrollback is %q, want none", got)
	}
}
