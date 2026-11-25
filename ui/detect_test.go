package ui

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Two sessions on two buffers, with two Terminals, write at once and each gets
// its own bytes: one painted 30 wide, one plain, neither the other's, and
// nothing reaches the process's streams.
//
// What it costs when this fails: a program with two sessions, as a server with
// one a request, writes one's lines into the other's stream or paints a pipe.
func TestTwoSessionsOnTwoBuffersEachGetTheirOwnBytes(t *testing.T) {
	t.Parallel()
	var richOut, richErr, plainOut, plainErr bytes.Buffer
	rich := New(Out(&richOut), Err(&richErr), On(Terminal{Unicode: true, Colour: true, ErrColour: true, Width: 30}), WithMarkup(BacktickMarkup))
	plain := New(Out(&plainOut), Err(&plainErr))
	const rounds = 50
	var wg sync.WaitGroup
	for _, s := range []*Session{rich, plain} {
		wg.Go(func() {
			for range rounds {
				s.Log("run `app sync` to bring the folder up to date with the remote copy")
			}
			s.Warn(closing{word: "warning", what: "the cache is old"})
		})
	}
	wg.Wait()
	if !strings.Contains(richOut.String(), "\x1b[") || strings.Contains(plainOut.String(), "\x1b[") {
		t.Errorf("the rich session's lines are plain or the plain one's painted:\n%q\n%q", richOut.String()[:80], plainOut.String()[:80])
	}
	if got := strings.Count(plainOut.String(), "\n"); got != rounds {
		t.Errorf("the plain session wrote %d lines, want %d, one a Log", got, rounds)
	}
	if got := strings.Count(richOut.String(), "\n"); got <= rounds {
		t.Errorf("the rich session wrote %d lines, want each Log wrapped to 30 cells", got)
	}
	if !strings.Contains(richErr.String(), "cache") || !strings.Contains(plainErr.String(), "cache") {
		t.Errorf("a warning missed its own error stream: %q, %q", richErr.String(), plainErr.String())
	}
}

// Only detect.go reads the process: no other file of ui names os.Stdout,
// os.Stderr, os.Stdin, os.Getenv, os.LookupEnv, os.Environ or os.Args, or calls
// time.Now. It reads the files as Go, so a comment that names one does not count.
//
// What it costs when this fails: a program that hands its session its own
// streams, environment or clock gets the process's somewhere, and a test that
// runs two sessions at once sees them cross.
func TestOnlyDetectReadsTheProcess(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	ambient := map[string]bool{
		"os.Stdout": true, "os.Stderr": true, "os.Stdin": true, "os.Getenv": true,
		"os.LookupEnv": true, "os.Environ": true, "os.Args": true, "time.Now": true,
		"time.Since": true, "time.Until": true,
	}
	read := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || name == "detect.go" {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		// Each local name the file gives an import, so os imported as xos is os.
		imported := map[string]string{}
		named := 0
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			local := pathpkg.Base(path)
			if imp.Name != nil {
				local = imp.Name.Name
			}
			if local == "." && (path == "os" || path == "time") {
				t.Errorf("%s imports %s with a dot, so its names cannot be told apart", name, path)
			}
			imported[local] = path
			named++
		}
		if named != len(f.Imports) {
			t.Fatalf("%s: named %d of %d imports", name, named, len(f.Imports))
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if pkg, ok := sel.X.(*ast.Ident); ok && ambient[imported[pkg.Name]+"."+sel.Sel.Name] {
					t.Errorf("%s reads %s.%s", name, imported[pkg.Name], sel.Sel.Name)
				}
			}
			return true
		})
		read++
	}
	if read < 20 {
		t.Fatalf("read %d files, want ui's", read)
	}
	// detect.go does read them, so the check above has something to find.
	if b, err := os.ReadFile("detect.go"); err != nil || !strings.Contains(string(b), "os.Stdout") {
		t.Fatalf("detect.go names no os.Stdout, so this proves nothing: %v", err)
	}
}

// A session's clock is the one Clock gives: a bar's start and a wait's time
// left are read from it, not from the process's.
//
// What it costs when this fails: a test of a five-hour run takes five hours, or a
// program replaying a log gets the wall clock's rates.
func TestASessionsClockIsTheOneItWasGiven(t *testing.T) {
	t.Parallel()
	at := time.Unix(1_000_000, 0)
	s := New(Clock(func() time.Time { return at }))
	if b := s.NewBar("reading", 10); !b.started.Equal(at) {
		t.Errorf("a bar started at %v, want the session's %v", b.started, at)
	}
	if w := s.NewWait("measuring", time.Minute); !w.now().Equal(at) {
		t.Errorf("a wait reads %v, want the session's %v", w.now(), at)
	}
}

