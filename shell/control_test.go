package shell_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/birdtable/go-console/shell"
)

type named struct {
	name string
	d    shell.Dialect
}

func dialects() []named {
	return []named{{"posix", shell.POSIX}, {"powershell", shell.PowerShell}, {"cmd", shell.Cmd}}
}

// Every control character and every format character is refused, in every dialect, with ErrControl, the argument's
// index and the rune as U+XXXX, and never the rune itself.
//
// What it costs when this fails: a line printed for a person to paste hides or redraws part of itself on a terminal
// (ESC, a bidi override, a zero-width space), or a program that strips controls before it shows the line shows
// another line than the one pasted.
func TestQuoteRefusesControlAndFormatCharacters(t *testing.T) {
	t.Parallel()
	for _, r := range []rune{'\t', 0x1b, 0x7f, 0x85, 0x200b, 0x2060, 0xfeff, 0x180e, 0xad, 0x200d, 0x202e, 0x2066, 0x2069} {
		for _, dd := range dialects() {
			for _, arg := range []string{"a" + string(r) + "b", string(r)} {
				line, err := shell.Quote(dd.d, []string{"x", arg})
				want := fmt.Sprintf("U+%04X", r)
				if !errors.Is(err, shell.ErrControl) || line != "" || !strings.Contains(err.Error(), "argument 1 ") ||
					!strings.Contains(err.Error(), want) || strings.ContainsRune(err.Error(), r) {
					t.Errorf("%s U+%04X: Quote(%q) = %q, %v; want ErrControl, argument 1, %s and not the rune", dd.name, r, arg, line, err, want)
				}
			}
			if _, err := shell.Quote(dd.d, []string{string(r), "x"}); !errors.Is(err, shell.ErrControl) || !strings.Contains(err.Error(), "argument 0 ") {
				t.Errorf("%s U+%04X first: %v; want ErrControl, argument 0", dd.name, r, err)
			}
		}
	}
}

// The range: all of unicode.Cf and all of unicode.Cc bar NUL, LF and CR (which keep their own errors).
func TestQuoteRefusesEveryControlAndFormatRune(t *testing.T) {
	t.Parallel()
	for r := rune(1); r <= unicode.MaxRune; r++ {
		if r == '\n' || r == '\r' || (!unicode.IsControl(r) && !unicode.Is(unicode.Cf, r)) {
			continue
		}
		for _, dd := range dialects() {
			if _, err := shell.Quote(dd.d, []string{"x", "a" + string(r) + "b"}); !errors.Is(err, shell.ErrControl) {
				t.Errorf("%s U+%04X: %v; want ErrControl", dd.name, r, err)
			}
		}
	}
}

// NUL, LF and CR keep the errors they had, so no caller's errors.Is breaks.
func TestQuoteKeepsTheErrorsOfNULAndNewlines(t *testing.T) {
	t.Parallel()
	for _, dd := range dialects() {
		for arg, want := range map[string]error{"a\x00b": shell.ErrNUL, "a\nb": shell.ErrNewline, "a\rb": shell.ErrNewline} {
			if _, err := shell.Quote(dd.d, []string{"x", arg}); !errors.Is(err, want) || errors.Is(err, shell.ErrControl) {
				t.Errorf("%s Quote(%q): %v; want %v alone", dd.name, arg, err, want)
			}
		}
	}
}

// Ordinary non-ASCII, an emoji with no joiner or selector, and a combining mark still pass.
func TestQuotePassesOrdinaryNonASCII(t *testing.T) {
	t.Parallel()
	for _, a := range []string{"é", "日本", "😀", "a b", "ß", "e\u0301", "\ue000"} {
		for _, dd := range dialects() {
			if _, err := shell.Quote(dd.d, []string{"x", a}); err != nil {
				t.Errorf("%s Quote(%q) is refused: %v", dd.name, a, err)
			}
		}
	}
}

// Every class the ruling names is refused with ErrControl, by name, in every dialect: the line and paragraph
// separators, the default-ignorable and variation-selector runes (U+FE0F too, with no emoji exception), every
// space but U+0020, and the blank braille.
//
// What it costs when this fails: a hair space or a variation selector inside quotes reads like nothing, so the
// line a person pastes holds a character they never saw.
func TestQuoteRefusesTheInvisibleClassesByName(t *testing.T) {
	t.Parallel()
	runes := []rune{0x34f, 0xfe0f, 0xfe00, 0x180b, 0x115f, 0x3164, 0xffa0, 0xe0100, 0x2028, 0x2029,
		0xa0, 0x1680, 0x2000, 0x200a, 0x202f, 0x205f, 0x3000, 0x2800}
	for _, r := range runes {
		for _, dd := range dialects() {
			for _, arg := range []string{"a" + string(r) + "b", string(r)} {
				line, err := shell.Quote(dd.d, []string{"x", arg})
				want := fmt.Sprintf("U+%04X", r)
				if !errors.Is(err, shell.ErrControl) || line != "" || !strings.Contains(err.Error(), "argument 1 ") ||
					!strings.Contains(err.Error(), want) || strings.ContainsRune(err.Error(), r) {
					t.Errorf("%s U+%04X: Quote(%q) = %q, %v; want ErrControl, argument 1, %s and not the rune", dd.name, r, arg, line, err, want)
				}
			}
		}
	}
}

