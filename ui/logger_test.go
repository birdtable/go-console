package ui

import (
	"bytes"
	"image/color"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"charm.land/log/v2"
	"github.com/charmbracelet/colorprofile"
)

func fixedNow() time.Time { return time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC) }

// Off a terminal a line is logfmt, one to a line, with the time: what the
// journal and a pipe read.
func TestLogIsLogfmtOffATerminal(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := newLogger(&buf, "app", false, fixedNow, colorprofile.NoTTY, todaysStream())
	l.Info("started", "port", 80)
	l.Warn("slow", "who", "a b")
	got := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	want := []string{
		`time=2026-10-31T12:00:00Z level=info prefix=app msg=started port=80`,
		`time=2026-10-31T12:00:00Z level=warn prefix=app msg=slow who="a b"`,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines:\n%s", len(got), buf.String())
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d:\n got %s\nwant %s", i, got[i], want[i])
		}
	}
	// A level below the logger's own is not written.
	buf.Reset()
	l.Debug("quiet")
	if buf.Len() != 0 {
		t.Errorf("debug was written: %q", buf.String())
	}
}

// The real entry point picks logfmt when it is given a writer of its own, and
// when the test's standard error is not a terminal.
func TestNewLoggerWritesLogfmtToAWriter(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	New().NewLogger(&buf, "app").Error("failed", "err", "no")
	if !strings.Contains(buf.String(), "level=error") || !strings.Contains(buf.String(), "prefix=app") {
		t.Errorf("%q", buf.String())
	}
}

// At a terminal a line is the level word and the message, in the package's
// colours, with no key=value for the level and no timestamp.
func TestLogAtATerminalIsStyled(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := newLogger(&buf, "app", true, fixedNow, colorprofile.NoTTY, todaysStream())
	l.Info("started", "port", 80)
	got := buf.String()
	for _, want := range []string{"INFO", "app", "started", "port"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q is missing from %q", want, got)
		}
	}
	if strings.Contains(got, "level=") || strings.Contains(got, "2026") {
		t.Errorf("the styled face has logfmt or a timestamp: %q", got)
	}
}

// A name from outside cannot start a line of its own, on either face.
func TestALoggerNameCannotInjectALine(t *testing.T) {
	t.Parallel()
	for _, styled := range []bool{false, true} {
		var buf bytes.Buffer
		newLogger(&buf, "app\nforged level=error", styled, fixedNow, colorprofile.NoTTY, todaysStream()).Info("x")
		if n := strings.Count(strings.TrimRight(buf.String(), "\n"), "\n"); n != 0 {
			t.Errorf("styled %v: a name made %d extra lines: %q", styled, n, buf.String())
		}
	}
}

// The styled face is for a person at a terminal on standard error, and for
// nobody else: not --plain, not --json, not a pipe, not a writer of the
// caller's.
func TestTheStyledLogFaceIsForATerminalOnStderr(t *testing.T) {
	t.Parallel()
	terminal := stream{rich: true, interactive: true}
	for _, tc := range []struct {
		name  string
		w     io.Writer
		s     stream
		plain bool
		json  bool
		want  bool
	}{
		{"a terminal", nil, terminal, false, false, true},
		{"--plain", nil, terminal, true, false, false},
		{"--json", nil, terminal, false, true, false},
		{"a pipe", nil, stream{rich: false, interactive: false}, false, false, false},
		{"colour off on a terminal", nil, stream{interactive: true}, false, false, false},
		{"a writer given", &bytes.Buffer{}, terminal, false, false, false},
	} {
		if got := loggerIsStyled(tc.w, tc.s, tc.plain, tc.json); got != tc.want {
			t.Errorf("%s: styled %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Each level has its own colour: a warning is the attention colour, not the
// colour of an error.
func TestLogLevelsAreInTheirOwnColours(t *testing.T) {
	t.Parallel()
	st := logStyles(todaysStream())
	for level, want := range map[log.Level]color.Color{
		log.InfoLevel:  todaysStream().colour(nameAction),
		log.WarnLevel:  todaysStream().colour(nameAttn),
		log.ErrorLevel: todaysStream().colour(nameBad),
	} {
		if got := st.Levels[level].GetForeground(); got != want {
			t.Errorf("%v is %v, want %v", level, got, want)
		}
	}
	if todaysStream().colour(nameAttn) == todaysStream().colour(nameBad) {
		t.Fatal("attention and bad are one colour, so this proves nothing")
	}
}

// A plain session's logger, and one under `--no-color`, writes logfmt on
// standard error even at a terminal that renders, and another session's
// logger there is styled: the session's `--plain` and `--no-color` reach the
// face NewLogger picks.
//
// What it costs when this fails: a person who passed --plain to a
// long-running program still gets the drawn log face, which a screen reader
// reads as noise.
func TestAPlainSessionsLoggerIsLogfmtAtATerminal(t *testing.T) {
	// serial: it sets the stderr stream's face and reads the process's stderr.
	errTerm := On(Terminal{ErrColour: true, Interactive: true})
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	n := 0
	for _, c := range []struct {
		name   string
		s      *Session
		logfmt bool
	}{
		{"a plain session", New(errTerm, Plain()), true},
		{"a session under --no-color", noColour(New(errTerm)), true},
		{"a session", New(errTerm), false},
	} {
		out := capture(t, &os.Stderr, func() { c.s.NewLogger(nil, "app").Info("started") })
		if got := strings.Contains(out, "time=") && strings.Contains(out, "msg=started"); got != c.logfmt {
			t.Errorf("%s: logged %q, logfmt %v, want %v", c.name, out, got, c.logfmt)
		}
		n++
	}
	if n != 3 {
		t.Fatalf("made %d loggers, want 3", n)
	}
}

// noColour is s under `--no-color`, as the flag sets it.
func noColour(s *Session) *Session {
	s.noColor.Store(true)
	return s
}
