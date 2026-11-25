package ui

// The one place ui reads the process: Detect, which a caller calls to say what
// its streams can do, and New's defaults, the process's own streams. Nothing
// else in ui reads os.Stdout, os.Stderr, os.Stdin or the environment, or asks
// the clock but through the session (Clock); TestOnlyDetectReadsTheProcess
// holds that.

import (
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
)

// Terminal is what a session's streams can do, as its caller says. The zero
// value is a pipe: no colour, width unknown, and nobody at it to ask.
type Terminal struct {
	Colour, ErrColour bool
	// ColourBits is the depth of the colour Colour has: 0 with none, 24 where colorprofile takes COLORTERM
	// or TERM as truecolor, 8 where it takes TERM as 256 colours (256color, screen, tmux), and 4 otherwise.
	ColourBits int
	// Width and Height are the screen's size in cells and lines; 0 is unknown.
	Width, Height int
	// Interactive is whether the input, the output and the error stream are a
	// terminal a person is at: a question can be asked, a screen drawn.
	Interactive bool
	Accessible  bool
	// Dark is whether the screen's ground is dark, which a theme of fixed
	// colours (Contrast) draws its dark half for. Detect does not ask it, since
	// asking writes to the terminal.
	Dark bool
	// Unicode is whether the terminal draws UTF-8: the banner, spinner, bars and borders are drawn in their
	// non-ASCII forms only where it is true. Detect reads it from the console's output code page on Windows
	// (65001), and elsewhere from the first of the three locale names that is set. The zero value is ASCII.
	Unicode bool
	// Env is the environment the screens and questions are drawn under:
	// bubbletea and huh read TERM and the like from it, not from the process.
	// Detect sets it to the env it was handed; nil is an empty environment.
	Env []string
}

// EnvRules are the environment variables Detect reads, by name; a name left
// empty is not read.
type EnvRules struct {
	// NoColour turns colour off when set to anything; ForceColour turns it on
	// off a terminal when set to anything but empty or 0; Columns is the
	// width where there is no terminal; Accessible asks for plain questions
	// when set to anything.
	NoColour, ForceColour, Columns, Accessible string
	// ForceWins lets ForceColour beat NoColour. By default NoColour wins: a
	// person who sets it has said so about every program on the machine, and
	// wanting colour in a capture is not grounds to overrule them.
	ForceWins bool
	// DumbTermIsPlain makes TERM=dumb plain, with the questions accessible.
	DumbTermIsPlain bool
	// CI is the variable a build service sets: set to anything but empty, 0 or false it makes
	// the run plain, no colour and nobody to ask, as a pipe is. A name left empty is not read.
	CI string
	// ColourTerm and Term are the variables the colour depth is read from, COLORTERM and TERM; a name left empty
	// is not read.
	ColourTerm, Term string
	// LCAll, LCType and Lang are the locale variables, LC_ALL, LC_CTYPE and LANG, read in that order: the first
	// that is set says whether the terminal draws UTF-8. A name left empty is not read. Windows reads the code
	// page instead.
	LCAll, LCType, Lang string
}

// The standard names of the colour variables, which colorprofile also reads.
const (
	envNoColour    = "NO_COLOR"
	envForceColour = "CLICOLOR_FORCE"
)

// StandardEnv is the rules most programs take: NO_COLOR, CLICOLOR_FORCE,
// COLUMNS and ACCESSIBLE, NO_COLOR winning, TERM=dumb plain, and CI set (not empty, 0 or false)
// plain with nobody to ask.
func StandardEnv() EnvRules {
	return EnvRules{
		NoColour:        envNoColour,
		ForceColour:     envForceColour,
		Columns:         "COLUMNS",
		Accessible:      "ACCESSIBLE",
		DumbTermIsPlain: true,
		CI:              "CI",
		ColourTerm:      "COLORTERM",
		Term:            "TERM",
		LCAll:           "LC_ALL",
		LCType:          "LC_CTYPE",
		Lang:            "LANG",
	}
}

