package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// With NoHelpLine a refusal or a failure that gives no line to type prints one line, its word and its
// sentence, and is kept with an empty Next, and Warn prints it with no line too. An ending that gives its
// own line prints and keeps it. Without the option the same lineless ending has the command's help.
//
// What it costs when this fails: a program whose contract is that such an ending has no line prints
// `--help` under it, or a program that never asked leaves a person stopped with nothing to type.
func TestNoHelpLineLeavesALinelessEndingWithNoLine(t *testing.T) {
	t.Parallel()
	lineless := func(c closing) closing { c.next = ""; return c }
	n := 0
	for _, e := range []closing{lineless(usage()), lineless(busy()), lineless(notReady()), lineless(refused()), lineless(failed()), refused(), failed()} {
		for _, on := range []bool{true, false} {
			var out, errb bytes.Buffer
			opts := []SessionOption{Out(&out), Err(&errb)}
			if on {
				opts = append(opts, NoHelpLine())
			}
			run := runner(opts...)
			_ = run.End(e)
			want := e.next
			if want == "" && !on {
				want = lineFor("", "", "--help")
			}
			wantLines := 2
			if want == "" {
				wantLines = 1
			}
			if got := run.Ended(); got == nil || got.Next() != want || got.Word() != e.word {
				t.Errorf("%s %q, option %v: kept %+v, want the line %q", e.word, e.next, on, got, want)
			}
			if got := len(strings.Split(strings.Trim(errb.String(), "\n"), "\n")); got != wantLines || out.Len() != 0 {
				t.Errorf("%s %q, option %v: printed %d lines, want %d: %q (stdout %q)", e.word, e.next, on, got, wantLines, errb.String(), out.String())
			}
			if has := strings.Contains(errb.String(), "--help"); has != (e.next == "" && !on) {
				t.Errorf("%s %q, option %v: the help line printed %v: %q", e.word, e.next, on, has, errb.String())
			}
			errb.Reset()
			run.Warn(e)
			if has := strings.Contains(errb.String(), "--help"); has != (e.next == "" && !on) || !strings.Contains(errb.String(), e.what) {
				t.Errorf("%s %q, option %v: Warn printed the help line %v: %q", e.word, e.next, on, has, errb.String())
			}
			n++
		}
	}
	if n != 14 {
		t.Fatalf("ran %d cases, want 14", n)
	}
}

// Under --json with JSONEndingsOnStderr the closing word is on stderr and stdout holds the one document
// and nothing else, with EndingsOnStdout and without it. Without the option the word prints nowhere under
// --json, as before; off --json the option moves nothing; and inside a Capture the word is drawn there.
//
// What it costs when this fails: a program whose contract puts the closing word on stderr under --json
// prints none, or a word lands on stdout and a script reads it as part of the document.
func TestJSONEndingsOnStderrPrintsTheWordBesideTheDocument(t *testing.T) { //nolint:paralleltest // serial: Execute sets the process's streams
	n := 0
	for _, e := range []closing{done(), refused(), failed()} {
		for _, on := range []bool{true, false} {
			for _, toStdout := range []bool{true, false} {
				var out, errb bytes.Buffer
				opts := []SessionOption{Out(&out), Err(&errb)}
				if on {
					opts = append(opts, JSONEndingsOnStderr())
				}
				if toStdout {
					opts = append(opts, EndingsOnStdout())
				}
				run := runner(opts...)
				root := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) { _ = run.End(e) }}
				root.SetOut(&out)
				root.SetErr(&errb)
				root.SetArgs([]string{"--json"})
				if err := Execute(context.Background(), root, run, testBrand()); err != nil {
					t.Fatal(err)
				}
				var doc Result
				dec := json.NewDecoder(&out)
				if err := dec.Decode(&doc); err != nil || doc.Word != e.word || strings.TrimSpace(readAll(t, dec)+out.String()) != "" {
					t.Errorf("%s, option %v, EndingsOnStdout %v: stdout is not the one document: %v, word %q", e.word, on, toStdout, err, doc.Word)
				}
				line := "  " + Column(e.word, 11) + e.what + "\n"
				if got := strings.Contains(errb.String(), line); got != on {
					t.Errorf("%s, option %v, EndingsOnStdout %v: the word on stderr %v: %q", e.word, on, toStdout, got, errb.String())
				}
				n++
			}
		}
	}
	if n != 12 {
		t.Fatalf("ran %d cases, want 12", n)
	}

	var out, errb, drawn bytes.Buffer
	off := runner(Out(&out), Err(&errb), JSONEndingsOnStderr())
	_ = off.End(done())
	_ = off.End(refused())
	if !strings.Contains(out.String(), done().what) || strings.Contains(out.String(), refused().what) || !strings.Contains(errb.String(), refused().what) || strings.Contains(errb.String(), done().what) {
		t.Errorf("off --json the option moved an ending: stdout %q, stderr %q", out.String(), errb.String())
	}
	out.Reset()
	errb.Reset()
	in := runner(Out(&out), Err(&errb), JSONEndingsOnStderr(), JSON())
	in.Capture(&drawn, func() { _ = in.End(refused()) })
	if !strings.Contains(drawn.String(), refused().what) || out.Len() != 0 || errb.Len() != 0 {
		t.Errorf("inside a Capture the word is drawn %q, stdout %q, stderr %q, want it in the Capture alone", drawn.String(), out.String(), errb.String())
	}
}

// readAll is what a decoder has read past its first document.
func readAll(t *testing.T, dec *json.Decoder) string {
	t.Helper()
	var b bytes.Buffer
	if _, err := b.ReadFrom(dec.Buffered()); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
