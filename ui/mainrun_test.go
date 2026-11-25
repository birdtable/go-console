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

// mainRun is Main on a tree with the given arguments, with what it wrote to the process's stdout
// and stderr.
func mainRun(t *testing.T, root func(*Session) *cobra.Command, opts ...Option) (code int, stdout, stderr string) {
	t.Helper()
	stdout = capture(t, &os.Stdout, func() {
		stderr = capture(t, &os.Stderr, func() {
			code = Main(context.Background(), root, testBrand(), append([]Option{WithSession(House()...)}, opts...)...)
		})
	})
	return code, stdout, stderr
}

// oneCommand is a root command named app that runs fn and is given args.
func oneCommand(fn func(*Session, *cobra.Command) error, args ...string) func(*Session) *cobra.Command {
	return func(s *Session) *cobra.Command {
		root := &cobra.Command{Use: "app", SilenceUsage: true, SilenceErrors: true}
		root.RunE = func(c *cobra.Command, _ []string) error { return fn(s, c) }
		root.SetArgs(args)
		return root
	}
}

// A command that ends itself "refused" makes Main return exit's 5, and one that ends done, 0.
//
// What it costs when this fails: a program's exit status is not what its ending said, so a script
// that branches on 5 reads a refusal as success or as a crash.
func TestMainReturnsTheCodeOfTheEnding(t *testing.T) { //nolint:paralleltest // serial: it sets the process's streams.
	// serial: it sets the process's streams.
	n := 0
	for _, c := range []struct {
		name string
		fn   func(*Session, *cobra.Command) error
		want int
	}{
		{"done", func(s *Session, _ *cobra.Command) error { return s.End(done()) }, 0},
		{"refused", func(s *Session, _ *cobra.Command) error {
			return s.End(refused())
		}, 5},
		{"an error with no ending", func(*Session, *cobra.Command) error { return errors.New("it broke") }, 1},
	} {
		got, _, _ := mainRun(t, oneCommand(c.fn))
		if got != c.want {
			t.Errorf("%s: Main returned %d, want %d", c.name, got, c.want)
		}
		n++
	}
	if n != 3 {
		t.Fatalf("ran %d cases, want 3", n)
	}
}

// A panic in a command makes Main return 1 and, under --json, print one document.
//
// What it costs when this fails: a crash exits with Go's 2, which exit reads as a usage mistake,
// or a script reading --json gets a stack trace in place of its document.
func TestMainReturnsOneForAPanicAndPrintsOneDocument(t *testing.T) { //nolint:paralleltest // serial: it sets the process's streams.
	// serial: it sets the process's streams.
	code, out, _ := mainRun(t, oneCommand(func(*Session, *cobra.Command) error { panic("boom") }, "--json"))
	if code != 1 {
		t.Errorf("a panic made Main return %d, want 1", code)
	}
	if got := documents(out); got != 1 {
		t.Errorf("stdout holds %d documents, want 1: %q", got, out)
	}
}

// With CI set the look is plain: no escape byte on either stream, and no question is asked.
//
// What it costs when this fails: a build log holds escape bytes, or a pipeline waits on a prompt
// nobody answers.
func TestMainReadsCIAsNoColourAndAsksNothing(t *testing.T) {
	// serial: it sets the process's streams and the environment.
	t.Setenv("CI", "1")
	t.Setenv("CLICOLOR_FORCE", "1")
	var asked bool
	code, out, errs := mainRun(t, oneCommand(func(s *Session, _ *cobra.Command) error {
		s.Info("a line")
		asked = s.CanAsk()
		return nil
	}), WithBanner())
	if code != 0 || asked {
		t.Errorf("Main returned %d, and could ask: %v; want 0 and false", code, asked)
	}
	if strings.ContainsRune(out+errs, '\x1b') {
		t.Errorf("CI=1 drew an escape byte: %q %q", out, errs)
	}
	if strings.Contains(out, wordmarkOf(testBrand().Name)[0]) {
		t.Errorf("CI=1 drew a banner: %q", out)
	}
}

// Detect reads CI in one place, StandardEnv's rules: set means no colour even when colour is forced
// and no question; unset, "0" and "false" mean neither.
//
// What it costs when this fails: a CI run paints, or a developer with CI=0 is told no.
func TestDetectReadsCIOnlyThroughTheRules(t *testing.T) {
	t.Parallel()
	n := 0
	for _, c := range []struct {
		env  []string
		rule EnvRules
		want bool
	}{
		{[]string{"CI=1", "CLICOLOR_FORCE=1"}, StandardEnv(), false},
		{[]string{"CI=true", "CLICOLOR_FORCE=1"}, StandardEnv(), false},
		{[]string{"CI=0", "CLICOLOR_FORCE=1"}, StandardEnv(), true},
		{[]string{"CI=false", "CLICOLOR_FORCE=1"}, StandardEnv(), true},
		{[]string{"CLICOLOR_FORCE=1"}, StandardEnv(), true},
		{[]string{"CI=1", "CLICOLOR_FORCE=1"}, EnvRules{ForceColour: StandardEnv().ForceColour}, true},
	} {
		var out, errb bytes.Buffer
		got := Detect(nil, &out, &errb, c.env, c.rule)
		if got.Colour != c.want || got.ErrColour != c.want || got.Interactive {
			t.Errorf("%v: Colour %v ErrColour %v Interactive %v, want colour %v and no question", c.env, got.Colour, got.ErrColour, got.Interactive, c.want)
		}
		n++
	}
	if n != 6 {
		t.Fatalf("ran %d cases, want 6", n)
	}
}

// The banner is drawn after the flags are read, so with --json the first byte of stdout is the
// document's brace and with no flag the banner comes first.
//
// What it costs when this fails: `tool status --json | jq` reads a banner as its first byte.
func TestTheBannerNeverPrecedesAJSONDocument(t *testing.T) { //nolint:paralleltest // serial: it sets the process's streams.
	// serial: it sets the process's streams.
	scr := screenOpt(true, 0)
	drawn := func(args ...string) (banner, doc string) {
		root := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) {}}
		root.SetArgs(args)
		run := runner(scr)
		var out bytes.Buffer
		doc = capture(t, &os.Stdout, func() {
			run.Capture(&out, func() {
				if err := Execute(context.Background(), root, run, testBrand(), WithBanner()); err != nil {
					t.Errorf("Execute: %v", err)
				}
			})
		})
		return out.String(), doc
	}
	mark := wordmarkOf(testBrand().Name)[0]
	if banner, doc := drawn("--json"); strings.Contains(banner, mark) || !strings.HasPrefix(doc, "{") {
		t.Errorf("with --json the banner was %q and stdout started with %q, want no banner and the document's {", banner, doc[:min(len(doc), 20)])
	}
	if banner, _ := drawn(); !strings.Contains(banner, mark) {
		t.Errorf("with no flag the banner was not drawn: %q", banner)
	}
}

// The stub endings of this package's tests still pass: the look reads only the four methods.
//
// What it costs when this fails: a program's own ending type must grow methods ui does not
// document.
func TestAStubEndingStillEndsAMainRun(t *testing.T) { //nolint:paralleltest // serial: it sets the process's streams.
	// serial: it sets the process's streams.
	code, _, _ := mainRun(t, oneCommand(func(s *Session, _ *cobra.Command) error {
		return s.End(declined())
	}))
	if want := declined().code; code != want {
		t.Errorf("a stub ending of code %d made Main return %d", want, code)
	}
}
