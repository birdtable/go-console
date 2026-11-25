//go:build !windows

package ui_test

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"
)

// A POSIX line from shell.Quote is run by a real sh, which reads back the argv it was made from. The test is
// here, where a test may start a program; the shell package itself starts none. On Windows the native sessions
// paste each line into sh, PowerShell 5.1 and 7 and cmd.
//
// What it costs when this fails: a line a person pastes into sh runs a different command than the one quoted.
func TestShellQuotePOSIXIsReadBackBySh(t *testing.T) {
	t.Parallel()
	args := []string{"", "plain", "a b", "two  spaces", `it's`, `'`, `''`, `"`, `a"b`, `\`, `a\`, `a\"b`, "$HOME", "`x`", "a;b", "a&b", "a|b", "<a>", "(a)", "~", "*", "--flag=value", "a=b", "é", "‘x’", "@x", "a,b", "--%", "-", "!x", "%s"}
	line, err := shell.Quote(shell.POSIX, append([]string{"printf", `%s\0`}, args...))
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.CommandContext(t.Context(), "sh", "-c", line).Output()
	if err != nil {
		t.Fatalf("%q: %v", line, err)
	}
	if got := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00"); !slices.Equal(got, args) {
		t.Errorf("sh read back %q from %q, want %q", got, line, args)
	}
	first, err := shell.Quote(shell.POSIX, []string{"a=b", "c=d"})
	if err != nil || first != "'a=b' c=d" {
		t.Errorf("a first word with = is %q, %v: sh would read it as an assignment", first, err)
	}
}
