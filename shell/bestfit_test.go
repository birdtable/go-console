package shell

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Each rune of the best-fit table, alone and inside a word, is refused in Cmd and PowerShell, with the argument's index
// and the rune as U+XXXX and never the rune itself, and POSIX passes it. A rune ErrControl covers (the spaces of U+2000
// to U+200A among them) answers ErrControl, which is checked first, and a curly double quote in PowerShell answers
// ErrPowerShellArg, which keeps its own error.
//
// What it costs when this fails: a program that reads its command line as ANSI (main, GetCommandLineA) is given U+FF02
// as a quote, so a value from outside ends a word and adds words to a pasted line.
func TestQuoteRefusesEveryRuneOfTheBestFitTable(t *testing.T) {
	t.Parallel()
	if len(bestFitRunes) == 0 {
		t.Fatal("the table is empty")
	}
	for _, r := range bestFitRunes {
		want := fmt.Sprintf("U+%04X", r)
		for _, arg := range []string{string(r), "a" + string(r) + "b"} {
			for _, d := range []Dialect{Cmd, PowerShell} {
				line, err := Quote(d, []string{"x", arg})
				kind := ErrBestFit
				if hasHidden(arg) {
					kind = ErrControl
				} else if d == PowerShell && powerShellWrong(arg) {
					kind = ErrPowerShellArg
				}
				if !errors.Is(err, kind) || line != "" || !strings.Contains(err.Error(), "argument 1 ") || (!errors.Is(kind, ErrPowerShellArg) && !strings.Contains(err.Error(), want)) || strings.ContainsRune(err.Error(), r) {
					t.Errorf("%v %s: Quote(%q) = %q, %v; want %v, argument 1, %s and not the rune", d, want, arg, line, err, kind, want)
				}
			}
			if _, err := Quote(POSIX, []string{"x", arg}); err != nil && !hasHidden(arg) {
				t.Errorf("posix %s: %v; want it passed", want, err)
			}
		}
		if _, err := Quote(Cmd, []string{string(r), "x"}); err == nil || !strings.Contains(err.Error(), "argument 0 ") {
			t.Errorf("cmd %s first: %v; want a refusal of argument 0", want, err)
		}
	}
}

// The rune that names the class: U+FF02 and U+FF3C are refused with ErrBestFit (neither is hidden), the line v0.14.1
// wrote for the measured argument no longer comes, and the refusal is the same in both dialects.
func TestQuoteRefusesTheFullwidthQuoteAndBackslash(t *testing.T) {
	t.Parallel()
	for _, arg := range []string{"a＂ --evil ＂b", "a＼b"} {
		for _, d := range []Dialect{Cmd, PowerShell} {
			if _, err := Quote(d, []string{`.\argvmain.exe`, arg}); !errors.Is(err, ErrBestFit) {
				t.Errorf("%v %q: %v; want ErrBestFit", d, arg, err)
			}
		}
		if _, err := Quote(POSIX, []string{"argvmain", arg}); err != nil {
			t.Errorf("posix %q: %v; want it passed", arg, err)
		}
	}
}

// Fullwidth punctuation as a whole is everyday CJK and passes: U+FF01 and U+FF08 in all three dialects.
func TestQuotePassesOtherFullwidthPunctuation(t *testing.T) {
	t.Parallel()
	for _, r := range []rune{0xFF01, 0xFF08} {
		if len(string(r)) == 0 {
			continue
		}
		for _, d := range []Dialect{POSIX, PowerShell, Cmd} {
			if _, err := Quote(d, []string{"x", "a" + string(r) + "b", string(r)}); err != nil {
				t.Errorf("%v U+%04X: %v; want it passed", d, r, err)
			}
		}
	}
}

// A Hangul medial or final jamo that begins an argument or follows U+0020 is ErrControl in every dialect; after U+1100
// it joins the letter and passes.
func TestQuoteRefusesAJamoThatBeginsAnArgumentOrFollowsASpace(t *testing.T) {
	t.Parallel()
	for _, r := range []rune{0x1161, 0x11FF, 0xD7B0, 0xD7FF} {
		for _, d := range []Dialect{POSIX, PowerShell, Cmd} {
			for _, arg := range []string{string(r), "a " + string(r)} {
				if _, err := Quote(d, []string{"x", arg}); !errors.Is(err, ErrControl) {
					t.Errorf("%v U+%04X in %q: %v; want ErrControl", d, r, arg, err)
				}
			}
			if r == 0xD7FF {
				continue // unassigned, so ErrControl wherever it sits
			}
			if _, err := Quote(d, []string{"x", "ᄀ" + string(r)}); err != nil {
				t.Errorf("%v U+1100 then U+%04X: %v; want it passed", d, r, err)
			}
		}
	}
}
