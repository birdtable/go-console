package shell_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"
)

// The line Quote writes for Windows is one command that cmd and PowerShell read the same: a plain word bare, any other in double
// quotes with nothing escaped, and a two-step fix is two lines, since Quote never joins two commands.
//
// What it costs when this fails: a fix line a person pastes into an unknown Windows shell splits at a ; & or | in a folder's name,
// and the part after it is a second command the folder's author chose.
func TestQuoteWindowsIsOneLineForBothShells(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		argv []string
		want string
	}{
		{"a semicolon in a path", []string{"icacls", `C:\w\a;b\x`, "/reset", "/L"}, `icacls "C:\w\a;b\x" /reset /L`},
		{"an ampersand in a path", []string{"icacls", `C:\w\a&b\x`, "/reset", "/L"}, `icacls "C:\w\a&b\x" /reset /L`},
		{"a pipe in a path", []string{"icacls", `C:\w\a|b\x`, "/reset", "/L"}, `icacls "C:\w\a|b\x" /reset /L`},
		{"plain words stay bare", []string{"git", "a/b-c.txt", "k=v", "C:/a:b", "_x+"}, "git a/b-c.txt k=v C:/a:b _x+"},
		{"a word ending in a backslash that needs no quoting", []string{"dir", `C:\dir\`}, `dir C:\dir\`},
		{"a dash word with more than letters is quoted", []string{"x", "-x:y", "--flag=v", "--ok-1", "-"}, `x "-x:y" "--flag=v" --ok-1 -`},
		{"what cmd or PowerShell reads as its own", []string{"x", "a^b", "(x)", "@x", "a,b", "#x", "<a>"}, `x "a^b" "(x)" "@x" "a,b" "#x" "<a>"`},
		{"a space", []string{"x", "a b", `a\b c`}, `x "a b" "a\b c"`},
		{"a single quote", []string{"x", "it's"}, `x "it's"`},
		{"an empty argv", nil, ""},
	} {
		got, err := shell.Quote(shell.Windows, c.argv)
		if err != nil || got != c.want {
			t.Errorf("%s: Quote = %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

// Every refusal of Cmd and of PowerShell holds in Windows, and ErrWindows is the one it adds, for what cmd and PowerShell do not both read as
// itself: a $ or a backtick, a quoted word that ends in a backslash, and a first word that is not plain. A caller says the step in words.
//
// What it costs when this fails: a word one shell reads as another one is quoted anyway, and the line gives a program an argv the caller did not mean.
func TestQuoteWindowsRefusesWhatNoOneLineServes(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		argv []string
		want error
	}{
		{"a NUL", []string{"x", "a\x00b"}, shell.ErrNUL},
		{"a newline", []string{"x", "C:\\w\\a\nb"}, shell.ErrNewline},
		{"a byte that is not UTF-8", []string{"x", "a\xffb"}, shell.ErrNotUTF8},
		{"a control", []string{"x", "a\u200bb"}, shell.ErrControl},
		{"a best-fit double quote", []string{"x", "a\uff02b"}, shell.ErrBestFit},
		{"a percent sign", []string{"x", "50%"}, shell.ErrCmdChar},
		{"a bang", []string{"x", "hi!"}, shell.ErrCmdChar},
		{"a double quote", []string{"x", `a"b`}, shell.ErrPowerShellArg},
		{"a curly double quote", []string{"x", "a\u201cb"}, shell.ErrPowerShellArg},
		{"an empty word", []string{"x", ""}, shell.ErrPowerShellArg},
		{"the stop-parsing token, which holds a percent sign", []string{"x", "--%"}, shell.ErrCmdChar},
		{"a space and a trailing backslash", []string{"x", `a b\`}, shell.ErrPowerShellArg},
		{"a dollar sign", []string{"x", "$HOME"}, shell.ErrWindows},
		{"a dollar sign in a path", []string{"x", `C:\w\a$b`}, shell.ErrWindows},
		{"a backtick", []string{"x", "a`b"}, shell.ErrWindows},
		{"a quoted word that ends in a backslash", []string{"x", `C:\w\a;b\`}, shell.ErrWindows},
		{"a quoted first word", []string{`C:\w\a;b\x.exe`, "a"}, shell.ErrWindows},
		{"a first word with a space", []string{"C:/Program Files/x.exe", "a"}, shell.ErrWindows},
	} {
		line, err := shell.Quote(shell.Windows, c.argv)
		if !errors.Is(err, c.want) || line != "" {
			t.Errorf("%s: Quote = %q, %v; want %v and no line", c.name, line, err, c.want)
			continue
		}
		if !strings.Contains(err.Error(), "argument ") {
			t.Errorf("%s: the error %q does not name the argument", c.name, err)
		}
		if !errors.Is(c.want, shell.ErrWindows) && errors.Is(err, shell.ErrWindows) {
			t.Errorf("%s: the error is ErrWindows, which is for what no one line serves, not for %v", c.name, c.want)
		}
	}
}
