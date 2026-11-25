package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dustin/go-humanize/english"
)

// A line the call site already painted keeps its colour all the way along.
//
// Emphasis does not nest. Bolding a path inside a dimmed sentence ends the
// dimming where the path ends, and the rest of the line comes back at full
// strength, which reads as two sentences on one row.
func TestMarkupLeavesTextThatWasAlreadyPaintedAlone(t *testing.T) {
	t.Parallel()
	plain := `look in D:\Files for the rest`
	painted := "\x1b[2m" + plain + "\x1b[m"

	got := onPlainRuns(painted, func(string) string { return "TOUCHED" })
	if got != painted {
		t.Errorf("painted text was marked up:\n got %q\nwant %q", got, painted)
	}

	// The other half of the rule. A painted prefix and a plain tail is what
	// every `doctor` row is, and the path in the tail still gets marked up.
	row := "\x1b[32mok\x1b[m  app  " + plain
	if got := onPlainRuns(row, func(string) string { return "TOUCHED" }); !strings.Contains(got, "TOUCHED") {
		t.Errorf("the plain part of a row was skipped too: %q", got)
	}
	if !strings.Contains(onPlainRuns(row, func(s string) string { return s }), "\x1b[32mok\x1b[m") {
		t.Error("the painted prefix did not survive")
	}
}

// notATerminal is a real file, which is what a redirect, a pipe and a log all
// look like to the detection.
func notATerminal(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// Colour off a terminal, when it was asked for, and off when it was not.
//
// CLICOLOR_FORCE is the convention for "I am capturing this, keep the colour",
// which is how the screenshots in the README get made. The detection used to
// return before it read the variable whenever the stream was not a terminal,
// which is the only situation the variable is ever set in.
func TestColourIsKeptWhenAskedForOffATerminal(t *testing.T) {
	t.Parallel()
	f := notATerminal(t)
	if !Detect(nil, f, f, []string{"TERM=xterm-256color", "CLICOLOR_FORCE=1"}, StandardEnv()).Colour {
		t.Error("CLICOLOR_FORCE=1 and the output came back plain")
	}
	if Detect(nil, f, f, []string{"TERM=xterm-256color"}, StandardEnv()).Colour {
		t.Error("a redirect nobody asked to colour came back with escapes in it")
	}
}

// NO_COLOR wins. Somebody who has set it has said so about every tool on the
// machine, and a screenshot is not a reason to override them.
func TestNoColourIsHonouredEvenWhenColourIsForced(t *testing.T) {
	t.Parallel()
	f := notATerminal(t)
	if Detect(nil, f, f, []string{"TERM=xterm-256color", "CLICOLOR_FORCE=1", "NO_COLOR=1"}, StandardEnv()).Colour {
		t.Error("NO_COLOR was set and the output still came back coloured")
	}
}

// A stream that is not a terminal is never interactive, whatever the colour
// says. The two questions are separate: a pipe into a pager takes colour and
// cannot answer a question.
func TestForcedColourDoesNotMakeAPipeAskable(t *testing.T) {
	t.Parallel()
	f := notATerminal(t)
	if Detect(f, f, f, []string{"CLICOLOR_FORCE=1"}, StandardEnv()).Interactive {
		t.Error("a file was treated as something that could answer a question")
	}
}

// The count and its noun that the table note and the progress line print are go-humanize's english.Plural, which prints what ui.Plural printed: the same for -1, 0, 1, 2 and a large count, with
// the commas, and the singular for 1 alone.
func TestPluralReadsLargeCounts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		n    int
		want string
	}{
		{-1, "-1 files"}, {0, "0 files"}, {1, "1 file"}, {2, "2 files"}, {999, "999 files"}, {1000, "1,000 files"},
		{165440, "165,440 files"}, {1234567, "1,234,567 files"},
	} {
		if got := english.Plural(tc.n, "file", "files"); got != tc.want {
			t.Errorf("english.Plural(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
