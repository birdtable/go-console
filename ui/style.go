package ui

import (
	"image/color"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/birdtable/go-console/measure"
)

type stream struct {
	rich        bool
	width       int // 0 means unknown, not narrow
	interactive bool
	dark        bool   // the terminal's ground is dark (Terminal.Dark)
	maxWidth    int    // the widest a line is drawn (MaxWidth); 0 is no cap
	thm         *Theme // the theme it is painted in; nil is ANSI
	ascii       bool   // the terminal does not draw UTF-8 (Terminal.Unicode is false): the ASCII forms are drawn
}

func (s stream) theme() Theme {
	if s.thm == nil {
		return ANSI()
	}
	return *s.thm
}

func (s stream) paints() bool { return s.rich }

// Width is the session's screen width in cells, at most its MaxWidth, or 0 when
// it is unknown. Callers branch on zero rather than passing it to a wrapper,
// which would wrap to zero.
func (s *Session) Width() int { return widthOf(s.outStream()) }

func widthOf(st stream) int {
	if st.maxWidth > 0 && st.width > st.maxWidth {
		return st.maxWidth
	}
	return st.width
}

// MaxWidth caps the width the session draws to, help included: past a measure,
// longer lines get harder to read, so a wide terminal gets margin. Without it
// the terminal's whole width is drawn to.
func MaxWidth(cells int) SessionOption { return func(s *Session) { s.maxWidth = cells } }

// --- the theme --------------------------------------------------------------

// A Colour is one role's colour on each ground a terminal can have, Light on a
// light background and Dark on a dark one: "#rrggbb", or "0" to "15" for one of
// the terminal's own sixteen. Empty is none. Which half is drawn is
// Terminal.Dark's to say.
type Colour struct{ Light, Dark string }

// A Theme is the colours ui paints in: the four coloured roles (Bad is wrong,
// Attn needs a person, Good is right, Action is to type), Muted for what a
// reader may skip, faint when it is empty, and CodeGround, the ground a code
// block is set on, none when it is empty. Bold is bold in every theme. A role
// left empty is drawn in no colour.
type Theme struct{ Bad, Attn, Good, Action, Muted, CodeGround Colour }

// ANSI is the theme a session paints in unless WithTheme says otherwise: the
// terminal's own red, yellow, green and cyan, Muted faint, and code on the
// terminal's grey. It follows the reader's own colour scheme, and needs no
// question asked of the terminal.
func ANSI() Theme {
	return Theme{
		Bad: Colour{"1", "1"}, Attn: Colour{"3", "3"}, Good: Colour{"2", "2"}, Action: Colour{"6", "6"},
		CodeGround: Colour{"7", "8"},
	}
}

// Contrast is a theme of fixed colours chosen against measured contrast on both
// grounds; it draws the half for the ground Terminal.Dark says. Its contrast,
// light then dark: all eight clear the 4.5:1 readable threshold for body text,
// amber on white by the least.
//
//	bad     #b91c1c on #ffffff  6.47   #f87171 on #1e1e1e  6.03
//	attn    #a16207 on #ffffff  4.92   #fbbf24 on #1e1e1e  9.99
//	good    #15803d on #ffffff  5.02   #4ade80 on #1e1e1e  9.57
//	action  #0e7490 on #ffffff  5.36   #22d3ee on #1e1e1e  9.22
//
// TestEveryRoleIsReadableOnBothGrounds computes over these pairs. A table in a
// comment is a claim; the test is the check.
func Contrast() Theme {
	return Theme{
		Bad:        Colour{"#b91c1c", "#f87171"},
		Attn:       Colour{"#a16207", "#fbbf24"},
		Good:       Colour{"#15803d", "#4ade80"},
		Action:     Colour{"#0e7490", "#22d3ee"},
		CodeGround: Colour{"#f3f3f1", "#2a2a2a"},
	}
}

// Mono is a theme of no colour: Bold bold and Muted faint, and nothing else.
func Mono() Theme { return Theme{} }

// WithTheme paints the session in t: its lines and words (s.Bad, s.Good and
// the rest), its questions, its tables, its logger and the help Execute
// renders for it.
func WithTheme(t Theme) SessionOption {
	return func(s *Session) { s.theme = &t }
}

const (
	nameBad    = "bad"
	nameAttn   = "attn"
	nameGood   = "good"
	nameAction = "action"
	nameMuted  = "muted"
	nameGround = "ground"
)

