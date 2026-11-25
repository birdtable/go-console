package ui

// CLICOLOR_FORCE off a terminal: any value but empty or `0`
// keeps the colour on a pipe, NO_COLOR beats it unless the rules say the force
// wins, and the environment read is the one the caller hands Detect, never the
// process's.

import (
	"os"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

// pipe is a file that is not a terminal, as a capture or a log is.
func pipe(t *testing.T) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	return w
}

// What it costs when this fails: a person who sets CLICOLOR_FORCE=yes to keep the colour in a
// CI log gets none, or one who set NO_COLOR gets colour they asked not to have.
func TestCLIColorForceColoursAPipeAndNoColorWins(t *testing.T) {
	t.Parallel()
	w := pipe(t)
	n := 0
	for _, c := range []struct {
		env  []string
		rich bool
	}{
		{[]string{"CLICOLOR_FORCE=1"}, true},
		{[]string{"CLICOLOR_FORCE=yes"}, true},
		{[]string{"CLICOLOR_FORCE=true"}, true},
		{[]string{"CLICOLOR_FORCE=0"}, false},
		{[]string{"CLICOLOR_FORCE="}, false},
		{nil, false},
		{[]string{"CLICOLOR_FORCE=1", "NO_COLOR=1"}, false},
		{[]string{"CLICOLOR_FORCE=yes", "NO_COLOR=x"}, false},
		{[]string{"CLICOLOR_FORCE=1", "CLICOLOR_FORCE=0"}, false},
	} {
		got := Detect(nil, w, w, c.env, StandardEnv())
		if got.Colour != c.rich || got.ErrColour != c.rich {
			t.Errorf("%v on a pipe: colour %v and %v, want %v", c.env, got.Colour, got.ErrColour, c.rich)
		}
		n++
	}
	if n != 9 {
		t.Fatalf("ran %d cases, want 9", n)
	}
}

// Detect reads the environment it is handed, by the names its rules give, and
// never the process's: a force the process sets does not reach it, a force under
// another name does, and ForceWins gives the force the last word over NO_COLOR.
//
// What it costs when this fails: a program that hands Detect an environment of its
// own gets the process's colour, or one that names its own variables is ignored.
func TestDetectReadsTheEnvironmentItIsGivenByItsRules(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	w := pipe(t)
	if Detect(nil, w, w, nil, StandardEnv()).Colour {
		t.Error("the process's CLICOLOR_FORCE=1 reached Detect, which was handed no environment")
	}
	own := EnvRules{ForceColour: "APP_COLOUR", NoColour: "APP_PLAIN"}
	if !Detect(nil, w, w, []string{"APP_COLOUR=1"}, own).Colour {
		t.Error("a force under the program's own name did not colour the pipe")
	}
	if Detect(nil, w, w, []string{"CLICOLOR_FORCE=1"}, own).Colour {
		t.Error("CLICOLOR_FORCE coloured the pipe under rules that do not name it")
	}
	if Detect(nil, w, w, []string{"APP_COLOUR=1", "APP_PLAIN=1"}, own).Colour {
		t.Error("the program's own NO_COLOR did not win")
	}
	own.ForceWins = true
	if !Detect(nil, w, w, []string{"APP_COLOUR=1", "APP_PLAIN=1"}, own).Colour {
		t.Error("ForceWins did not give the force the last word")
	}
}

// A session's logger paints by the session's Terminal: true colour on an error
// stream that takes colour, none on one that does not, and none on a writer the
// caller gives.
//
// What it costs when this fails: a log line in a pipe carries escapes, or one at
// a terminal that takes colour is grey.
func TestASessionsLoggerPaintsByTheSessionsTerminal(t *testing.T) {
	t.Parallel()
	if p := New(On(Terminal{ErrColour: true})).loggerProfile(nil); p <= colorprofile.Ascii {
		t.Errorf("an error stream that takes colour gives the logger profile %v", p)
	}
	if p := New().loggerProfile(nil); p > colorprofile.Ascii {
		t.Errorf("a pipe gives the logger profile %v", p)
	}
	if p := New(On(Terminal{ErrColour: true})).loggerProfile(pipe(t)); p > colorprofile.Ascii {
		t.Errorf("a writer the caller gave gives the logger profile %v", p)
	}
}
