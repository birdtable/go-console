package ui

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// stripCases are strings that carry something a terminal would act on, and what
// Strip leaves of each.
func stripCases() map[string]string {
	return map[string]string{
		"plain words\tand a line\nbreak":                 "plain words\tand a line\nbreak",
		"\x1b[31mred\x1b[0m":                             "red",
		"\x1b[1;38;2;1;2;3mbold\x1b[m":                   "bold",
		"a\x1b[2Ab":                                      "ab",
		"a\x1b[2J\x1b[Hb":                                "ab",
		"\x1b]0;a title\x07after":                        "after",
		"\x1b]8;;https://x.test\x1b\\link\x1b]8;;\x1b\\": "link",
		"bare\x1bescape":                                 "barescape",
		"c1\u009bcontrol":                                "c1control",
		"nul\x00bell\x07":                                "nulbell",
		"cr\rline":                                       "crline",
		"bad\xffbyte":                                    "badbyte",
		"accents é and 🙂":                                "accents é and 🙂",
		"\x1bPdevice string\x1b\\x":                      "x",
	}
}

// Strip leaves no byte a terminal would act on, whatever the text holds.
//
// What it costs when this fails: text from outside written through Strip still
// moves the cursor, retitles the window or paints what follows.
func TestStripLeavesNoControl(t *testing.T) {
	t.Parallel()
	ran := 0
	for in, want := range stripCases() {
		ran++
		got := Strip(in)
		if got != want {
			t.Errorf("Strip(%q) = %q, want %q", in, got, want)
		}
		checkClean(t, in, got)
	}
	if ran == 0 {
		t.Fatal("no case ran")
	}
}

// checkClean fails when out holds a control character but a newline or a tab, or
// is not UTF-8.
func checkClean(t *testing.T, in, out string) {
	t.Helper()
	if !utf8.ValidString(out) {
		t.Errorf("Strip(%q) = %q, not UTF-8", in, out)
	}
	for _, r := range out {
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			t.Errorf("Strip(%q) = %q, holds control %U", in, out, r)
		}
	}
	if strings.ContainsRune(out, 0x1b) {
		t.Errorf("Strip(%q) = %q, holds an escape", in, out)
	}
}

// FuzzStripLeavesNoControl runs its seeds under go test; it is not fuzzed here.
func FuzzStripLeavesNoControl(f *testing.F) {
	for in := range stripCases() {
		f.Add(in)
	}
	f.Fuzz(func(t *testing.T, in string) {
		checkClean(t, in, Strip(in))
	})
}