func (t Theme) byName(role string) Colour {
	switch role {
	case nameBad:
		return t.Bad
	case nameAttn:
		return t.Attn
	case nameGood:
		return t.Good
	case nameAction:
		return t.Action
	case nameMuted:
		return t.Muted
	case nameGround:
		return t.CodeGround
	}
	return Colour{}
}

type pair struct{ light, dark string }

func hexes() map[string]pair {
	c := Contrast()
	out := map[string]pair{}
	for _, role := range []string{nameBad, nameAttn, nameGood, nameAction} {
		col := c.byName(role)
		out[role] = pair{col.Light, col.Dark}
	}
	return out
}

const (
	lightGround = "#ffffff"
	darkGround  = "#1e1e1e"
)

func colourIn(t Theme, role string, dark bool) color.Color {
	c := t.byName(role)
	v := c.Light
	if dark {
		v = c.Dark
	}
	if v == "" {
		return nil
	}
	return lipgloss.Color(v)
}

func (s stream) colour(role string) color.Color { return colourIn(s.theme(), role, s.dark) }

// A role is a value, not a closure over one stream, so Warn can paint
// it against stderr's own detection, and in the theme that stream carries.
// TestAWarningIsPaintedAgainstStderrNotStdout holds that.
type role struct {
	colour string // the theme's role it is painted in; empty is none
	faint  bool   // faint where the theme gives the role no colour
	bold   bool
}

func roleBad() role    { return role{colour: nameBad} }
func roleAttn() role   { return role{colour: nameAttn} }
func roleGood() role   { return role{colour: nameGood} }
func roleAction() role { return role{colour: nameAction} }
func roleNote() role   { return role{colour: nameMuted, faint: true} }
func roleBold() role   { return role{bold: true} }

func (r role) on(s stream, text string) string {
	if !s.paints() {
		return text
	}
	st := lipgloss.NewStyle()
	if c := s.colour(r.colour); c != nil {
		st = st.Foreground(c)
	} else if r.faint {
		st = st.Faint(true)
	}
	if r.bold {
		st = st.Bold(true)
	}
	return st.Render(text)
}

// The six roles, and Wrap and Column, painted for the session's stdout, as its
// Terminal says. The package doc says when to reach for each. Each shares its package
// function's one implementation, the role's on, so the two cannot differ.

// Bad paints state that is wrong: a file at risk, or a guard that failed.
func (s *Session) Bad(text string) string { return roleBad().on(s.outStream(), text) }

// Attn paints state that is undecided and needs a person, which includes
// "cannot tell", a routine outcome rather than a failure.
func (s *Session) Attn(text string) string { return roleAttn().on(s.outStream(), text) }

// Good paints state that is right.
func (s *Session) Good(text string) string { return roleGood().on(s.outStream(), text) }

// Action paints text the reader will type. Nothing else may take this colour.
func (s *Session) Action(text string) string { return roleAction().on(s.outStream(), text) }

// Note paints text the reader may skip.
func (s *Session) Note(text string) string { return roleNote().on(s.outStream(), text) }

// Bold paints the one thing on a screen that is the answer.
func (s *Session) Bold(text string) string { return roleBold().on(s.outStream(), text) }

// Wrap reflows text to the session's screen, indenting continuation lines by
// n. At an unknown width it returns text untouched. It breaks at spaces only: a
// hyphen or an equals sign is never a breakpoint, so --apply and --out=dir stay
// whole, and a word wider than the line overflows rather than being cut. It
// cannot tell a flag's value from the next argument, so in text that wraps
// write --flag=value, not --flag value. A line printed for the reader to copy
// goes out without Wrap, and overflows a narrow screen rather than breaking.
func (s *Session) Wrap(n int, text string) string { return wrapOn(s.outStream(), n, text) }

// Column is the package function Column, which pads by cells and paints nothing, so it is the same
// for every terminal; a method so a caller drawing for a session reads as one.
func (s *Session) Column(text string, n int) string { return Column(text, n) }

// --- markup -----------------------------------------------------------------

// A Tone is what a piece of text is, which a theme paints: plain, good,
// attention, bad, to type, to skip, or the one thing that is the answer.
type Tone int

// The tones, in the order a theme lists them.
const (
	TonePlain Tone = iota
	ToneGood
	ToneAttn
	ToneBad
	ToneAction
	ToneMuted
	ToneStrong
)

// A Painter paints text in a tone, for the terminal and theme a line is drawn
// for: plain where the stream takes no colour.
type Painter interface {
	Paint(t Tone, text string) string
}

type painter struct{ st stream }

