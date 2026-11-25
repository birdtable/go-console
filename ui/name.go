package ui

// Printing a name somebody else chose.

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/birdtable/go-console/shell"
)

// Name renders a filename for display.
//
// A filename is not trusted text. On macOS and Linux it may hold a newline, a
// carriage return or an escape character, and printed raw it ends the line and
// writes the rest of itself as though this tool had said it: a file can add
// "done  filed 40,000 files" to a transcript, or colour and reposition
// what follows. Every place a name from outside the tool reaches the screen goes
// through this. Log,
// Info, Change, Warn and End apply it to each argument themselves; a caller
// calls it where text from outside is built into a string another way, a
// table's cell or a tree's line.
//
// Only the characters that cannot be trusted on a line are replaced, each by its
// number, so an ordinary name with accents, emoji or spaces in it prints exactly
// as it is. Those are:
//   - the controls (Cc);
//   - the format characters (Cf): the bidi controls, which reorder the rest of the
//     line, so `evil\u202etxt.exe` would show as `evilexe.txt`, and the zero-width
//     ones, which make two names look the same;
//   - the line and paragraph separators U+2028 and U+2029 (Zl, Zp), which some
//     terminals and most log viewers take for a line break;
//   - the other characters Unicode says a display may ignore, such as a combining
//     grapheme joiner or a Hangul filler, and the variation selectors, which print
//     as nothing.
//
// Emoji keep what builds them: a zero-width joiner between two parts, U+FE0E or
// U+FE0F after an emoji, U+FE0F in a keycap. Anywhere else each is quoted. The
// start and end of a name, and a byte that is not UTF-8, are no part of an emoji.
//
// A newline, a carriage return and a tab are \n, \r and \t; any other control
// below 0x80 is \x and two digits; anything else replaced is \u and four digits,
// or \U and eight; and a byte that is not UTF-8 is \x and its two digits, 0x80 or
// above, never U+FFFD. So two names that differ only in what is replaced print
// differently. A backslash is left as it is, by design: quoting it
// as \\ would print every Windows path, C:\Users\ana, with its separators doubled,
// and a person reads a path more often than a quote. So a name can hold the text
// of a quote, and `a\x07b` typed out prints as a name with a bell in it does. That
// is a name its author chose to write that way; it cannot end the line, reorder it
// or hide a character, which is what the replacing is for.
func Name(s string) string { return escape(s, nil) }

// escape is Name's rule, keeping each colour escape that keep holds: the ones
// this package paints with, so a word a caller painted with Bold or Note keeps
// its colour when it is passed as an argument, while one that came from
// outside is quoted with the rest. A colour escape only colours; one that
// moves the cursor, erases or ends the line is never kept. When the last one
// kept starts a style, a reset ends it, so the colour stops with s and does
// not run on through the rest of the line and the lines after.
func escape(s string, keep map[string]bool) string {
	if !needsEscaping(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	open := false
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && len(keep) > 0 {
			if m := leadingSGR.FindString(s[i:]); m != "" && keep[m] {
				b.WriteString(m)
				open = !endsEveryStyle(m)
				i += len(m)
				continue
			}
		}
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			// A byte that is not UTF-8: its number, never U+FFFD, which two
			// different names would both print as.
			b.WriteString(hexByte(s[i]))
			i++
			continue
		}
		at := i
		i += n
		switch {
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case hidden(s, at, r, n):
			// Quoted as its number rather than dropped, so two names that
			// differ only there still look different.
			b.WriteString(quoteRune(r))
		default:
			b.WriteRune(r)
		}
	}
	if open {
		b.WriteString(reset)
	}
	return b.String()
}

var leadingSGR = regexp.MustCompile("^\x1b\\[[0-9;]*m")

const reset = "\x1b[m"

func endsEveryStyle(m string) bool { return m == reset || m == "\x1b[0m" }

func needsEscaping(s string) bool {
	if !utf8.ValidString(s) {
		return true
	}
	for i, r := range s {
		if hidden(s, i, r, utf8.RuneLen(r)) {
			return true
		}
	}
	return false
}

const (
	zwj    = 0x200d // the zero-width joiner, which joins the parts of an emoji
	vs15   = 0xfe0e // the variation selector that asks for an emoji as text
	vs16   = 0xfe0f // the variation selector that asks for an emoji as a picture
	keycap = 0x20e3 // the enclosing keycap a digit, # or * becomes a key with
)

// hidden is whether r, the character of n bytes at s[i:], may not reach the screen as it is: what
// shell.Invisible says of it, the one set ErrControl refuses too, with one
// exception, the emoji's: a joiner or a selector that is part of an emoji is not hidden.
func hidden(s string, i int, r rune, n int) bool {
	switch {
	case r == zwj:
		return !endsEmoji(s, i) || !pictograph(after(s, i+n))
	case unicode.Is(unicode.Variation_Selector, r):
		return !selectsEmoji(s, i, r, n)
	}
	return shell.Invisible(r)
}

func selectsEmoji(s string, i int, r rune, n int) bool {
	if r != vs15 && r != vs16 {
		return false
	}
	p := before(s, i)
	return pictograph(p) || (r == vs16 && keycapBase(p) && after(s, i+n) == keycap)
}

func endsEmoji(s string, i int) bool {
	p := before(s, i)
	if p == vs16 {
		return pictograph(before(s, i-utf8.RuneLen(vs16)))
	}
	return pictograph(p) || (p >= 0x1f3fb && p <= 0x1f3ff)
}

func pictograph(r rune) bool { return r >= 0 && r != utf8.RuneError && unicode.Is(unicode.So, r) }

func keycapBase(r rune) bool { return (r >= '0' && r <= '9') || r == '#' || r == '*' }

// before is the character that ends s[:i], or -1 at the start, or where a byte is not UTF-8.
func before(s string, i int) rune {
	r, n := utf8.DecodeLastRuneInString(s[:i])
	if n == 0 || (r == utf8.RuneError && n == 1) {
		return -1
	}
	return r
}

// after is the character that starts s[i:], or -1 at the end, or where a byte is not UTF-8.
func after(s string, i int) rune {
	r, n := utf8.DecodeRuneInString(s[i:])
	if n == 0 || (r == utf8.RuneError && n == 1) {
		return -1
	}
	return r
}

func quoteRune(r rune) string {
	switch {
	case r < 0x80:
		return `\x` + pad(strconv.FormatInt(int64(r), 16), 2)
	case r <= 0xffff:
		return `\u` + pad(strconv.FormatInt(int64(r), 16), 4)
	}
	return `\U` + pad(strconv.FormatInt(int64(r), 16), 8)
}

func hexByte(b byte) string { return `\x` + pad(strconv.FormatInt(int64(b), 16), 2) }

func pad(digits string, width int) string {
	return strings.Repeat("0", max(width-len(digits), 0)) + digits
}

// Strip is s as plain text: every control sequence taken out (colours, cursor
// moves, an OSC title, a string a terminal would act on), then every control
// character left, a bare escape byte or a C1 control among them, and a byte that
// is not UTF-8. A newline and a tab stay. Use it where text from outside goes to
// a place that wants words and nothing a terminal could act on; Name is for text
// a person reads, and shows what it replaces.
func Strip(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || r == utf8.RuneError {
			return -1
		}
		return r
	}, s)
}
