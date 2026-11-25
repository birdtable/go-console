package ui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// executeCaught runs root under Execute with args and run, off a terminal,
// so with stdout a pipe, and returns what the run printed on the process's
// stdout and on its stderr, what fang and cobra wrote to the command's error
// stream, and what Execute returned.
func executeCaught(t *testing.T, root *cobra.Command, run *Session, args ...string) (stdout, stderr, cobraErr string, err error) {
	t.Helper()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	stdout = capture(t, &os.Stdout, func() {
		stderr = capture(t, &os.Stderr, func() {
			err = Execute(context.Background(), root, run, testBrand())
		})
	})
	return stdout, stderr, errb.String(), err
}

// appAndGo is a root, app, with one command below it, go, which takes no
// argument, and whose run is runE.
func appAndGo(runE func(*cobra.Command, []string) error) *cobra.Command {
	root := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(&cobra.Command{Use: "go", Args: cobra.NoArgs, RunE: runE})
	return root
}

// Every error a run meets under Execute ends it on an ending with a line to
// type, the help of the command that failed, and comes back to the caller: a
// command line not understood, before any command ran, ends on the usage
// refusal; an error a command or its pre-run returned ends on the failure.
// The ending prints on stderr, so with stdout a pipe nothing of it goes down
// the pipe, and fang prints nothing of its own beside it.
//
// What it costs when this fails: a person who typed a command wrong, or whose
// command failed, reads a bare error with nothing to type next, or reads it
// twice in two looks, and a script reads no closing word.
func TestEveryErrorUnderExecuteEndsOnAnEndingWithTheFailingCommandsHelp(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it sets the process's streams.
	boom := errors.New("it broke")
	n := 0
	for _, c := range []struct {
		name   string
		root   func() *cobra.Command
		args   []string
		word   string
		code   int
		next   string
		isBoom bool
	}{
		{"a command's error", func() *cobra.Command {
			return appAndGo(func(*cobra.Command, []string) error { return boom })
		}, []string{"go"}, "failed", 1, "app go --help", true},
		{"the error of a command with no argument check", func() *cobra.Command {
			root := &cobra.Command{Use: "app"}
			root.AddCommand(&cobra.Command{Use: "go", RunE: func(*cobra.Command, []string) error { return boom }})
			return root
		}, []string{"go"}, "failed", 1, "app go --help", true},
		{"a pre-run's error", func() *cobra.Command {
			root := appAndGo(func(*cobra.Command, []string) error { return nil })
			root.PersistentPreRunE = func(*cobra.Command, []string) error { return boom }
			return root
		}, []string{"go"}, "failed", 1, "app go --help", true},
		{"an unknown flag", func() *cobra.Command {
			return appAndGo(func(*cobra.Command, []string) error { return nil })
		}, []string{"go", "--nope"}, "refused", 2, "app go --help", false},
		{"an argument the command takes none of", func() *cobra.Command {
			return appAndGo(func(*cobra.Command, []string) error { return nil })
		}, []string{"go", "there"}, "refused", 2, "app go --help", false},
		{"an unknown command", func() *cobra.Command {
			root := &cobra.Command{Use: "app"}
			root.AddCommand(&cobra.Command{Use: "go", Run: func(*cobra.Command, []string) {}})
			return root
		}, []string{"nope"}, "refused", 2, "app --help", false},
	} {
		run := runner()
		stdout, stderr, cobraErr, err := executeCaught(t, c.root(), run, c.args...)
		if err == nil || c.isBoom != errors.Is(err, boom) {
			t.Errorf("%s: Execute returned %v, want an error, %v itself: %v", c.name, err, boom, c.isBoom)
		}
		e := run.Ended()
		if e == nil || e.Word() != c.word || e.Code() != c.code || e.Next() != c.next || err == nil || e.What() != err.Error() {
			t.Errorf("%s: the run ended on %+v, want %s, code %d, with %q and the error's words", c.name, e, c.word, c.code, c.next)
		}
		if !strings.Contains(stderr, c.word) || !strings.Contains(stderr, c.next) || stdout != "" {
			t.Errorf("%s: printed %q on stderr and %q on stdout, want the word and the line on stderr alone", c.name, stderr, stdout)
		}
		if cobraErr != "" {
			t.Errorf("%s: fang or cobra printed %q of its own", c.name, cobraErr)
		}
		n++
	}
	if n != 6 {
		t.Fatalf("ran %d command lines, want 6", n)
	}
}

