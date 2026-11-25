package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// errBroke is the error the commands of these tests return.
var errBroke = errors.New("it broke")

// breaks is a command that returns errBroke.
func breaks(*Session, *cobra.Command) error { return errBroke }

// handler is an error handler that answers code and handled, writes "handled: <err>" on w, and counts its calls.
func handler(calls *int, code int, handled bool) Option {
	return WithErrorHandler(func(w io.Writer, err error) (int, bool) {
		*calls++
		_, _ = fmt.Fprintln(w, "handled:", err)
		return code, handled
	})
}

// A handler that answers handled gives Main its code, 0 too, on a held session and on one that is not, and ui
// paints no ending. It is called once, with the error the command returned, and what it writes is on stderr.
//
// What it costs when this fails: a program that words its own failures exits on ui's failed code and not its
// own, or a person reads two closing lines, the program's and ui's.
func TestAnErrorHandlerThatHandlesGivesTheCodeAndNoEnding(t *testing.T) { //nolint:paralleltest // serial: it sets the process's arguments and streams.
	for _, c := range []struct {
		name string
		held []SessionOption
		code int
		want int
	}{
		{"a held session, 7", []SessionOption{Held()}, 7, 7},
		{"a session not held, 0", nil, 0, 0},
		{"a session not held, 7", nil, 7, 7},
		{"over 255 is held to 255", nil, 300, 255},
		{"under 0 is held to 0", nil, -1, 0},
	} {
		calls := 0
		var code int
		var out, errs string
		withArgs(t, nil, func() {
			code, out, errs = mainRun(t, oneCommand(breaks), WithSession(c.held...), handler(&calls, c.code, true))
		})
		if code != c.want || calls != 1 || out != "" || errs != "handled: it broke\n" {
			t.Errorf("%s: Main returned %d after %d calls, stdout %q, stderr %q; want %d, 1 call and only the handler's line", c.name, code, calls, out, errs, c.want)
		}
	}
}

// A handler that answers not handled changes nothing: for a command's error and for an unknown command the
// code and both streams are, past the handler's own line, the bytes of a run with no handler.
//
// What it costs when this fails: a program that handles only some errors loses ui's ending for the rest, or
// leaves with the code it said to ignore.
func TestAnErrorHandlerThatDoesNotHandleLeavesTheRunAsItWas(t *testing.T) { //nolint:paralleltest // serial: it sets the process's arguments and streams.
	for _, args := range [][]string{nil, {"nope"}, {"--nope"}} {
		tree := func(s *Session) *cobra.Command {
			root := oneCommand(breaks)(s)
			root.AddCommand(&cobra.Command{Use: "sub", Run: func(*cobra.Command, []string) {}})
			return root
		}
		calls := 0
		var code, plainCode int
		var out, errs, plainOut, plainErrs string
		withArgs(t, args, func() {
			plainCode, plainOut, plainErrs = mainRun(t, tree)
			code, out, errs = mainRun(t, tree, handler(&calls, 99, false))
		})
		line, rest, _ := strings.Cut(errs, "\n")
		if calls != 1 || !strings.HasPrefix(line, "handled: ") || code != plainCode || out != plainOut || rest != plainErrs {
			t.Errorf("%v: %d calls, code %d, stdout %q, stderr %q; with no handler code %d, stdout %q, stderr %q", args, calls, code, out, errs, plainCode, plainOut, plainErrs)
		}
		if plainCode == 0 || plainCode == 99 || !strings.Contains(plainErrs, "app --help") {
			t.Errorf("%v: with no handler the run ended on %d with %q, want ui's own ending", args, plainCode, plainErrs)
		}
	}
}

