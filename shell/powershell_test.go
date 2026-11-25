package shell_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"
)

// What PowerShell refuses by name (ErrPowerShellArg), the argument's index in the error, and what it lets through.
//
// What it costs when this fails: a line pasted into Windows PowerShell 5.1 gives a program another argv than the one
// quoted: a value splits into words and adds flags, or an empty word is dropped and the later ones move up.
func TestQuotePowerShellRefusals(t *testing.T) {
	t.Parallel()
	refused := []struct {
		name string
		argv []string
		idx  int
	}{
		{"a double quote", []string{"x", `a"b`}, 1},
		{"a value that adds flags", []string{"x", `x" --apply "y`}, 1},
		{"a lone double quote", []string{"x", `"`}, 1},
		{"a left curly double quote", []string{"x", "a\u201cb"}, 1},
		{"a right curly double quote", []string{"x", "a\u201db"}, 1},
		{"a low curly double quote", []string{"x", "a\u201eb"}, 1},
		{"a high reversed curly double quote", []string{"x", "a\u201fb"}, 1},
		{"an empty word in the middle", []string{"x", "--owner", "", "--apply"}, 2},
		{"an empty first word", []string{"", "x"}, 0},
		{"an empty last word", []string{"x", ""}, 1},
		{"a space and a trailing backslash", []string{"x", `C:\Program Files\`}, 1},
		{"the stop-parsing token", []string{"x", "--%"}, 1},
		{"the stop-parsing token first", []string{"--%", "x"}, 0},
		{"two trailing backslashes after a space", []string{"x", `a b\\`}, 1},
		{"the first word too", []string{`C:\Program Files\`, "a"}, 0},
	}
	for _, c := range refused {
		line, err := shell.Quote(shell.PowerShell, c.argv)
		if !errors.Is(err, shell.ErrPowerShellArg) || line != "" {
			t.Errorf("%s: Quote(%q) = %q, %v; want ErrPowerShellArg", c.name, c.argv, line, err)
			continue
		}
		if want := "argument " + string(rune('0'+c.idx)) + " "; !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error %q does not say %q", c.name, err, want)
		}
	}
	for _, a := range []string{`C:\dir\`, `a\b`, `a b`, `it's`, "\u2018x\u2019", "\u201ax\u201b", "$x", "`", ";", "&", "|", `a\\`, `a\`, "é", "--%x", "x--%", "-%", "--%%"} {
		if _, err := shell.Quote(shell.PowerShell, []string{"x", a}); err != nil {
			t.Errorf("Quote(PowerShell, %q) is refused: %v", a, err)
		}
	}
}

// spaces is every blank rune that is not U+0020: the no-break space, the Ogham space mark, the en quad to the hair space, the narrow no-break space, the medium mathematical space, the ideographic space and the blank braille U+2800.
func spaces() []rune {
	out := []rune{0xA0, 0x1680}
	for r := rune(0x2000); r <= 0x200A; r++ {
		out = append(out, r)
	}
	return append(out, 0x202F, 0x205F, 0x3000, 0x2800)
}

// Every blank but U+0020 is refused with ErrControl, with a trailing backslash or without, so the whitespace rule of ErrPowerShellArg is left to the space alone.
func TestQuotePowerShellRefusesEveryBlankButTheSpace(t *testing.T) {
	t.Parallel()
	for _, r := range spaces() {
		for _, arg := range []string{"a" + string(r) + "b\\", "a" + string(r) + "b"} {
			if line, err := shell.Quote(shell.PowerShell, []string{"x", arg}); !errors.Is(err, shell.ErrControl) || line != "" {
				t.Errorf("U+%04X: Quote(%q) = %q, %v; want ErrControl", r, arg, line, err)
			}
		}
	}
}

// A bare word that begins with - and holds a . or ends in a : is quoted, since 5.1 can split it (PowerShell#6291); every other bare word stays bare.
func TestQuotePowerShellQuotesADashWordWithADot(t *testing.T) {
	t.Parallel()
	for arg, want := range map[string]string{"-a.b": "'-a.b'", "--x=a.b": "'--x=a.b'", "-o:-x:--apply": "'-o:-x:--apply'", "-a:-b:c": "'-a:-b:c'", "-a:b": "'-a:b'", "-a:--": "'-a:--'", "--flag=value": "'--flag=value'", "-x:": "'-x:'", "-:": "'-:'", "-a/b": "'-a/b'", "-a+b": "'-a+b'", "-a_b": "'-a_b'", "\u2013x": "'\u2013x'", "\u2014x": "'\u2014x'", "\u2015x": "'\u2015x'", "-\u00e9": "'-\u00e9'", "-x": "-x", "--apply": "--apply", "--dry-run": "--dry-run", "-1": "-1", "-": "-", "a:": "a:", "a.b": "a.b", "a-b.c": "a-b.c", "x=-a:b": "x=-a:b"} {
		if got, err := shell.Quote(shell.PowerShell, []string{"x", arg}); err != nil || got != "x "+want {
			t.Errorf("Quote(%q) = %q, %v; want %q", arg, got, err, "x "+want)
		}
	}
}

// The refusal is PowerShell's alone: POSIX and cmd carry the same words as before.
func TestQuotePowerShellRefusalIsPowerShellsAlone(t *testing.T) {
	t.Parallel()
	for _, a := range []string{`a"b`, "", `C:\Program Files\`, "a\u201cb"} {
		for _, d := range []shell.Dialect{shell.POSIX, shell.Cmd} {
			if _, err := shell.Quote(d, []string{"x", a}); errors.Is(err, shell.ErrPowerShellArg) {
				t.Errorf("Quote(%v, %q) refuses with ErrPowerShellArg", d, a)
			}
		}
	}
}