// The negative halves: a command that succeeds returns nil and ends on
// nothing Execute chose, and a command that ended the run itself and
// returned an error keeps its own ending, printed once.
//
// What it costs when this fails: a run that went well prints a failure, or a
// command's own closing word is followed by a second one.
func TestExecuteEndsOnlyARunThatFailedAndDidNotEndItself(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it sets the process's streams.
	run := runner()
	stdout, stderr, _, err := executeCaught(t, appAndGo(func(*cobra.Command, []string) error { return nil }), run, "go")
	if err != nil || run.Ended() != nil || stdout+stderr != "" {
		t.Errorf("a command that succeeded returned %v, ended on %v and printed %q", err, run.Ended(), stdout+stderr)
	}
	run = runner()
	stdout, stderr, _, err = executeCaught(t, appAndGo(func(*cobra.Command, []string) error {
		run.End(refused())
		return errors.New("stopped")
	}), run, "go")
	if err == nil || run.Ended() != refused() || strings.Count(stdout+stderr, "refused") != 1 || strings.Contains(stdout+stderr, "--help") {
		t.Errorf("a command that ended itself returned %v, ended on %v and printed %q, want its own ending once", err, run.Ended(), stdout+stderr)
	}
}

// A session made with no Endings is refused by Execute before anything runs, and says what to give,
// since there is no default table: a program's codes and words are its own.
//
// What it costs when this fails: a program that gave no table runs with endings of a code and a
// word nobody chose.
func TestASessionWithNoEndingsIsRefusedByExecute(t *testing.T) { //nolint:paralleltest // serial: it sets the process's streams.
	ran := false
	root := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) { ran = true }}
	_, _, _, err := executeCaught(t, root, New())
	if err == nil || !strings.Contains(err.Error(), "Endings") || ran {
		t.Errorf("Execute with no Endings returned %v and ran %v, want a refusal naming Endings", err, ran)
	}
}

// ui's own endings always give a line to type, so End's fallback to the
// help catches only a program's: every ending ui builds from the session's table, from
// Execute's errors and from a question that could not be asked, is built with a next line that
// is not empty.
//
// What it costs when this fails: one of ui's refusals ships with no line of
// its own and the help hides it, so the person is sent to read a page where
// one line would do.
func TestUisOwnEndingsNeverRelyOnTheHelpFallback(t *testing.T) {
	t.Parallel()
	s := runner()
	root := appAndGo(func(*cobra.Command, []string) error { return nil })
	built := []Ending{
		endingFor(s, false, root, errors.New("bad flag")),
		endingFor(s, true, root, errors.New("it broke")),
	}
	_, e := s.Confirm("delete box", "", "--yes")
	built = append(built, e)
	_, e = s.ConfirmName("delete box", "box", "--name")
	built = append(built, e)
	for _, flag := range []string{"--yes", ""} {
		built = append(built, s.couldNotAsk("delete box", flag, errors.New("no tty")))
	}
	for i, e := range built {
		if e == nil || strings.TrimSpace(e.Next()) == "" {
			t.Errorf("ending %d is %v, want one with a line to type", i, e)
		}
	}
}

// The line to type that End and Warn fill under Execute is the reached
// command's path with --help, and outside Execute the program's name with
// it: a value typed after the command, as a token can be, is never in it,
// on stderr, in the ending the run log keeps, or in the Result.
//
// What it costs when this fails: `app login <token>` that fails prints the
// token back on the screen, into the scheduler's log and into the --json
// document a script stores.
func TestTheHelpLineNamesTheCommandAndNoWordTypedAfterIt(t *testing.T) { //nolint:paralleltest // serial: it sets the process's streams or arguments
	// serial: it sets the process's arguments and streams.
	const planted = "PLANTED-TOKEN-VALUE"
	wasArgs := os.Args
	t.Cleanup(func() { os.Args = wasArgs })
	os.Args = []string{"/usr/bin/app", "login", planted}
	lineless := closing{word: "failed", what: "could not log in", code: 1}
	n := 0
	for _, c := range []struct {
		name string
		args cobra.PositionalArgs
		body func(run *Session)
	}{
		{"End", cobra.ArbitraryArgs, func(run *Session) { run.End(lineless) }},
		{"Warn", cobra.ArbitraryArgs, func(run *Session) { run.Warn(lineless); run.End(failed()) }},
		// No argument check, so the command's run alone records its path.
		{"End with no argument check", nil, func(run *Session) { run.End(lineless) }},
	} {
		run := runner()
		root := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) {}}
		root.AddCommand(&cobra.Command{Use: "login", Args: c.args, Run: func(*cobra.Command, []string) { c.body(run) }})
		_, stderr, _, _ := executeCaught(t, root, run, "login", planted)
		ended := run.Ended()
		if !strings.Contains(stderr, "app login --help") || strings.Contains(stderr, planted) {
			t.Errorf("%s: stderr printed %q, want app login --help and not the typed word", c.name, stderr)
		}
		if ended == nil || strings.Contains(ended.Next(), planted) || strings.Contains(run.Result(nil).Next, planted) {
			t.Errorf("%s: the run kept %v and answered %q", c.name, ended, run.Result(nil).Next)
		}
		n++
	}
	if n != 3 {
		t.Fatalf("ran %d commands, want 3", n)
	}
	// Outside Execute there is no command's path: the program's name.
	stderr := capture(t, &os.Stderr, func() { runner(Program("app")).End(lineless) })
	if !strings.Contains(stderr, "    app --help\n") || strings.Contains(stderr, planted) {
		t.Errorf("outside Execute a failure printed %q, want app --help and not the typed word", stderr)
	}
}
