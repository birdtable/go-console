package shell_test

import (
	"errors"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/birdtable/go-console/shell"
)

// isControlRefusal is whether Quote refuses the argument s with ErrControl.
func isControlRefusal(d shell.Dialect, s string) bool {
	_, err := shell.Quote(d, []string{s})
	return errors.Is(err, shell.ErrControl)
}

// Every rune class that Invisible, the word-start rule and the Hangul jamo name is not Shown, and what is none of them is.
//
// What it costs when this fails: a program that names a value in prose with Shown says "shown as written" of a value
// that Quote refuses, or hides a value that Quote would print.
func TestShownAndInvisibleNameTheRuneClasses(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		r    rune
		inv  bool // Invisible by itself
		word bool // Shown when it begins an argument
	}{
		{"control, tab", '\t', true, false},
		{"control, ESC", 0x1b, true, false},
		{"control, DEL", 0x7f, true, false},
		{"control, C1", 0x85, true, false},
		{"NUL", 0, true, false},
		{"LF", '\n', true, false},
		{"CR", '\r', true, false},
		{"format, zero-width space", 0x200b, true, false},
		{"format, soft hyphen", 0xad, true, false},
		{"format, bidi override", 0x202e, true, false},
		{"format, zero-width joiner", 0x200d, true, false},
		{"line separator", 0x2028, true, false},
		{"paragraph separator", 0x2029, true, false},
		{"default-ignorable", 0x180e, true, false},
		{"variation selector", 0xfe0f, true, false},
		{"no-break space", 0xa0, true, false},
		{"hair space", 0x200a, true, false},
		{"blank braille", 0x2800, true, false},
		{"object replacement", 0xfffc, true, false},
		{"unassigned", 0x378, true, false},
		{"noncharacter", 0xfffe, true, false},
		{"combining mark", 0x301, false, false},
		{"other grapheme extend", 0x9be, false, false},
		{"hangul medial jamo", 0x1161, false, false},
		{"hangul final jamo", 0x11a8, false, false},
		{"hangul extended-B jamo", 0xd7b0, false, false},
		{"space", ' ', false, true},
		{"letter", 'a', false, true},
		{"private use", 0xe000, false, true},
		{"emoji", 0x1f600, false, true},
		{"replacement character", utf8.RuneError, false, true},
	} {
		if got := shell.Invisible(c.r); got != c.inv {
			t.Errorf("%s U+%04X: Invisible = %v, want %v", c.name, c.r, got, c.inv)
		}
		r := string(c.r)
		if got := shell.Shown(r); got != c.word {
			t.Errorf("%s U+%04X: Shown(%q) = %v, want %v", c.name, c.r, r, got, c.word)
		}
		if got := shell.Shown("a" + r + "b"); got != !c.inv {
			t.Errorf("%s U+%04X: Shown in a word = %v, want %v", c.name, c.r, got, !c.inv)
		}
		if got := shell.Shown("a " + r); got != c.word {
			t.Errorf("%s U+%04X: Shown after a space = %v, want %v", c.name, c.r, got, c.word)
		}
	}
	if !shell.Shown("") || !shell.Shown("a\u0301") || shell.Shown("\u0301a") || shell.Shown("a \u0301") || shell.Shown("a \u1161") || !shell.Shown("a\u1161") {
		t.Error(`Shown of the empty text and of a mark after a letter, at the start and after a space is not as Quote has it`)
	}
}

// For every rune, alone, in a word, at the start and after a space, Shown is what Quote answers with ErrControl, in
// every dialect, and Invisible is what Shown says of the rune in a word.
//
// What it costs when this fails: Shown and Quote differ, so a caller that tests with one is told another thing than the
// line it prints with the other.
func TestShownIsWhatQuoteAnswersWithErrControlForEveryRune(t *testing.T) {
	t.Parallel()
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if r >= 0xd800 && r <= 0xdfff || r >= 0x3400 && r < 0xf000 && r%7 != 0 || r >= 0x10000 && r < 0xe0000 && r%7 != 0 || r >= 0xe1000 && r%7 != 0 {
			continue // the race run is slow: the planes with no format or blank rune are taken on a stride
		}
		for _, s := range []string{string(r), "a" + string(r) + "b", " " + string(r), "a " + string(r)} {
			if strings.ContainsAny(s, "\x00\n\r") {
				if shell.Shown(s) {
					t.Fatalf("Shown(%q) is true, and the text holds a NUL, an LF or a CR", s)
				}
				continue
			}
			// Every rune in POSIX; the other dialects share the check, so they take the low planes and a stride of the rest.
			ds := dialects()[:1]
			if r < 0x3400 || r%97 == 0 {
				ds = dialects()
			}
			for _, dd := range ds {
				if got, want := shell.Shown(s), !isControlRefusal(dd.d, s); got != want {
					t.Fatalf("%s: Shown(%q) = %v, and Quote answers ErrControl = %v", dd.name, s, got, !want)
				}
			}
		}
		if got := shell.Shown("a" + string(r) + "b"); got != !shell.Invisible(r) {
			t.Fatalf("U+%04X: Shown in a word = %v, Invisible = %v", r, got, shell.Invisible(r))
		}
	}
}

// A refusal that is not ErrControl does not make a text not Shown, and one that holds a hidden rune does.
func TestShownSaysNothingOfTheOtherRefusals(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"say \"hi\"", "50%", "a\\", "--%", "\uff02", "", "a b"} {
		if !shell.Shown(s) {
			t.Errorf("Shown(%q) is false; no hidden rune is in it", s)
		}
	}
	if shell.Shown("bad\xff\u200b") {
		t.Error("a text that is not UTF-8 and holds a zero-width space is Shown")
	}
}

func FuzzShownIsWhatQuoteAnswersWithErrControl(f *testing.F) {
	for _, s := range []string{"", "a", "a\u200bb", "\u0301", "a\u0301", " \u0301", "\x1b[31m", "\u1161", "a b", "\uff02", "\xff", "a\x00b", "a\nb", "\U0001f600"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if strings.ContainsAny(s, "\x00\n\r") || !utf8.ValidString(s) {
			return // Quote answers these before it looks for a hidden rune
		}
		for _, dd := range dialects() {
			if got, want := shell.Shown(s), !isControlRefusal(dd.d, s); got != want {
				t.Fatalf("%s: Shown(%q) = %v, and Quote answers ErrControl = %v", dd.name, s, got, !want)
			}
		}
	})
}
