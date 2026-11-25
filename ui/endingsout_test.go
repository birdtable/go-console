package ui

import (
	"bytes"
	"strings"
	"testing"
)

// With EndingsOnStdout a refused, failed and busy ending prints on stdout and nothing reaches stderr; without
// it the same ending prints on stderr and nothing on stdout, as before.
//
// What it costs when this fails: a program whose contract puts its closing word on stdout has it on stderr, or
// a program that never asked has its failures moved off the stream `app | grep` leaves alone.
func TestEndingsOnStdoutMovesEveryClosingWord(t *testing.T) {
	t.Parallel()
	n := 0
	for _, e := range []closing{refused(), failed(), busy()} {
		for _, on := range []bool{true, false} {
			var out, errb bytes.Buffer
			opts := []SessionOption{Out(&out), Err(&errb)}
			if on {
				opts = append(opts, EndingsOnStdout())
			}
			run := runner(opts...)
			_ = run.End(e)
			gotOut, gotErr := strings.Contains(out.String(), e.word), strings.Contains(errb.String(), e.word)
			if on && (!gotOut || errb.Len() != 0) {
				t.Errorf("%s with the option: stdout %q, stderr %q, want the word on stdout and nothing on stderr", e.word, out.String(), errb.String())
			}
			if !on && (gotOut || !gotErr) {
				t.Errorf("%s without the option: stdout %q, stderr %q, want the word on stderr alone", e.word, out.String(), errb.String())
			}
			n++
		}
	}
	if n != 6 {
		t.Fatalf("ran %d cases, want 6", n)
	}
}
