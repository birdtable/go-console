package ui

// Text from outside reaches the screen as an argument of the printing
// functions: a file name, a peer's message, an error that quotes one. These
// hold that it is made safe there, whatever the caller remembered to do.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

// forged is data that would rewrite a line printed raw: erase it, return to
// its start, end it and write a line of its own, ring the bell, or hide what
// follows. Each comes as a string and as an error, which %v formats.
func forged() []string {
	return []string{
		"a\x1b[2Kb",
		"a\rdone  filed 40,000 files",
		"a\ndone  filed 40,000 files",
		"a\x07b",
		"a\x1b[8mhidden",
		"a\x1b]0;title\x07b",
	}
}

// printers is each printing function, with the stream it writes to.
type printer struct {
	name   string
	stream **os.File
	print  func(a any)
}

func printers() []printer {
	return []printer{
		{"Log", &os.Stdout, func(a any) { New().Log("%s", a) }},
		{"Info", &os.Stdout, func(a any) { New().Info("%v", a) }},
		{"Change", &os.Stdout, func(a any) { New(WithMessages(houseMessages())).Change("file %s", a) }},
		{"Warn", &os.Stderr, func(a any) {
			New().Warn(closing{word: "failed", what: fmt.Sprint(a), code: 1})
		}},
	}
}

// controls is every control character in s, other than the newline that
// ends it.
func controls(s string) []rune {
	var found []rune
	// An ending with no line of its own ends on the command's help, a line
	// ui writes itself.
	if t, ok := strings.CutSuffix(s, "\n    --help\n"); ok {
		s = t + "\n"
	}
	for _, r := range strings.TrimSuffix(s, "\n") {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			found = append(found, r)
		}
	}
	return found
}

// Off a terminal nothing is painted, so the line holds no control byte at all
// but the newline that ends it, and the forged part is quoted, not dropped.
func TestAnArgumentFromOutsidePrintsNoControlByte(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	ran := 0
	for _, p := range printers() {
		for _, data := range forged() {
			for _, arg := range []any{data, errors.New(data)} {
				ran++
				out := capture(t, p.stream, func() { p.print(arg) })
				if c := controls(out); len(c) > 0 {
					t.Errorf("%s(%q): control bytes %q in %q", p.name, data, c, out)
				}
				if !strings.Contains(out, Name(data)) {
					t.Errorf("%s(%q): %q does not say it as Name does, %q", p.name, data, out, Name(data))
				}
			}
		}
	}
	if ran == 0 {
		t.Fatal("no case ran")
	}
}

// At a terminal the line is painted, and only with the package's own colours:
// none of the forged escapes reaches it.
func TestAnArgumentFromOutsideKeepsNoEscapeAtATerminal(t *testing.T) {
	scr := screenOpt(true, 0)
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	own := ownColours(New(scr).outStream(), New(scr).errStream())
	if len(own) == 0 {
		t.Fatal("the roles paint nothing, so this proves little")
	}
	ran := 0
	for _, p := range printers() {
		for _, data := range forged() {
			ran++
			out := capture(t, p.stream, func() { p.print(data) })
			rest := sgr.ReplaceAllStringFunc(out, func(m string) string {
				if own[m] {
					return ""
				}
				return m
			})
			if c := controls(rest); len(c) > 0 {
				t.Errorf("%s(%q): control bytes %q besides the package's colours in %q", p.name, data, c, out)
			}
		}
	}
	if ran == 0 {
		t.Fatal("no case ran")
	}
}

// What Change reports is the record of the run as well as the screen: the
// list and the run log get the line made safe too.
func TestAChangeRecordsTheLineMadeSafe(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	var logged []string
	s := New(WithMessages(houseMessages()), OnChange(func(line string) { logged = append(logged, line) }))
	s.SetDryRun(false)
	_ = capture(t, &os.Stdout, func() { s.Change("file %s", "a\nforged") })
	if len(logged) != 1 || logged[0] != `file a\nforged` {
		t.Errorf("the run log was told %q", logged)
	}
	if got := s.Changes(); len(got) != 1 || got[0] != `file a\nforged` {
		t.Errorf("the changes list holds %q", got)
	}
}

