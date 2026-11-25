package shell_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"
)

// nasty is argument text each dialect's rule has to carry, or refuse by name.
func nasty() []string {
	return []string{"", "plain", "a b", "two  spaces", `it's`, `'`, `''`, `"`, `a"b`, `\`, `a\`, `a\\`, `a\"b`, `\\"`, `a b\`, "$HOME", "`x`", "a;b", "a&b", "a|b", "<a>", "(a)", "^", "a^b", "~", "*", "--flag=value", "a=b", "é", "‘x’", "‚x‛", "@x", "a,b", "--%", "-"}
}

// Every dialect's line for a few argv, as a person is shown it.
func TestQuoteLines(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		d    shell.Dialect
		argv []string
		want string
	}{
		{"posix bare", shell.POSIX, []string{"git", "--flag=v", "a/b-c.txt"}, "git --flag=v a/b-c.txt"},
		{"posix space and quote", shell.POSIX, []string{"echo", "it's a b", ""}, `echo 'it'\''s a b' ''`},
		{"posix first word with =", shell.POSIX, []string{"a=b", "c=d"}, "'a=b' c=d"},
		{"posix leading = quoted", shell.POSIX, []string{"echo", "=ls"}, "echo '=ls'"},
		{"posix later = bare", shell.POSIX, []string{"echo", "a=b="}, "echo a=b="},
		{"posix dollar and space", shell.POSIX, []string{"echo", "$HOME", "a b"}, "echo '$HOME' 'a b'"},
		{"powershell bare", shell.PowerShell, []string{"git", "--flag", "x=-v"}, "git --flag x=-v"},
		{"powershell dash word quoted", shell.PowerShell, []string{"git", "--flag=v"}, "git '--flag=v'"},
		{"powershell quoted", shell.PowerShell, []string{"echo", "it's", "a,b", "@x", "50%", "C:\\dir\\"}, "echo 'it''s' 'a,b' '@x' '50%' 'C:\\dir\\'"},
		{"powershell curly quotes doubled", shell.PowerShell, []string{"echo", "‘a’‚b‛"}, "echo '‘‘a’’‚‚b‛‛'"},
		{"powershell quoted command", shell.PowerShell, []string{"C:/Program Files/x.exe", "a"}, "& 'C:/Program Files/x.exe' a"},
		{"cmd bare", shell.Cmd, []string{"git", "--flag=v", `C:\a\b`}, `git --flag=v C:\a\b`},
		{"cmd space", shell.Cmd, []string{"echo", "a b", ""}, `echo ^"a b^" ^"^"`},
		{"cmd quote and backslashes", shell.Cmd, []string{"x", `a\"b`, `a b\`}, `x ^"a\\\^"b^" ^"a b\\^"`},
		{"cmd metacharacters", shell.Cmd, []string{"x", "a&b|c<d>e(f)g^h"}, "x a^&b^|c^<d^>e^(f^)g^^h"},
		{"empty argv", shell.POSIX, nil, ""},
	}
	for _, c := range cases {
		got, err := shell.Quote(c.d, c.argv)
		if err != nil || got != c.want {
			t.Errorf("%s: Quote = %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

// What each dialect refuses is named: NUL and a raw newline everywhere, % and ! in cmd alone.
func TestQuoteRefusals(t *testing.T) {
	t.Parallel()
	all := []shell.Dialect{shell.POSIX, shell.PowerShell, shell.Cmd, shell.Windows}
	cases := []struct {
		arg  string
		want error
		in   []shell.Dialect
	}{
		{"a\x00b", shell.ErrNUL, all},
		{"a\nb", shell.ErrNewline, all},
		{"a\rb", shell.ErrNewline, all},
		{"50%", shell.ErrCmdChar, []shell.Dialect{shell.Cmd, shell.Windows}},
		{"hi!", shell.ErrCmdChar, []shell.Dialect{shell.Cmd, shell.Windows}},
	}
	for _, c := range cases {
		for _, d := range all {
			line, err := shell.Quote(d, []string{"ok", c.arg})
			refused := err != nil
			if want := slices.Contains(c.in, d); refused != want || (refused && (!errors.Is(err, c.want) || line != "")) {
				t.Errorf("Quote(%d, %q) = %q, %v; refused %v, want %v of %v", d, c.arg, line, err, refused, want, c.want)
			}
			if refused && !strings.Contains(err.Error(), "argument 1") {
				t.Errorf("Quote(%d, %q) error %q does not name the argument", d, c.arg, err)
			}
		}
	}
	for _, d := range []shell.Dialect{0, shell.Windows + 1} {
		if _, err := shell.Quote(d, []string{"a"}); !errors.Is(err, shell.ErrDialect) {
			t.Errorf("Quote(%d) = %v, want ErrDialect", d, err)
		}
	}
}

// readCmd is the line as cmd hands it to a program, carets taken out, split by the C runtime's argv rule.
func readCmd(line string) []string {
	var raw strings.Builder
	for i := 0; i < len(line); i++ {
		if line[i] == '^' && i+1 < len(line) {
			i++
		}
		raw.WriteByte(line[i])
	}
	s := raw.String()
	var out []string
	for i := 0; i < len(s); {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}
		var w strings.Builder
		in := false
		for ; i < len(s); i++ {
			slashes := 0
			for i < len(s) && s[i] == '\\' {
				slashes++
				i++
			}
			if i < len(s) && s[i] == '"' {
				w.WriteString(strings.Repeat(`\`, slashes/2))
				if slashes%2 == 1 {
					w.WriteByte('"')
				} else {
					in = !in
				}
				continue
			}
			w.WriteString(strings.Repeat(`\`, slashes))
			if i >= len(s) || ((s[i] == ' ' || s[i] == '\t') && !in) {
				break
			}
			w.WriteByte(s[i])
		}
		out = append(out, w.String())
	}
	return out
}

// A cmd line, with its carets taken out, is the argv rule's reading of the argument it was made from.
func TestQuoteCmdReadBackByArgvRule(t *testing.T) {
	t.Parallel()
	for _, a := range cmdSafe() {
		line, err := shell.Quote(shell.Cmd, []string{"x", a, "y"})
		if err != nil {
			t.Fatal(err)
		}
		if got := readCmd(line); len(got) != 3 || got[1] != a {
			t.Errorf("argv rule read %q from %q, want %q", got, line, a)
		}
	}
}

// Every metacharacter cmd reads is behind a caret, so none is left to run, redirect or pipe.
func TestQuoteCmdLeavesNoBareMetacharacter(t *testing.T) {
	t.Parallel()
	for _, a := range cmdSafe() {
		line, err := shell.Quote(shell.Cmd, []string{"x", a})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < len(line); i++ {
			switch line[i] {
			case '^':
				i++
			case '&', '|', '<', '>', '(', ')', '"':
				t.Errorf("%q: a bare %q at %d", line, line[i], i)
			}
		}
	}
}

// cmdSafe is nasty without what cmd refuses.
func cmdSafe() []string {
	return slices.DeleteFunc(nasty(), func(a string) bool { return strings.ContainsAny(a, "%!") })
}