// The handler is not called for a panic, on a held session either, nor when a command ended the run itself,
// nor for an OwnCode subtree's own code. PanicWith words the panic's ending, and finish is given the failed code.
//
// What it costs when this fails: a panic's stack is handed to a handler that maps it to a quiet code, or a
// run that ended refused is ended a second time.
func TestTheErrorHandlerIsNotCalledForAPanicOrAnEndedRun(t *testing.T) { //nolint:paralleltest // serial: it sets the process's arguments and streams.
	worded := PanicWith(func(any) (string, string) { return "the program's own words", "app doctor" })
	for _, c := range []struct {
		name    string
		fn      func(*Session, *cobra.Command) error
		session []SessionOption
		want    int
		stderr  string
	}{
		{"a panic", func(*Session, *cobra.Command) error { panic("boom") }, []SessionOption{worded}, 1, "the program's own words"},
		{"a panic on a held session", func(*Session, *cobra.Command) error { panic("boom") }, []SessionOption{worded, Held()}, 1, "panic: boom"},
		{"a command that ended refused", func(s *Session, _ *cobra.Command) error { _ = s.End(refused()); return errBroke }, nil, 5, "not Debian"},
	} {
		calls, finished := 0, []int{}
		var code int
		var errs string
		withArgs(t, nil, func() {
			code, _, errs = mainRun(t, oneCommand(c.fn), WithSession(c.session...), handler(&calls, 7, true),
				WithFinish(func(code int) { finished = append(finished, code) }))
		})
		if calls != 0 || code != c.want || !slices.Equal(finished, []int{c.want}) || !strings.Contains(errs, c.stderr) || strings.Contains(errs, "handled:") {
			t.Errorf("%s: %d calls, Main returned %d, finish got %v, stderr %q; want no call, %d, and %q", c.name, calls, code, finished, errs, c.want, c.stderr)
		}
	}
	calls := 0
	var code int
	withArgs(t, []string{"hook"}, func() {
		code, _, _ = mainRun(t, func(*Session) *cobra.Command {
			root := &cobra.Command{Use: "app", SilenceUsage: true, SilenceErrors: true}
			root.AddCommand(OwnCode(&cobra.Command{Use: "hook", RunE: func(*cobra.Command, []string) error { return codedError{75} }}))
			return root
		}, handler(&calls, 7, true))
	})
	if calls != 0 || code != 75 {
		t.Errorf("an OwnCode subtree: %d calls, Main returned %d; want no call and 75", calls, code)
	}
}

// Under --json a handled error still prints one Result, with the handler's code and the error and no ending,
// and none when the handler printed its own document and said so.
//
// What it costs when this fails: a script reading --json gets no document for a handled error, or two, or
// one whose code is not the one the process left with.
func TestAHandledErrorStillPrintsTheResult(t *testing.T) { //nolint:paralleltest // serial: it sets the process's arguments and streams.
	var code int
	var out string
	calls := 0
	withArgs(t, []string{"--json"}, func() { code, out, _ = mainRun(t, oneCommand(breaks), handler(&calls, 7, true)) })
	r, err := ReadResult([]byte(out))
	if err != nil || code != 7 || r.Code != 7 || r.Word != "" || r.Error == nil || r.Error.Msg != "it broke" || r.Error.Kind != "" {
		t.Errorf("Main returned %d and printed %q (%v), want 7 and one Result of code 7 with the error and no ending", code, out, err)
	}
	var run *Session
	withArgs(t, []string{"--json"}, func() {
		code, out, _ = mainRun(t, func(s *Session) *cobra.Command { run = s; return oneCommand(breaks)(s) },
			WithErrorHandler(func(io.Writer, error) (int, bool) {
				_, _ = fmt.Fprintln(os.Stdout, `{"mine":true}`)
				run.MarkPrinted()
				return 9, true
			}))
	})
	if code != 9 || out != "{\"mine\":true}\n" {
		t.Errorf("a handler that printed its own document: Main returned %d, stdout %q; want 9 and only its document", code, out)
	}
}

// Main calls finish once, last, with the code it returns, on every return: a run that ended done, a command's
// error, a usage error, the menu off a terminal, the menu with nothing picked, the menu's picked run, and a
// session refused for having no Endings.
//
// What it costs when this fails: a program's run record is left open, or its lock held, on the one path that
// skipped the hook, or it is closed with a code the process did not leave with.
func TestMainCallsFinishOnceWithTheCodeItReturns(t *testing.T) { //nolint:paralleltest // serial: it sets the process's arguments and streams.
	ok := func(s *Session, _ *cobra.Command) error { return s.End(done()) }
	tree := func(fn func(*Session, *cobra.Command) error) func(*Session) *cobra.Command {
		return func(s *Session) *cobra.Command {
			root := &cobra.Command{Use: "app", Short: "does a thing", SilenceUsage: true, SilenceErrors: true}
			root.AddCommand(&cobra.Command{Use: "go", Short: "goes", RunE: func(c *cobra.Command, _ []string) error { return fn(s, c) }})
			return root
		}
	}
	screen := WithSession(On(Terminal{Interactive: true}))
	pick := func(argv []string, picked bool) Option {
		return WithMenu(func(*Session) ([]string, bool) { return argv, picked })
	}
	for _, c := range []struct {
		name string
		args []string
		fn   func(*Session, *cobra.Command) error
		opts []Option
		want int
	}{
		{"success", []string{"go"}, ok, nil, 0},
		{"a command's error", []string{"go"}, breaks, nil, 1},
		{"a usage error", []string{"go", "--nope"}, ok, nil, 2},
		{"the menu off a terminal", nil, ok, []Option{pick(nil, false)}, 2},
		{"the menu with nothing picked", nil, ok, []Option{screen, pick(nil, false)}, 6},
		{"the menu's picked run", nil, breaks, []Option{screen, pick([]string{"go"}, true)}, 1},
	} {
		var finished []int
		var code int
		withArgs(t, c.args, func() {
			code, _, _ = mainRun(t, tree(c.fn), append(c.opts, WithFinish(func(code int) { finished = append(finished, code) }))...)
		})
		if code != c.want || !slices.Equal(finished, []int{c.want}) {
			t.Errorf("%s: Main returned %d and finish got %v, want %d and one call with it", c.name, code, finished, c.want)
		}
	}
	var finished []int
	var code int
	withArgs(t, []string{"go"}, func() {
		_ = captureBoth(t, func() {
			code = Main(context.Background(), tree(ok), testBrand(), WithFinish(func(code int) { finished = append(finished, code) }))
		})
	})
	if code != 1 || !slices.Equal(finished, []int{1}) {
		t.Errorf("a session with no Endings: Main returned %d and finish got %v, want 1 and one call with it", code, finished)
	}
}