// A word the caller painted with the package's own roles keeps its colour as
// an argument; a forged escape inside it is still quoted.
func TestAPaintedArgumentKeepsItsColour(t *testing.T) {
	scr := screenOpt(true, 0)
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	question, answer := New(scr).Note("Which files?"), New(scr).Bold("all")
	if question == "Which files?" || answer == "all" {
		t.Fatal("the roles paint nothing, so this proves little")
	}
	out := capture(t, &os.Stdout, func() { New(scr).Log("%s %s", question, answer) })
	if !strings.Contains(out, question+" "+answer) {
		t.Errorf("the painted words lost their colour: %q", out)
	}
	out = capture(t, &os.Stdout, func() { New(scr).Log("%s", New(scr).Bold("a\x1b[2Kb")) })
	if !strings.Contains(out, New(scr).Bold(`a\x1b[2Kb`)) {
		t.Errorf("a forged escape inside a painted word: %q, want %q in it", out, New(scr).Bold(`a\x1b[2Kb`))
	}
}

// Ordinary text from outside prints exactly as given: accents, other
// scripts, emoji and a space are not control characters.
func TestAccentsAndEmojiPrintExactlyAsGiven(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	const data = "Zoë's café, 東京 📷 naïve—ok"
	ran := 0
	for _, p := range printers() {
		ran++
		if out := capture(t, p.stream, func() { p.print(data) }); !strings.Contains(out, data) {
			t.Errorf("%s: %q is not in %q", p.name, data, out)
		}
	}
	for _, styled := range []bool{false, true} {
		ran++
		var buf bytes.Buffer
		newLogger(&buf, "app", styled, fixedNow, colorprofile.NoTTY, todaysStream()).Info(data, "who", data)
		if strings.Count(buf.String(), data) != 2 {
			t.Errorf("logger, styled %v: %q is not twice in %q", styled, data, buf.String())
		}
	}
	if ran == 0 {
		t.Fatal("no case ran")
	}
}

// width is a named integer type, as a caller passes for a * width.
type width int

// level is a named integer type that says itself, with a line break in it.
type level int

func (level) String() string { return "warn\nforged" }