func (p painter) Paint(t Tone, text string) string {
	switch t {
	case ToneGood:
		return roleGood().on(p.st, text)
	case ToneAttn:
		return roleAttn().on(p.st, text)
	case ToneBad:
		return roleBad().on(p.st, text)
	case ToneAction:
		return roleAction().on(p.st, text)
	case ToneMuted:
		return roleNote().on(p.st, text)
	case ToneStrong:
		return roleBold().on(p.st, text)
	case TonePlain:
	}
	return text
}

// WithMarkup marks up each line the session prints with f, given a Painter
// for the stream it is printed on: a program's own way of writing emphasis
// into its strings. Without it a line is printed as it is written.
func WithMarkup(f func(text string, p Painter) string) SessionOption {
	return func(s *Session) { s.markupFn = f }
}

var backticked = regexp.MustCompile("`([^`\n]+)`")

// BacktickMarkup paints each command written between backticks as text to type,
// the backticks dropped, so emphasis costs nothing where the line is written
// and the string stays legible in a log. Where the stream takes no colour the
// line is as written: the backticks are then the only thing marking a command
// as one.
func BacktickMarkup(text string, p Painter) string {
	if p.Paint(ToneAction, "x") == "x" {
		return text
	}
	return backticked.ReplaceAllStringFunc(text, func(m string) string {
		return p.Paint(ToneAction, strings.Trim(m, "`"))
	})
}

var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

func (s *Session) markupOn(st stream, text string) string {
	if s.markupFn == nil {
		return text
	}
	return onPlainRuns(text, func(run string) string { return s.markupFn(run, painter{st}) })
}

// Text a call site has already painted is left alone, because emphasis inside
// it does not nest. A style ends at a reset, and the marked-up word brings its
// own reset with it, so bolding a path inside a dimmed line ends the dimming at
// the path and the rest of the sentence comes back at full strength. The call
// site picked that style for the whole line, and it gets to keep it.
func onPlainRuns(s string, f func(string) string) string {
	var out strings.Builder
	styled, last := false, 0
	for _, m := range sgr.FindAllStringIndex(s, -1) {
		run := s[last:m[0]]
		if styled || run == "" {
			out.WriteString(run)
		} else {
			out.WriteString(f(run))
		}
		out.WriteString(s[m[0]:m[1]])
		// \x1b[m and \x1b[0m both end every style. Anything else starts one.
		code := s[m[0]+2 : m[1]-1]
		styled = code != "" && code != "0"
		last = m[1]
	}
	if tail := s[last:]; styled || tail == "" {
		out.WriteString(tail)
	} else {
		out.WriteString(f(tail))
	}
	return out.String()
}

// --- layout -----------------------------------------------------------------

func wrapOn(st stream, n int, s string) string {
	w := widthOf(st)
	if w == 0 {
		return s
	}
	return strings.ReplaceAll(breakAtSpaces(s, w-n), "\n", "\n"+strings.Repeat(" ", n))
}

// breakAtSpaces is s broken into lines of at most limit cells, at spaces and
// nowhere else: a hyphen or an equals sign is never a breakpoint, so a flag
// such as --apply or --out=dir stays whole. A word wider than limit overflows
// the line; it is not cut. Existing newlines stay, and escape sequences have
// no width. A limit under one leaves s as it is.
func breakAtSpaces(s string, limit int) string {
	if limit < 1 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = breakLine(line, limit)
	}
	return strings.Join(lines, "\n")
}

func breakLine(line string, limit int) string {
	if cells(line) <= limit {
		return line
	}
	var out strings.Builder
	used := 0
	for i, w := range strings.Split(line, " ") {
		wc := cells(w)
		switch {
		case i == 0:
		case used+1+wc <= limit:
			out.WriteByte(' ')
			used++
		case w == "":
			continue
		default:
			out.WriteByte('\n')
			used = 0
		}
		out.WriteString(w)
		used += wc
	}
	return out.String()
}

// cells is the display width of s, which is the unit every column here uses.
// Not bytes and not runes: a filename is the one string in this tool that
// arrives from outside, and one day it will not be ASCII.
func cells(s string) int { return ansi.StringWidth(s) }

// Column is measure.Column: s padded to n display cells, with a gap after a label that fills its column.
func Column(s string, n int) string { return measure.Column(s, n) }

func mutedStyle(st stream) lipgloss.Style {
	if c := st.colour(nameMuted); c != nil {
		return lipgloss.NewStyle().Foreground(c)
	}
	return lipgloss.NewStyle().Faint(true)
}
