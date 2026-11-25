package ui

// One rule for colour, read where output is written (issue 26): NO_COLOR, any
// value, beats CLICOLOR_FORCE, and TERM=dumb is plain, for help and for
// everything else, even when the variable is set after start-up.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

// helpText runs a small command tree's --help through Execute and returns what was
// written, the way a program's main would get it.
func helpText(t *testing.T, opts ...SessionOption) string {
	t.Helper()
	root := &cobra.Command{Use: "app", Short: "does a thing", Run: func(*cobra.Command, []string) {}}
	root.Flags().Bool("loud", false, "make noise")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--help"})
	if err := Execute(context.Background(), root, runner(opts...), Brand{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "loud") {
		t.Fatalf("the help did not list the flag:\n%q", out.String())
	}
	return out.String()
}

func TestNoColorBeatsForce(t *testing.T) {
	// serial: fang reads the process's CLICOLOR_FORCE to paint the help, and the
	// session's Terminal strips what it paints.
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("NO_COLOR", "")
	forced := On(detectedTerm("TERM=xterm-256color", "CLICOLOR_FORCE=1"))
	if got := helpText(t, forced); !strings.Contains(got, "\x1b[") {
		t.Fatalf("CLICOLOR_FORCE alone gave plain help, so the test below proves nothing:\n%q", got)
	}
	noColour := On(detectedTerm("TERM=xterm-256color", "CLICOLOR_FORCE=1", "NO_COLOR=1"))
	if got := helpText(t, noColour); strings.Contains(got, "\x1b[") {
		t.Errorf("NO_COLOR=1 CLICOLOR_FORCE=1 and the help was coloured:\n%q", got)
	}
	if got := New(noColour).Bad("x"); got != "x" {
		t.Errorf("Bad under NO_COLOR=1 CLICOLOR_FORCE=1 = %q", got)
	}
}

// TERM=dumb is plain everywhere: the lines, the markup, the logger and the help.
//
// What it costs when this fails: a terminal that cannot show colour fills with
// escape codes a person has to read past.
func TestADumbTerminalIsPlain(t *testing.T) {
	// serial: fang reads the process's CLICOLOR_FORCE to paint the help.
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("NO_COLOR", "")
	if got := New(On(detectedTerm("TERM=xterm-256color", "CLICOLOR_FORCE=1"))).Bad("x"); got == "x" {
		t.Fatalf("a rich stream was plain with nothing asking: %q", got)
	}
	dumb := New(On(detectedTerm("TERM=dumb", "CLICOLOR_FORCE=1")))
	if got := dumb.Bad("x"); got != "x" {
		t.Errorf("Bad under TERM=dumb = %q", got)
	}
	if got := dumb.markupOn(dumb.outStream(), "run `app`"); got != "run `app`" {
		t.Errorf("markup under TERM=dumb = %q", got)
	}
	errTerm := dumb.errStream()
	errTerm.interactive = true // stderr as a terminal would be: the logger's face needs one
	if loggerIsStyled(nil, errTerm, false, false) {
		t.Error("the logger kept its styled face under TERM=dumb")
	}
	if got := helpText(t, On(detectedTerm("TERM=dumb", "CLICOLOR_FORCE=1"))); strings.Contains(got, "\x1b[") {
		t.Errorf("help under TERM=dumb CLICOLOR_FORCE=1 was coloured:\n%q", got)
	}
}

// failing runs a command that returns an error through Execute and returns
// the ending the run printed, on stderr, as every ending that did not get
// there is. fang's own error page is gone, so nothing reaches
// the command's error stream.
func failing(t *testing.T, opts ...SessionOption) string {
	t.Helper()
	root := &cobra.Command{Use: "app", RunE: func(*cobra.Command, []string) error { return errors.New("it broke") }}
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(nil)
	var err error
	ending := capture(t, &os.Stderr, func() {
		err = Execute(context.Background(), root, runner(opts...), Brand{})
	})
	if err == nil {
		t.Fatal("the command's error was not returned")
	}
	if !strings.Contains(ending, "broke") || errOut.Len() != 0 {
		t.Fatalf("the failure did not end the run on stderr alone: ending %q, the command's error stream %q", ending, errOut.String())
	}
	return ending
}

// A failed run's ending, on a stderr that paints, is plain under NO_COLOR,
// even with CLICOLOR_FORCE set, and painted without it.
//
// What it costs when this fails: a run under NO_COLOR prints escapes at the
// one line a person reads first, and a screen reader reads them out.
func TestAFailedRunsEndingIsPlainUnderNoColorBeatingForce(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it reads the process's stderr.
	if got := failing(t, On(detectedTerm("TERM=xterm-256color", "CLICOLOR_FORCE=1"))); !strings.Contains(got, "\x1b[") {
		t.Fatalf("a terminal that paints gave a plain ending, so the check below proves nothing:\n%q", got)
	}
	if got := failing(t, On(detectedTerm("TERM=xterm-256color", "CLICOLOR_FORCE=1", "NO_COLOR=1"))); strings.Contains(got, "\x1b[") {
		t.Errorf("NO_COLOR=1 CLICOLOR_FORCE=1 and the ending was coloured:\n%q", got)
	}
}

// jsonOut runs a command that writes line to its standard output through
// Execute, at COLUMNS=40 off a terminal, with a session that answers in JSON
// or not, and returns what came out.
func jsonOut(t *testing.T, line string, json bool) string {
	t.Helper()
	t.Setenv("COLUMNS", "40")
	run := runner()
	if json {
		run = runner(JSON())
	}
	root := &cobra.Command{Use: "app", Run: func(c *cobra.Command, _ []string) {
		_, _ = c.OutOrStdout().Write([]byte(line + "\n"))
		// The command prints its own document, so Execute adds no answer.
		run.MarkPrinted()
	}}
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(nil)
	if err := Execute(context.Background(), root, run, Brand{}); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// A 500-byte JSON line at COLUMNS=40 comes back as one line that parses, under
// --json and when the program never set JSON (issue 50).
//
// What it costs when this fails: `version --json` is not JSON once a line passes
// the width, and a script that reads it breaks.
func TestJSONOutputIsNeverWrapped(t *testing.T) { //nolint:paralleltest // serial: it sets the process's environment or directory
	doc := `{"version":"` + strings.Repeat("v", 480) + `","ok":true}`
	if len(doc) < 500 {
		t.Fatalf("the planted line is %d bytes", len(doc))
	}
	for _, on := range []bool{true, false} {
		got := jsonOut(t, doc, on)
		var v map[string]any
		if err := json.Unmarshal([]byte(got), &v); err != nil || strings.Count(strings.TrimRight(got, "\n"), "\n") != 0 {
			t.Errorf("JSON=%v: the document was changed (%v): %q", on, err, got)
		}
	}
}

// rawRun runs a command that writes payload through write, under Execute at
// COLUMNS=40 with NO_COLOR set, and returns what reached standard output and
// what reached standard error, kept in two buffers.
func rawRun(t *testing.T, payload []byte, write func(c *cobra.Command, b []byte)) (stdout, stderr []byte) {
	t.Helper()
	t.Setenv("COLUMNS", "40")
	t.Setenv("NO_COLOR", "1")
	root := &cobra.Command{Use: "app"}
	sub := &cobra.Command{Use: "connect", Run: func(c *cobra.Command, _ []string) { write(c, payload) }}
	root.AddCommand(sub)
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"connect"})
	if err := Execute(context.Background(), root, runner(), Brand{}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes(), errb.Bytes()
}

// 1 MiB of random bytes, long lines and escape bytes included, written to
// ui.Raw come back byte for byte (issue 52).
//
// What it costs when this fails: ssh's key exchange, a file copy or a remote
// command's output is changed on its way through.
func TestRawBytesPassExecuteUnchanged(t *testing.T) { //nolint:paralleltest // serial: it sets the process's environment or directory
	payload := make([]byte, 1<<20)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}
	payload = append(payload, []byte("\x1b[31mred\x1b[0m "+strings.Repeat("long ", 100)+"\n")...)
	other := append([]byte("\x1b[2Jstderr "+strings.Repeat("e", 200)), payload[:4096]...)
	got, gotErr := rawRun(t, payload, func(c *cobra.Command, b []byte) {
		out, errw := Raw(c)
		if _, err := out.Write(b); err != nil {
			t.Error(err)
		}
		if _, err := errw.Write(other); err != nil {
			t.Error(err)
		}
	})
	if !bytes.Equal(got, payload) {
		t.Errorf("the bytes changed: wrote %d, got %d", len(payload), len(got))
	}
	// Each stream gets its own bytes: stderr's are not on stdout, nor the reverse.
	if !bytes.Equal(gotErr, other) {
		t.Errorf("stderr's bytes changed or went elsewhere: wrote %d, got %d", len(other), len(gotErr))
	}
	// Words still go through the stripping writer.
	words, _ := rawRun(t, []byte("\x1b[31mred\x1b[0m word\n"), func(c *cobra.Command, b []byte) {
		_, _ = c.OutOrStdout().Write(b)
	})
	if string(words) != "red word\n" {
		t.Errorf("words were not stripped under NO_COLOR: %q", words)
	}
}