// A number prints as it did before it was wrapped: a * width still reads
// it, of a named type too, and a named integer with a String method is said
// by it, quoted.
func TestANumberArgumentIsFormattedAsBefore(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	out := capture(t, &os.Stdout, func() {
		New().Log("%*d|%5.1f|%t|%-4s|%*s|%x|%08b|%v|%d|", 5, 3, 2.25, true, "ab", width(3), "c", uint8(255), int8(-3), level(1), uintptr(7))
	})
	if want := "    3|  2.2|true|ab  |  c|ff|-0000011|warn\\nforged|7|\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// printf is each printing function given its own format, with the stream it
// writes to.
type printerf struct {
	name   string
	stream **os.File
	print  func(format string, a ...any)
}

func printf(opts ...SessionOption) []printerf {
	return []printerf{
		{"Log", &os.Stdout, func(format string, a ...any) { New(opts...).Log(format, a...) }},
		{"Info", &os.Stdout, func(format string, a ...any) { New(opts...).Info(format, a...) }},
		{"Change", &os.Stdout, func(format string, a ...any) {
			New(append([]SessionOption{WithMessages(houseMessages())}, opts...)...).Change("file "+format, a...)
		}},
	}
}

// An integer printed with %c is a character, from outside when it was read
// from a file, so it is quoted as a string's would be: an escape, a carriage
// return, a C1 control, a bell, of any integer type.
func TestAnIntegerPrintedAsACharacterIsQuoted(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	type key rune
	ran := 0
	for _, p := range printf() {
		for _, c := range []struct {
			arg  any
			want string
		}{
			{rune(0x1b), `\x1b`},
			{byte('\r'), `\r`},
			{rune(0x9b), `\u009b`},
			{key(0x07), `\x07`},
			{uint32('\n'), `\n`},
		} {
			ran++
			out := capture(t, p.stream, func() { p.print("unexpected %c in the name", c.arg) })
			if cs := controls(out); len(cs) > 0 {
				t.Errorf("%s(%%c of %#v): control bytes %q in %q", p.name, c.arg, cs, out)
			}
			if !strings.Contains(out, "unexpected "+c.want+" in the name") {
				t.Errorf("%s(%%c of %#v): %q does not say %s", p.name, c.arg, out, c.want)
			}
		}
	}
	if ran == 0 {
		t.Fatal("no case ran")
	}
}

// A colour this package paints with, kept in an argument from outside, ends
// with the argument: a reset follows it, so it colours neither the rest of
// the line nor the lines after. A reset of the argument's own needs none.
func TestAColourKeptInAnArgumentEndsWithIt(t *testing.T) {
	scr := screenOpt(true, 0)
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	ran := 0
	for m := range ownColours(New(scr).outStream(), New(scr).errStream()) {
		if endsEveryStyle(m) {
			continue
		}
		for _, p := range printf(scr) {
			ran++
			out := capture(t, p.stream, func() { p.print("%s copied", "x"+m+"y") })
			if !strings.Contains(out, "x"+m+"y"+reset+" copied") {
				t.Errorf("%s(%q): the colour is not ended with the argument: %q", p.name, m, out)
			}
		}
		ran++
		closed := "x" + m + "y" + reset
		if out := capture(t, &os.Stdout, func() { New(scr).Log("%s copied", closed) }); !strings.Contains(out, closed+" copied") || strings.Contains(out, reset+reset) {
			t.Errorf("Log(%q): %q", closed, out)
		}
		for _, styled := range []bool{false, true} {
			ran++
			var buf bytes.Buffer
			newLogger(&buf, "app", styled, fixedNow, colorprofile.TrueColor, todaysStream()).Info("x" + m + "y")
			all := sgr.FindAllString(buf.String(), -1)
			if len(all) > 0 && !endsEveryStyle(all[len(all)-1]) {
				t.Errorf("logger, styled %v, %q: the entry ends in a style left open: %q", styled, m, buf.String())
			}
		}
	}
	if ran < 10 {
		t.Fatalf("ran %d cases: the roles paint too little to prove this", ran)
	}
}

// A log entry is one line on either face and in colour or not, whatever its
// message and its values hold, and no forged escape reaches it.
func TestALogEntryIsOneLineWhateverItsDataHolds(t *testing.T) {
	t.Parallel()
	ran := 0
	for _, styled := range []bool{false, true} {
		for _, profile := range []colorprofile.Profile{colorprofile.NoTTY, colorprofile.TrueColor} {
			for _, data := range forged() {
				ran++
				var buf bytes.Buffer
				l := newLogger(&buf, "app", styled, fixedNow, profile, todaysStream())
				l.Info(data, "who", data, "lines", "x\ny")
				l.Warn("plain", "err", errors.New(data))
				out := buf.String()
				if n := strings.Count(out, "\n"); n != 2 {
					t.Errorf("styled %v, %v, %q: %d lines: %q", styled, profile, data, n, out)
				}
				for _, line := range strings.SplitAfter(out, "\n") {
					if c := controls(sgr.ReplaceAllString(line, "")); len(c) > 0 {
						t.Errorf("styled %v, %v, %q: control bytes %q in %q", styled, profile, data, c, line)
					}
				}
				for _, esc := range []string{"\x1b[2K", "\x1b[8m", "\x1b]0;"} {
					if strings.Contains(out, esc) {
						t.Errorf("styled %v, %v: %q reached the log: %q", styled, profile, esc, out)
					}
				}
			}
		}
	}
	if ran == 0 {
		t.Fatal("no case ran")
	}
}

// The styled face in colour keeps its own colours: the level word is still
// painted.
func TestAStyledLogKeepsItsOwnColours(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	newLogger(&buf, "app", true, fixedNow, colorprofile.TrueColor, todaysStream()).Info("a\nb")
	if !sgr.MatchString(buf.String()) {
		t.Errorf("the styled face lost its colours: %q", buf.String())
	}
	if !strings.Contains(buf.String(), `a\nb`) {
		t.Errorf("the message is not quoted: %q", buf.String())
	}
}
