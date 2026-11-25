package ui

import (
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/birdtable/go-console/shell"
)

// Each character a name can hide behind is quoted as its number: the bidi
// controls, the zero-width ones, the byte-order mark, the line and paragraph separators, a
// joiner between letters, a soft hyphen and a tag; a joiner at either end of a name, beside a
// byte that is not UTF-8 or after the replacement character, none of which is part of an emoji;
// a variation selector after a letter; the other characters a display may ignore; and a byte
// that is not UTF-8, which is never written back as U+FFFD. The controls below 0x80 take two digits and those above take \u, so
// no quoted name reads as another.
//
// What it costs when this fails: a file named `evil` + U+202E + `txt.exe` is listed as
// evilexe.txt, two names that differ by a zero-width space look the same, or a U+2028 in a name
// breaks a log line in two as a newline would.
func TestANameQuotesEveryCharacterThatHidesWhatItSays(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, in, want string }{
		{"a right-to-left override", "evil\u202etxt.exe", `evil\u202etxt.exe`},
		{"a left-to-right isolate", "a\u2066b", `a\u2066b`},
		{"a zero-width space", "pay\u200bpal", `pay\u200bpal`},
		{"a byte-order mark", "\ufeffname", `\ufeffname`},
		{"a line separator", "one\u2028two", `one\u2028two`},
		{"a paragraph separator", "one\u2029two", `one\u2029two`},
		{"a joiner between letters", "pay\u200dpal", `pay\u200dpal`},
		{"a soft hyphen", "ba\u00adnk", `ba\u00adnk`},
		{"a tag character", "a\U000e0041", `a\U000e0041`},
		{"a byte that is not UTF-8", "a\xffb", `a\xffb`},
		{"a bell, in two digits", "a\af", `a\x07f`},
		{"a C1 control", "a\u009bb", `a\u009bb`},
		{"a joiner first", "\u200d\u2122report", `\u200d` + "\u2122report"},
		{"a joiner last", "invoice\u00ae\u200d", "invoice\u00ae" + `\u200d`},
		{"a joiner beside a byte that is not UTF-8", "\u00a9\u200d\xff", "\u00a9" + `\u200d\xff`},
		{"a joiner after the replacement character", "\ufffd\u200d\u00a9", "\ufffd" + `\u200d` + "\u00a9"},
		{"a picture selector after a letter", "pay\ufe0fpal", `pay\ufe0fpal`},
		{"a picture selector and a joiner after a letter", "a\ufe0f\u200d\u00a9", `a\ufe0f\u200d` + "\u00a9"},
		{"a text selector after a letter", "a\ufe0eb", `a\ufe0eb`},
		{"a combining grapheme joiner", "a\u034fb", `a\u034fb`},
		{"a Mongolian variation selector", "a\u180bb", `a\u180bb`},
		{"an ideographic variation selector", "a\U000e0100b", `a\U000e0100b`},
		{"a Hangul filler", "a\u3164b", `a\u3164b`},
		{"a keycap's selector with no keycap after it", "1\ufe0fx", `1\ufe0fx`},
	}
	ran := 0
	for _, c := range cases {
		if got := Name(c.in); got != c.want {
			t.Errorf("%s: Name(%q) = %q, and want %q", c.name, c.in, got, c.want)
		}
		ran++
	}
	if ran != len(cases) {
		t.Fatalf("ran %d of %d cases", ran, len(cases))
	}
	if Name("\ufffd") == Name("\xff") {
		t.Errorf("a U+FFFD and a byte that is not UTF-8 both print as %q", Name("\ufffd"))
	}
}

// An ordinary name prints exactly as it is: accents, an emoji, the emoji built with a
// zero-width joiner (a family, a flag, a skin tone), whose joiner sits between two parts of a
// picture and is kept, and the variation selectors an emoji or a keycap is drawn with.
//
// What it costs when this fails: a name a person typed in their own language, or with an emoji in
// it, prints as a row of numbers, and the program looks broken to everybody it was not attacking.
func TestAnOrdinaryNameWithAccentsOrEmojiPrintsAsItIs(t *testing.T) {
	t.Parallel()
	names := []string{
		"café résumé.txt",
		"naïve 😀 notes",
		"family 👨\u200d👩\u200d👧",
		"pride 🏳\ufe0f\u200d🌈",
		"coder 👩🏽\u200d💻",
		"\ufffd is a character",
		"key 1\ufe0f\u20e3 and #\ufe0f\u20e3 and *\ufe0f\u20e3",
		"love \u2764\ufe0f",
		"smile as text \u263a\ufe0e",
	}
	kept := 0
	for _, n := range names {
		if got := Name(n); got != n {
			t.Errorf("Name(%q) = %q, and want it as it is", n, got)
		}
		kept++
	}
	if kept != len(names) {
		t.Fatalf("looked at %d of %d names", kept, len(names))
	}
}

