package shell_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"
)

func splitDialects() []named {
	return []named{{"posix", shell.POSIX}, {"powershell", shell.PowerShell}, {"windows", shell.Windows}}
}

// Split gives back the argv that Quote wrote, for words plain, spaced, quoted, empty and holding a quote.
//
// What it costs when this fails: a program that reads back a line it printed gets another argv than it wrote.
func TestSplitReadsWhatQuoteWrote(t *testing.T) {
	t.Parallel()
	for _, dd := range splitDialects() {
		for _, argv := range [][]string{
			nil, {"git"}, {"echo", "a b", "it's", "x=y", "=x", "$HOME", "a;b", "*.go", "-x:y", "‘q’", "é世"},
			{"a b", "c"}, {"x", "back\\slash", "trail\\"}, {"x", ""}, {"x", "50%", "a!b"}, {"--%x"},
		} {
			line, err := shell.Quote(dd.d, argv)
			if err != nil {
				continue // a refusal of Quote's, not Split's
			}
			got, err := shell.Split(dd.d, line)
			if err != nil || (len(got) != len(argv) || len(argv) > 0 && !reflect.DeepEqual(got, argv)) {
				t.Errorf("%s: Split(%q) = %q, %v; want %q", dd.name, line, got, err, argv)
			}
		}
	}
}

// Each line Quote never writes is ErrNotQuoted, per dialect, and names the byte offset of what is wrong.
//
// What it costs when this fails: Split reads a line a shell would read another way (an expansion, a second command, a
// glob) as if it were plain words.
func TestSplitRefusesWhatQuoteNeverWrites(t *testing.T) {
	t.Parallel()
	rows := []struct {
		name  string
		d     shell.Dialect
		line  string
		offAt string // the byte offset the error names
	}{
		{"posix unclosed quote", shell.POSIX, "echo 'a b", "byte 5"},
		{"posix operator", shell.POSIX, "a && b", "byte 2"},
		{"posix pipe", shell.POSIX, "a | b", "byte 2"},
		{"posix dollar", shell.POSIX, "echo $HOME", "byte 5"},
		{"posix backtick", shell.POSIX, "echo `id`", "byte 5"},
		{"posix glob", shell.POSIX, "ls *.go", "byte 3"},
		{"posix comment", shell.POSIX, "ls # x", "byte 3"},
		{"posix double quotes", shell.POSIX, `echo "a"`, "byte 5"},
		{"posix backslash form", shell.POSIX, `echo a\ b`, "byte 5"},
		{"posix quoted but plain", shell.POSIX, "echo 'a'", "byte 5"},
		{"posix two spaces", shell.POSIX, "a  b", "byte 2"},
		{"posix leading space", shell.POSIX, " a", "byte 0"},
		{"posix trailing space", shell.POSIX, "a ", "byte 1"},
		{"posix a word Quote refuses", shell.POSIX, "echo 'a\tb'", "byte 5"},
		{"posix a newline", shell.POSIX, "echo a\nb", "byte 5"},
		{"posix not UTF-8", shell.POSIX, "echo '\xff'", "byte 5"},
		{"powershell unclosed", shell.PowerShell, "echo 'a b", "byte 5"},
		{"powershell semicolon", shell.PowerShell, "a; b", "byte 0"},
		{"powershell dollar", shell.PowerShell, "echo $x", "byte 5"},
		{"powershell double quotes", shell.PowerShell, `echo "a"`, "byte 5"},
		{"powershell backtick", shell.PowerShell, "echo a`b", "byte 5"},
		{"powershell lone curly quote", shell.PowerShell, "echo 'a‘b'", "byte 7"},
		{"powershell call operator on a bare word", shell.PowerShell, "& git", "byte 0"},
		{"powershell comma", shell.PowerShell, "echo a,b", "byte 5"},
		{"windows unclosed", shell.Windows, `echo "a b`, "byte 5"},
		{"windows ampersand", shell.Windows, "a & b", "byte 2"},
		{"windows dollar", shell.Windows, `echo "$x"`, "byte 5"},
		{"windows single quotes", shell.Windows, "echo 'a b'", "byte 5"},
		{"windows percent", shell.Windows, `echo "50%"`, "byte 5"},
		{"windows caret", shell.Windows, "echo a^b", "byte 5"},
	}
	for _, r := range rows {
		got, err := shell.Split(r.d, r.line)
		if !errors.Is(err, shell.ErrNotQuoted) || got != nil || !strings.Contains(err.Error(), r.offAt) {
			t.Errorf("%s: Split(%q) = %q, %v; want ErrNotQuoted naming %s", r.name, r.line, got, err, r.offAt)
		}
	}
}

// Cmd and a value that is no dialect are ErrDialect.
func TestSplitReadsNoCmdAndNoOtherDialect(t *testing.T) {
	t.Parallel()
	for _, d := range []shell.Dialect{shell.Cmd, 0, 9} {
		if _, err := shell.Split(d, "a"); !errors.Is(err, shell.ErrDialect) {
			t.Errorf("Split(%v) = %v; want ErrDialect", d, err)
		}
	}
}

// Wherever Quote accepts an argv, Split gives it back, and whenever Split accepts a line Quote writes that line.
func FuzzSplitInvertsQuote(f *testing.F) {
	for _, s := range []string{"", "a", "a b", "it's", "$x", "x'y'z", "‘", "=x", "-a:b", "\\", "a\\", "'", "\"", "50%"} {
		f.Add(s, "echo")
	}
	f.Fuzz(func(t *testing.T, a, b string) {
		for _, dd := range splitDialects() {
			if line, err := shell.Quote(dd.d, []string{b, a}); err == nil {
				got, err := shell.Split(dd.d, line)
				if err != nil || len(got) != 2 || got[0] != b || got[1] != a {
					t.Fatalf("%s: Split(%q) = %q, %v; want [%q %q]", dd.name, line, got, err, b, a)
				}
			}
			if got, err := shell.Split(dd.d, a); err == nil {
				if back, qerr := shell.Quote(dd.d, got); qerr != nil || back != a {
					t.Fatalf("%s: Split accepted %q as %q, and Quote writes %q, %v", dd.name, a, got, back, qerr)
				}
			}
		}
	})
}
