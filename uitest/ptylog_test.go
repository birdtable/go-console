package uitest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-console/uitest"
)

// A caller names the pty's byte log with the PtyLog option, and it holds what the program wrote. The test runs under
// go-console's own test main, which clears the environment: a log named by an environment variable would be empty
// here, and this one is not.
//
// What it costs when this fails: a program's pty run cannot be read afterwards, and the caller who needs the
// bytes has no way to ask for them that works under a cleared environment.
func TestAPtyLogNamedByTheCallerHoldsWhatTheProgramWrote(t *testing.T) {
	t.Parallel()
	exe := filepath.Join(t.TempDir(), "tool.exe")
	if out, err := exec.CommandContext(t.Context(), "go", "build", "-o", exe, "../ui/testdata/ptyprog").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	logFile := filepath.Join(t.TempDir(), "bytes.log")
	cmd := exec.CommandContext(t.Context(), exe, "--help")
	cmd.Env = []string{"TERM=xterm-256color"}
	p, err := uitest.StartPty(t.Context(), cmd, 60, 50, false, uitest.PtyLog(logFile))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.WaitFor(20*time.Second, func(s string, _ time.Duration) bool { return strings.Contains(s, "--loud") }); !ok {
		t.Fatalf("the help never showed --loud: %q", p.Screen())
	}
	p.Wait()
	p.Close()
	b, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("the log named by PtyLog was not made: %v", err)
	}
	if !strings.Contains(string(b), " read ") || !strings.Contains(string(b), "--loud") {
		t.Errorf("the log holds %q, want the program's reads with its bytes", b)
	}
}

// Without the option nothing is logged and no file is made, whatever the environment says.
//
// What it costs when this fails: a test run leaves a file nobody asked for, or reads a name the caller never gave.
func TestAPtyWithNoLogOptionLogsNothing(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "bytes.log")
	t.Setenv("UITEST_PTYLOG", logFile)
	cmd := exec.CommandContext(t.Context(), "go", "version")
	p, err := uitest.StartPty(t.Context(), cmd, 60, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	p.Wait()
	p.Close()
	if _, err := os.Stat(logFile); err == nil {
		t.Errorf("a log was made at %s with no PtyLog and only the environment naming it", logFile)
	}
}

// No environment variable is named or read in uitest: it takes its settings from its caller.
//
// What it costs when this fails: a second consumer finds a fixed name in an exported package it cannot change.
func TestUitestReadsNoEnvironmentVariable(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, word := range []string{"os.Getenv", "os.LookupEnv", "os.Environ", "UITEST_"} {
			if strings.Contains(string(raw), word) {
				t.Errorf("%s holds %q: uitest names no environment variable", f, word)
			}
		}
	}
}