// Execute alone does not call finish.
//
// What it costs when this fails: a program that runs Execute and closes its own run closes it twice.
func TestExecuteDoesNotCallFinish(t *testing.T) { //nolint:paralleltest // serial: Execute sets the process's streams.
	called := false
	root := oneCommand(breaks)(nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{})
	err := Execute(context.Background(), root, New(append(House(), On(Terminal{}), Out(io.Discard), Err(io.Discard))...), testBrand(), WithFinish(func(int) { called = true }))
	if called || !errors.Is(err, errBroke) {
		t.Errorf("Execute called finish (%v) or lost the error (%v)", called, err)
	}
}

// An On inside WithSession wins over the terminal Main reads from the process.
//
// What it costs when this fails: a program whose width and colour come from its settings is drawn on the
// width Main read, and has to leave Main to be drawn right.
func TestMainTakesTheProgramsOwnTerminal(t *testing.T) { //nolint:paralleltest // serial: it sets the process's arguments and streams.
	width := 0
	withArgs(t, nil, func() {
		mainRun(t, func(s *Session) *cobra.Command {
			width = s.Terminal().Width
			return oneCommand(func(s *Session, _ *cobra.Command) error { return s.End(done()) })(s)
		}, WithSession(On(Terminal{Width: 33})))
	})
	if width != 33 {
		t.Errorf("newRoot's session is %d wide, want the program's 33", width)
	}
}

// What the handler writes goes through the tree's error stream, so --no-color strips its escapes.
//
// What it costs when this fails: a program's own failure line puts escape bytes in a log that asked for none.
func TestTheErrorHandlersWriterIsTheTreesErrorStream(t *testing.T) { //nolint:paralleltest // serial: it sets the process's arguments and streams.
	var errs string
	withArgs(t, []string{"--no-color"}, func() {
		_, _, errs = mainRun(t, oneCommand(breaks), WithErrorHandler(func(w io.Writer, _ error) (int, bool) {
			_, _ = io.WriteString(w, "\x1b[31mred\x1b[0m\n")
			return 7, true
		}))
	})
	if errs != "red\n" {
		t.Errorf("under --no-color the handler's line reached stderr as %q, want it stripped", errs)
	}
}

// Execute returns the error the tree returned, after a handler took it: handled changes the run's code and the
// words, never the error the caller of Execute reads.
//
// What it costs when this fails: a program that calls Execute and branches on its error reads nil for a run that
// failed, and goes on as if it had worked.
func TestExecuteReturnsTheErrorAfterAHandledOne(t *testing.T) {
	t.Parallel()
	calls := 0
	root := oneCommand(breaks)(nil)
	run := New(append(House(), On(Terminal{}), Out(io.Discard), Err(io.Discard))...)
	err := Execute(context.Background(), root, run, testBrand(), handler(&calls, 7, true))
	if !errors.Is(err, errBroke) || calls != 1 || run.ExitCode() != 7 {
		t.Errorf("Execute returned %v after %d calls with code %d; want the error it handled, 1 call and 7", err, calls, run.ExitCode())
	}
}

// On a held session a command that called End and then returned an error gets the handler's code, not End's: a
// held session keeps no ending, so the handler is given the error, and its code is the run's.
//
// What it costs when this fails: the order of the two changes unseen, and a program's own code is overridden by
// ui's, or ui's by the program's, depending on which a release happened to do last.
func TestOnAHeldSessionTheHandlersCodeWinsOverEnds(t *testing.T) { //nolint:paralleltest // serial: it sets the process's arguments and streams.
	endsThenFails := func(s *Session, _ *cobra.Command) error { _ = s.End(refused()); return errBroke }
	calls := 0
	var code int
	withArgs(t, nil, func() {
		code, _, _ = mainRun(t, oneCommand(endsThenFails), WithSession(Held()), handler(&calls, 7, true))
	})
	if code != 7 || calls != 1 {
		t.Errorf("a held session whose command ended refused (5) and returned an error: Main returned %d after %d calls, want the handler's 7 and 1 call", code, calls)
	}
}