// A bar run under a Clock closes on the time that clock says passed, and logs its
// last line, off a terminal, when it is done.
//
// What it costs when this fails: a bar under a fixed clock closes on "10 files in
// 498211h 36m", or one under a clock ahead of the wall closes on a negative time
// and never logs its 100% line.
func TestABarUnderAClockClosesOnItsTime(t *testing.T) {
	t.Parallel()
	at := time.Unix(1_000_000, 0)
	var out bytes.Buffer
	s := New(Out(&out), Err(&out), Clock(func() time.Time { return at }))
	b := s.NewBar("reading", 10)
	// Off a terminal a bar logs a line every two minutes, so three later its 10/10
	// is logged, and it closes on the three minutes, which short gives as 3m.
	at = at.Add(3 * time.Minute)
	b.Add(10)
	b.Stop()
	if !strings.Contains(out.String(), "reading 10/10") {
		t.Errorf("the bar logged no 100%% line:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "10 files in 3m") {
		t.Errorf("the bar did not close on the 3m its clock said passed:\n%s", out.String())
	}
}

// Detect reports the colour's depth: 0 where there is no colour, 24 for the COLORTERM values and the TERMs colorprofile takes
// as truecolor (truecolor, 24bit, yes, true; kitty, st, xterm-direct), 8 for a TERM it takes as 256 colours (256color,
// screen, tmux), and 4 otherwise; NO_COLOR is no colour at any depth.
//
// What it costs when this fails: a program draws 24-bit colour on a screen that has 16, or takes a
// truecolor screen for one with 16 and draws its palette in the wrong shades.
func TestDetectReportsTheColourDepth(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		env  []string
		want int
	}{
		{"truecolor", []string{"CLICOLOR_FORCE=1", "COLORTERM=truecolor", "TERM=xterm"}, 24},
		{"24bit", []string{"CLICOLOR_FORCE=1", "COLORTERM=24bit", "TERM=xterm"}, 24},
		{"truecolor beats a 256color TERM", []string{"CLICOLOR_FORCE=1", "COLORTERM=truecolor", "TERM=xterm-256color"}, 24},
		{"COLORTERM unset", []string{"CLICOLOR_FORCE=1", "TERM=xterm"}, 4},
		{"yes is truecolor to colorprofile", []string{"CLICOLOR_FORCE=1", "COLORTERM=yes", "TERM=xterm"}, 24},
		{"another COLORTERM", []string{"CLICOLOR_FORCE=1", "COLORTERM=ansi", "TERM=xterm"}, 4},
		{"screen is 256 colours to colorprofile", []string{"CLICOLOR_FORCE=1", "TERM=screen"}, 8},
		{"kitty is truecolor to colorprofile", []string{"CLICOLOR_FORCE=1", "TERM=xterm-kitty"}, 24},
		{"an empty TERM, with colour forced", []string{"CLICOLOR_FORCE=1", "TERM="}, 4},
		{"no TERM, with colour forced", []string{"CLICOLOR_FORCE=1"}, 4},
		{"a 256color TERM", []string{"CLICOLOR_FORCE=1", "TERM=xterm-256color"}, 8},
		{"NO_COLOR", []string{"CLICOLOR_FORCE=1", "NO_COLOR=1", "COLORTERM=truecolor", "TERM=xterm-256color"}, 0},
		{"a pipe", []string{"COLORTERM=truecolor", "TERM=xterm-256color"}, 0},
	} {
		got := Detect(nil, &bytes.Buffer{}, &bytes.Buffer{}, c.env, StandardEnv())
		if got.ColourBits != c.want {
			t.Errorf("%s: ColourBits %d (Colour %v), want %d", c.name, got.ColourBits, got.Colour, c.want)
		}
	}
	if got := Detect(nil, &bytes.Buffer{}, &bytes.Buffer{}, []string{"CLICOLOR_FORCE=1", "COLORTERM=truecolor"}, EnvRules{ForceColour: "CLICOLOR_FORCE"}); got.ColourBits != 4 {
		t.Errorf("rules that name no COLORTERM read it anyway: %d", got.ColourBits)
	}
}