// The negative half, the planted twin: the same bytes written to
// cmd.OutOrStdout() are changed, so the test above is able to fail.
func TestBytesWrittenToTheWrappedWriterAreChanged(t *testing.T) { //nolint:paralleltest // serial: it sets the process's environment or directory
	payload := []byte("\x1b[31m" + strings.Repeat("long ", 100) + "\x1b[0m\n")
	got, _ := rawRun(t, payload, func(c *cobra.Command, b []byte) { _, _ = c.OutOrStdout().Write(b) })
	if bytes.Equal(got, payload) {
		t.Error("the wrapped writer left the bytes alone, so the raw test proves nothing")
	}
}

// Outside Execute, Raw is the command's own streams.
func TestRawOutsideExecuteIsTheCommandsOwn(t *testing.T) {
	var o, e bytes.Buffer
	c := &cobra.Command{Use: "app"}
	c.SetOut(&o)
	c.SetErr(&e)
	out, errw := Raw(c)
	if out != io.Writer(&o) || errw != io.Writer(&e) {
		t.Errorf("Raw outside Execute is not the command's streams")
	}
	// After Execute returns, Raw is the command's streams as they are then, not
	// the ones Execute found.
	t.Setenv("NO_COLOR", "1")
	var first, firstErr, later, laterErr bytes.Buffer
	root := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) {}}
	root.SetOut(&first)
	root.SetErr(&firstErr)
	root.SetArgs(nil)
	if err := Execute(context.Background(), root, runner(), Brand{}); err != nil {
		t.Fatal(err)
	}
	root.SetOut(&later)
	root.SetErr(&laterErr)
	out, errw = Raw(root)
	if out != io.Writer(&later) || errw != io.Writer(&laterErr) {
		t.Errorf("after Execute, Raw still returns the streams Execute found")
	}
}