// Detect is what the three streams can do under the environment env, read by
// rules: whether each is a terminal, its size, and whether escapes render. It
// asks each stream and reads env, and never writes to the terminal. A stream
// that is not an *os.File, as a buffer, is no terminal, and renders escapes
// only where env forces them.
func Detect(in io.Reader, out, errOut io.Writer, env []string, rules EnvRules) Terminal {
	// Switch the Windows console's escapes on before asking what renders: a
	// console that refuses reads as no colour below, and the session is plain.
	// A no-op off Windows and on a stream that is not a file.
	for _, w := range []io.Writer{out, errOut} {
		if f, ok := w.(*os.File); ok {
			lipgloss.EnableLegacyWindowsANSI(f)
		}
	}
	dumb := rules.DumbTermIsPlain && envValue(env, "TERM") == "dumb"
	force := envValue(env, rules.ForceColour)
	forced := force != "" && force != "0"
	noColour := envValue(env, rules.NoColour) != "" && (!rules.ForceWins || !forced)
	ci := !unset(envValue(env, rules.CI))
	plain := noColour || dumb || ci
	t := Terminal{
		Colour:      !plain && (forced || rendersEscapes(out, env)),
		ErrColour:   !plain && (forced || rendersEscapes(errOut, env)),
		Interactive: !ci && isTerminal(in) && isTerminal(out) && isTerminal(errOut),
		Accessible:  envValue(env, rules.Accessible) != "" || dumb,
		Env:         slices.Clone(env),
	}
	t.Unicode = unicodeOf(env, rules)
	t.ColourBits = colourBits(t.Colour, envValue(env, rules.ColourTerm), envValue(env, rules.Term))
	t.Width, t.Height = sizeOf(out)
	if t.Width == 0 {
		if n, err := strconv.Atoi(envValue(env, rules.Columns)); err == nil && n > 0 {
			t.Width = n
		}
	}
	return t
}

// localeIsUTF8 is whether the first of the locale names that is set names UTF-8, in any case, with or without
// the hyphen: en_US.UTF-8, C.utf8.
func localeIsUTF8(env []string, rules EnvRules) bool {
	for _, name := range []string{rules.LCAll, rules.LCType, rules.Lang} {
		if v := envValue(env, name); v != "" {
			return strings.Contains(strings.ReplaceAll(strings.ToLower(v), "-", ""), "utf8")
		}
	}
	return false
}

// colourBits is the depth of the colour, which colorprofile reads from the two variables the rules name: its
// words for them, not ours (COLORTERM yes, true, truecolor or 24bit, and a TERM such as st, xterm-kitty or xterm-direct, or any
// that holds rio, foot or kitty, are truecolor; screen, tmux and any TERM ending 256color are 256 colours); 4 where it has no more to say, and 0
// with no colour.
func colourBits(colour bool, colourTerm, term string) int {
	if !colour {
		return 0
	}
	if term == "" {
		term = "unknown" // an empty TERM makes colorprofile ask the host, on Windows, and the caller's names are all we read
	}
	switch p := colorprofile.Env([]string{"COLORTERM=" + colourTerm, "TERM=" + term}); {
	case p >= colorprofile.TrueColor:
		return 24
	case p >= colorprofile.ANSI256:
		return 8
	}
	return 4
}

func envValue(env []string, name string) string {
	if name == "" {
		return ""
	}
	v := ""
	for _, kv := range env {
		if k, val, ok := strings.Cut(kv, "="); ok && k == name {
			v = val
		}
	}
	return v
}

// rendersEscapes is whether w renders escapes as colorprofile reads it and
// TERM: the colour variables are Detect's to read, by the names its rules give.
func rendersEscapes(w io.Writer, env []string) bool {
	return colorprofile.Detect(w, withoutColourVars(env)) > colorprofile.Ascii
}

func sizeOf(w io.Writer) (width, height int) {
	f, ok := w.(fder)
	if !ok || !isTerminal(w) {
		return 0, 0
	}
	if cols, rows, err := term.GetSize(f.Fd()); err == nil && cols > 0 {
		return cols, rows
	}
	return 0, 0
}

// withoutColourVars is env without NO_COLOR, CLICOLOR_FORCE and CLICOLOR,
// which colorprofile would read by their standard names whatever rules says.
func withoutColourVars(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		switch k, _, _ := strings.Cut(kv, "="); k {
		case envNoColour, envForceColour, "CLICOLOR":
		default:
			out = append(out, kv)
		}
	}
	return out
}

type fder interface{ Fd() uintptr }

func isTerminal(x any) bool {
	f, ok := x.(fder)
	return ok && term.IsTerminal(f.Fd())
}

// The process's streams, New's defaults, read when they are written to so a
// test that swaps os.Stdout catches a session made before it.
func (s *Session) outW() io.Writer {
	if s.out != nil {
		return s.out
	}
	return os.Stdout
}

func (s *Session) errW() io.Writer {
	if s.errOut != nil {
		return s.errOut
	}
	return os.Stderr
}

func (s *Session) inR() io.Reader {
	if s.in != nil {
		return s.in
	}
	return os.Stdin
}

func processArgs() []string { return os.Args[1:] }

func processTerminal() Terminal {
	return Detect(os.Stdin, os.Stdout, os.Stderr, os.Environ(), StandardEnv())
}

func (s *Session) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// unset is whether a switch variable's value is empty, 0 or false.
func unset(v string) bool { return v == "" || v == "0" || v == "false" }
