package ui

// Help on a real terminal: Execute hands fang the real files, so fang finds a
// terminal, paints, and reads the screen's width itself. The tests re-run the
// test binary as the program, with its streams on a pty (or on pipes), because
// fang reads the process's own standard output.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/birdtable/go-console/uitest"
)

// ptyProgram is the path of ui/testdata/ptyprog, built once.
var ptyProgram = sync.OnceValues(func() (string, error) { //nolint:gochecknoglobals // builds the test program once for every pty test, and nothing else reads it
	dir, err := os.MkdirTemp("", "ptyprog-")
	if err != nil {
		return "", err
	}
	exe := filepath.Join(dir, "tool.exe")
	if out, err := exec.Command("go", "build", "-o", exe, "./testdata/ptyprog").CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build ptyprog: %w: %s", err, out)
	}
	return exe, nil
})

// helpAt runs the program with args and env on a pty cols wide, or on pipes
// when cols is 0, and returns what it wrote and how long it took. The pty
// answers the queries fang and lipgloss send, as a terminal does: left
// unanswered, each waits out 2 s before the program goes on (measured at
// 4.2 s for one help run; answered, 13 ms), and that wait is theirs, not the
// product's.
func helpAt(t *testing.T, cols int, env []string, args ...string) (string, time.Duration) {
	t.Helper()
	return helpRun(t, cols, env, true, args...)
}

// helpRun is helpAt with the choice of answering the queries. A pty that
// answers none is how TestHelpFitsAPty holds that help still returns.
func helpRun(t *testing.T, cols int, env []string, answer bool, args ...string) (string, time.Duration) {
	t.Helper()
	exe, err := ptyProgram()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), exe, args...)
	cmd.Env = env // nothing of the tests' own: the program reads only what a case gives it
	start := time.Now()
	var out bytes.Buffer
	if cols == 0 {
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("%v: %s", err, out.String())
		}
		return out.String(), time.Since(start)
	}
	p, err := uitest.StartPty(t.Context(), cmd, cols, 50, !answer)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if code := p.Wait(); code != 0 {
		t.Fatalf("exit %d: %s", code, p.Screen())
	}
	return p.Styled(), time.Since(start)
}

// `tool --help` at a terminal holds colour escapes. Plant: wrap the stream
// again in a writer that is no file, and the count is 0 and this is red.
//
// What it costs when this fails: the house help colours never show at a
// terminal, and Windows consoles are never switched to escapes.
func TestHelpAtAPtyHasColour(t *testing.T) {
	t.Parallel()
	got, _ := helpAt(t, 80, []string{"TERM=xterm-256color"}, "--help")
	if n := strings.Count(got, "\x1b["); n == 0 {
		t.Errorf("help at a terminal holds no colour escape: %q", got)
	}
}

// Every other face of help holds no escape byte: a pipe, --no-color, NO_COLOR
// set to anything, and TERM=dumb.
func TestHelpHoldsNoEscapeWhereColourIsNotWanted(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		cols int
		env  []string
		args []string
	}{
		{"a pipe", 0, []string{"TERM=xterm-256color"}, []string{"--help"}},
		{"--no-color", 80, []string{"TERM=xterm-256color"}, []string{"--no-color", "--help"}},
		{"NO_COLOR", 80, []string{"TERM=xterm-256color", "NO_COLOR=yes"}, []string{"--help"}},
		{"TERM=dumb", 80, []string{"TERM=dumb"}, []string{"--help"}},
	} {
		got, _ := helpAt(t, c.cols, c.env, c.args...)
		if strings.ContainsRune(got, '\x1b') {
			t.Errorf("%s: help holds an escape byte: %q", c.name, got)
		}
		if !strings.Contains(got, "loud") {
			t.Errorf("%s: help lost its flags: %q", c.name, got)
		}
	}
}

// Help fits the terminal at 80, 60 and 40 columns under a pty that answers the queries as a
// terminal does: every row, the flag and command rows too, as ui draws it at the screen's width
// and not at fang's own. That help still
// returns at a pty that answers nothing is held at the release tier, by TestHelpFitsAPty.
func TestHelpFitsAPtyAtEachWidth(t *testing.T) {
	t.Parallel()
	for _, cols := range []int{80, 60, 40} {
		t.Run(strconv.Itoa(cols), func(t *testing.T) {
			t.Parallel()
			got, _ := helpAt(t, cols, []string{"TERM=xterm-256color"}, "--help")
			for i, l := range strings.Split(got, "\n") {
				if w := ansi.StringWidth(strings.TrimRight(l, "\r ")); w > cols {
					t.Errorf("row %d is %d columns wide on a %d-column screen: %q", i+1, w, cols, l)
				}
			}
		})
	}
}

// A child that exits at once, as no-colour help does, has all its output read by the master.
//
// What it costs when this fails: the pty tests read an empty screen under load and fail on a
// program that is right.
func TestAChildThatExitsAtOnceIsReadWhole(t *testing.T) {
	t.Parallel()
	childExitsAtOnce(t, 5)
}

// childExitsAtOnce runs the exits-at-once child n times and holds each run to its whole output. A
// push runs it 5 times; the release tier runs it 50 (TestAChildThatExitsAtOnceIsReadWholeFifty), a
// stress that took 1.3 s alone and over 4 s at a push under load.
func childExitsAtOnce(t *testing.T, n int) {
	t.Helper()
	for range n {
		got, _ := helpAt(t, 80, []string{"TERM=xterm-256color"}, "--no-color", "--help")
		if !strings.Contains(got, "loud") {
			t.Fatalf("help read as %q", got)
		}
	}
}