// What a command and fang are handed as the streams under Execute is still the
// file's descriptor, where the stream was a file: fang reads the terminal's
// width from it and switches the Windows console's escapes on only for a
// stream that has one. A stream that was not a file, as a buffer, is not given
// one.
//
// What it costs when this fails: the help colours never reach a real terminal
// and a Windows console prints raw escapes.
func TestExecuteKeepsTheDescriptorOfAFile(t *testing.T) {
	t.Parallel()
	type described = term.File
	file, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	var buf bytes.Buffer
	for _, c := range []struct {
		name    string
		out     io.Writer
		hasFile bool
	}{{"a file", file, true}, {"a buffer", &buf, false}} {
		var gotOut, gotErr io.Writer
		root := &cobra.Command{Use: "app", Run: func(c *cobra.Command, _ []string) {
			gotOut, gotErr = c.OutOrStdout(), c.ErrOrStderr()
		}}
		root.SetOut(c.out)
		root.SetErr(c.out)
		root.SetArgs(nil)
		if err := Execute(context.Background(), root, runner(), Brand{}); err != nil {
			t.Fatal(err)
		}
		for _, w := range []io.Writer{gotOut, gotErr} {
			if _, ok := w.(described); ok != c.hasFile {
				t.Errorf("%s: the stream a command is handed has a descriptor = %v, want %v", c.name, ok, c.hasFile)
			}
		}
	}
}