// A followed line drops what Name quotes, an emoji's joiner and selector kept and a
// tab kept: a guest's line cannot reorder itself, break itself in two or hide a character.
//
// What it costs when this fails: a VM's console writes a line that shows itself backwards, or
// splits into two lines in the follow's scrollback, as though the program had said the second.
func TestAFollowedLineDropsWhatHidesWhatItSays(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"evil\u202etxt.exe", "eviltxt.exe"},
		{"one\u2028two\u2029three", "onetwothree"},
		{"pay\u200bpal\u200dx", "paypalx"},
		{"family 👨\u200d👩\u200d👧\tok", "family 👨\u200d👩\u200d👧\tok"},
		{"\u200d\u2122report", "\u2122report"},
		{"invoice\u00ae\u200d", "invoice\u00ae"},
		{"\u00a9\u200d\xff", "\u00a9"},
		{"pay\ufe0fpal and a\u3164b", "paypal and ab"},
		{"key 1\ufe0f\u20e3", "key 1\ufe0f\u20e3"},
	}
	ran := 0
	for _, c := range cases {
		if got := safeLine(c.in); got != c.want {
			t.Errorf("safeLine(%q) = %q, and want %q", c.in, got, c.want)
		}
		ran++
	}
	if ran != len(cases) {
		t.Fatalf("ran %d of %d cases", ran, len(cases))
	}
}

// A backslash is left as it is: a Windows path prints with its separators
// as they are, and a name that holds the text of a quote prints that text, not what the
// quote would stand for.
//
// What it costs when this fails: every Windows path a program prints shows its separators
// doubled, C:\\Users\\ana, and a path a person copies from the screen no longer names
// the file.
func TestANameKeepsItsBackslashesAsTheyAre(t *testing.T) {
	t.Parallel()
	names := []string{
		`C:\Users\ana\notes.txt`,
		`\\server\share\report.pdf`,
		`a\x07b`,
		`trailing\`,
	}
	kept := 0
	for _, n := range names {
		if needsEscaping(n) {
			t.Errorf("needsEscaping(%q) is true, and a backslash needs no quoting", n)
		}
		if got := Name(n); got != n {
			t.Errorf("Name(%q) = %q, and want it as it is", n, got)
		}
		kept++
	}
	if kept != len(names) {
		t.Fatalf("looked at %d of %d names", kept, len(names))
	}
}

// A rune is hidden by the set shell.Invisible holds, the one Quote refuses with ErrControl, and by no set of
// ui's own: for every rune in a plain text, Name quotes it exactly when shell.Invisible says it is invisible,
// and the classes the old set left out are named (an unassigned rune, a space that is not U+0020, the
// runes that draw as blank). The emoji's joiner and selectors are the one exception, written below, and
// each stays quoted where no emoji is.
//
// What it costs when this fails: a name Name prints as it stands is one Quote refuses, a caller cannot tell
// which rule a call applies, and a name made of a no-break space or a blank-drawing rune passes as the empty
// one it looks like.
func TestANameHidesWhatShellInvisibleHidesAndNothingElse(t *testing.T) {
	t.Parallel()
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if r == utf8.RuneError || (r >= 0xd800 && r <= 0xdfff) {
			continue
		}
		in := "a" + string(r) + "b"
		if quoted := Name(in) != in; quoted != shell.Invisible(r) {
			t.Fatalf("U+%04X: Name quotes it %v, shell.Invisible says %v", r, quoted, shell.Invisible(r))
		}
	}
	for _, c := range []struct{ name, in string }{
		{"an unassigned rune", "a\u0378b"},
		{"a no-break space", "a\u00a0b"},
		{"an ideographic space", "a\u3000b"},
		{"a braille blank", "a\u2800b"},
		{"an object replacement", "a\ufffcb"},
	} {
		if Name(c.in) == c.in {
			t.Errorf("%s: Name(%q) printed it as it stands", c.name, c.in)
		}
		if safeLine(c.in) == c.in {
			t.Errorf("%s: a followed line kept %q", c.name, c.in)
		}
	}
}

// The emoji exception: a joiner between two pictures and the selectors an emoji or a keycap is drawn with
// stay, though shell.Invisible holds all of them, and each is quoted where no emoji is around it.
//
// What it costs when this fails: a family, a flag or a keycap in a name prints as a row of numbers.
func TestTheEmojiJoinerAndSelectorsAreTheOneExceptionToShellInvisible(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"👨\u200d👩", "1\ufe0f\u20e3", "\u00a9\ufe0f", "\u00a9\ufe0e"} {
		if Name(in) != in {
			t.Errorf("Name(%q) = %q, and the emoji is kept as it is", in, Name(in))
		}
	}
	for _, r := range []rune{0x200d, 0xfe0e, 0xfe0f} {
		if !shell.Invisible(r) {
			t.Errorf("U+%04X is not in shell.Invisible: the exception is no longer one", r)
		}
		if in := "a" + string(r) + "b"; Name(in) == in {
			t.Errorf("U+%04X kept between letters", r)
		}
	}
}