// The range row is built from the Unicode tables by name, not from the predicate's own expression: every rune of
// each named table is refused, bar NUL, LF and CR, which keep their own errors, and U+0020 stays.
func TestQuoteRefusesEveryRuneOfTheNamedTables(t *testing.T) {
	t.Parallel()
	var tables []*unicode.RangeTable
	for _, n := range []string{"Cc", "Cf", "Zl", "Zp", "Zs"} {
		tables = append(tables, unicode.Categories[n])
	}
	for _, n := range []string{"Other_Default_Ignorable_Code_Point", "Variation_Selector"} {
		tables = append(tables, unicode.Properties[n])
	}
	for _, tb := range tables {
		if tb == nil {
			t.Fatal("a named Unicode table is missing")
		}
	}
	for r := rune(1); r <= unicode.MaxRune; r++ {
		if r == '\n' || r == '\r' || r == ' ' || !unicode.In(r, tables...) && r != 0x2800 {
			continue
		}
		for _, dd := range dialects() {
			if _, err := shell.Quote(dd.d, []string{"x", "a" + string(r) + "b"}); !errors.Is(err, shell.ErrControl) {
				t.Errorf("%s U+%04X: %v; want ErrControl", dd.name, r, err)
			}
		}
	}
	for _, dd := range dialects() {
		if _, err := shell.Quote(dd.d, []string{"x", "a b"}); err != nil {
			t.Errorf("%s: a plain space is refused: %v", dd.name, err)
		}
	}
}

// An argument that is not valid UTF-8 is refused with ErrNotUTF8, the argument's index and the byte offset, in
// every dialect, and before any other class: Cmd no longer rewrites it to U+FFFD.
func TestQuoteRefusesInvalidUTF8(t *testing.T) {
	t.Parallel()
	for _, arg := range []string{"a\x9bb", "a\x9db", "a\x90b", "a\xffb", "a\xe2\x82", "a\xc2"} {
		for _, dd := range dialects() {
			line, err := shell.Quote(dd.d, []string{"x", arg})
			if !errors.Is(err, shell.ErrNotUTF8) || errors.Is(err, shell.ErrControl) || line != "" ||
				!strings.Contains(err.Error(), "argument 1 ") || !strings.Contains(err.Error(), "byte 1") {
				t.Errorf("%s Quote(%q) = %q, %v; want ErrNotUTF8, argument 1, byte 1", dd.name, arg, line, err)
			}
		}
	}
	if _, err := shell.Quote(shell.POSIX, []string{"x", "\xff\u200b"}); !errors.Is(err, shell.ErrNotUTF8) {
		t.Errorf("invalid UTF-8 before a control: %v; want ErrNotUTF8", err)
	}
}

// A mark (unicode.M, Other_Grapheme_Extend) is refused when it begins an argument or follows U+0020, as the first
// word, a later word and after a space inside one; after any other rune it passes.
//
// What it costs when this fails: a combining mark at the start of a word draws over the quote or the space before it.
func TestQuoteRefusesAMarkAtTheStartOrAfterASpace(t *testing.T) {
	t.Parallel()
	for _, r := range []rune{0x332, 0x336, 0x20d2, 0x20dd} {
		m := string(r)
		for _, dd := range dialects() {
			for name, argv := range map[string][]string{"first word": {m, "x"}, "later word": {"x", m}, "after a space": {"x", "a " + m + "b"}} {
				if _, err := shell.Quote(dd.d, argv); !errors.Is(err, shell.ErrControl) || !strings.Contains(err.Error(), fmt.Sprintf("U+%04X", r)) || strings.ContainsRune(err.Error(), r) {
					t.Errorf("%s U+%04X %s: %v; want ErrControl naming the rune", dd.name, r, name, err)
				}
			}
			if _, err := shell.Quote(dd.d, []string{"x", "e" + m}); err != nil {
				t.Errorf("%s U+%04X after a letter is refused: %v", dd.name, r, err)
			}
		}
	}
}

// An unassigned rune (Cn), the noncharacters among them, is refused; private use (Co) passes, since WSL and Cygwin keep : there.
func TestQuoteRefusesUnassignedAndPassesPrivateUse(t *testing.T) {
	t.Parallel()
	for _, dd := range dialects() {
		for _, r := range []rune{0x378, 0xfffe, 0xfdd0, 0x10ffff} {
			if _, err := shell.Quote(dd.d, []string{"x", "a" + string(r) + "b"}); !errors.Is(err, shell.ErrControl) {
				t.Errorf("%s U+%04X: %v; want ErrControl", dd.name, r, err)
			}
		}
		for _, r := range []rune{0xe000, 0xf03a} {
			if _, err := shell.Quote(dd.d, []string{"x", "a" + string(r) + "b"}); err != nil {
				t.Errorf("%s U+%04X (private use) is refused: %v", dd.name, r, err)
			}
		}
	}
}

// The blanks no category names: each is refused by name, with ErrControl.
func TestQuoteRefusesTheHandListOfBlanks(t *testing.T) {
	t.Parallel()
	for _, r := range []rune{0x2800, 0xfffc, 0x1d159, 0x16fe4, 0x1bc9d, 0x1107f, 0x11f42, 0x13440} {
		for _, dd := range dialects() {
			if _, err := shell.Quote(dd.d, []string{"x", "a" + string(r) + "b"}); !errors.Is(err, shell.ErrControl) || !strings.Contains(err.Error(), fmt.Sprintf("U+%04X", r)) {
				t.Errorf("%s U+%04X: %v; want ErrControl naming it", dd.name, r, err)
			}
		}
	}
}

// The byte offset counts bytes, not runes: after a three-byte U+FFFD the bad byte is at 3.
func TestQuoteNamesTheByteOffsetOfInvalidUTF8(t *testing.T) {
	t.Parallel()
	for _, dd := range dialects() {
		if _, err := shell.Quote(dd.d, []string{"x", "\ufffd\xff"}); !errors.Is(err, shell.ErrNotUTF8) || !strings.Contains(err.Error(), "byte 3") {
			t.Errorf("%s: %v; want ErrNotUTF8 at byte 3", dd.name, err)
		}
	}
}
