package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"
)

// The choice of dialect from the ancestors' image names, nearest first, on every system.
//
// What it costs when this fails: on Windows a program quotes its pasted line for the wrong shell, and a value
// with a backtick or a $ reaches the person's shell as code; under pwsh 7 or bash, which no host of ours has run
// live, nothing else would catch it.
func TestDialectOfChain(t *testing.T) {
	t.Parallel()
	nine := []string{"a.exe", "b.exe", "c.exe", "d.exe", "e.exe", "f.exe", "g.exe", "h.exe", "cmd.exe"}
	rows := []struct {
		name  string
		chain []string
		want  shell.Dialect
	}{
		{"powershell.exe", []string{"powershell.exe"}, shell.PowerShell},
		{"pwsh.exe", []string{"pwsh.exe"}, shell.PowerShell},
		{"cmd.exe", []string{"cmd.exe"}, shell.Cmd},
		{"the nearest wins: cmd under pwsh", []string{"cmd.exe", "pwsh.exe"}, shell.Cmd},
		{"the nearest wins: pwsh under cmd", []string{"pwsh.exe", "cmd.exe"}, shell.PowerShell},
		{"case does not matter", []string{"PWSH.EXE"}, shell.PowerShell},
		{"pwsh-preview", []string{"pwsh-preview"}, shell.PowerShell},
		{"pwsh-lts", []string{"pwsh-lts"}, shell.PowerShell},
		{"PWSH-PREVIEW.EXE", []string{"PWSH-PREVIEW.EXE"}, shell.PowerShell},
		{"pwsh-preview in front of bash", []string{"pwsh-preview", "bash"}, shell.PowerShell},
		{"pwsh-previewx is no whole name and is passed over", []string{"pwsh-previewx", "cmd.exe"}, shell.Cmd},
		{"pwsh- is no whole name and is passed over", []string{"pwsh-", "cmd.exe"}, shell.Cmd},
		{"case does not matter for cmd", []string{"CMD.exe"}, shell.Cmd},
		{"others are passed over", []string{"explorer.exe", "cmd.exe"}, shell.Cmd},
		{"none within 8", nine, shell.PowerShell},
		{"the eighth counts", nine[1:], shell.Cmd},
		{"no match", []string{"explorer.exe"}, shell.PowerShell},
		{"empty chain", nil, shell.PowerShell},
		{"ash.exe", []string{"ash.exe"}, shell.POSIX},
		{"ksh93.exe", []string{"ksh93.exe"}, shell.POSIX},
		{"ash in front of cmd", []string{"ash.exe", "cmd.exe"}, shell.POSIX},
		{"ksh93 in front of cmd", []string{"ksh93.exe", "cmd.exe"}, shell.POSIX},
		{"a versioned busybox", []string{"busybox-w64-FRP-6354-g5f09fb446.exe"}, shell.POSIX},
		{"a versioned busybox in front of cmd", []string{"BusyBox64.exe", "cmd.exe"}, shell.POSIX},
		{"a name with no .exe is the same name: busybox", []string{"busybox", "cmd.exe"}, shell.POSIX},
		{"[zsh login]", []string{"zsh", "login"}, shell.POSIX},
		{"[bash tmux fish]", []string{"bash", "tmux", "fish"}, shell.POSIX},
		{"[tmux sshd] names none and is the default", []string{"tmux", "sshd"}, shell.PowerShell},
		{"[Zsh] in any case", []string{"Zsh"}, shell.POSIX},
		{"an unnamed shell in front of cmd", []string{"rc.exe", "cmd.exe"}, shell.Cmd},
		{"git, make: the nearest sh wins", []string{"sh.exe", "make.exe", "powershell.exe"}, shell.POSIX},
		{"bash under cmd", []string{"bash.exe", "cmd.exe"}, shell.POSIX},
		{"sh under powershell", []string{"sh.exe", "powershell.exe"}, shell.POSIX},
		{"conhost, bash, cmd", []string{"conhost.exe", "bash.exe", "cmd.exe"}, shell.POSIX},
		{"BASH under cmd", []string{"BASH.EXE", "cmd.exe"}, shell.POSIX},
		{"mintty, bash", []string{"mintty.exe", "bash.exe"}, shell.POSIX},
		{"cmd under bash", []string{"cmd.exe", "bash.exe"}, shell.Cmd},
	}
	for _, n := range []string{"bash", "sh", "zsh", "dash", "ash", "ksh", "ksh93", "mksh", "busybox"} {
		rows = append(rows, struct {
			name  string
			chain []string
			want  shell.Dialect
		}{n + ".exe", []string{n + ".exe"}, shell.POSIX})
	}
	for _, c := range rows {
		if got, err := dialectOfChain(c.chain); got != c.want || err != nil {
			t.Errorf("%s: %d, %v, want %d, nil", c.name, got, err, c.want)
		}
	}
}

// A shell the kit knows and shell.Quote writes no dialect for is an error that names it, and the walk ends there:
// a cmd behind it is not reached.
//
// What it costs when this fails: a program in fish or tcsh gets a POSIX-quoted line in which a backslash or a !
// is read as code, or a cmd's quoting behind it.
func TestDialectOfChainNoDialect(t *testing.T) {
	t.Parallel()
	for _, c := range [][]string{{"fish"}, {"login", "fish"}, {"fish", "tmux", "zsh"}, {"FISH", "bash"}} {
		got, err := dialectOfChain(c)
		if got != 0 || !errors.Is(err, ErrNoDialect) || !strings.Contains(strings.ToLower(err.Error()), "fish") {
			t.Errorf("%v: %d, %v, want 0 and an error wrapping ErrNoDialect", c, got, err)
		}
	}
	for _, image := range []string{"fish.exe", "csh.exe", "tcsh.exe", "xonsh.exe", "nu.exe", "elvish.exe", "wsl.exe", "wslhost.exe", "FISH.EXE"} {
		for _, chain := range [][]string{{image}, {image, "cmd.exe"}, {"conhost.exe", image, "bash.exe"}} {
			got, err := dialectOfChain(chain)
			if got != 0 || !errors.Is(err, ErrNoDialect) || !strings.Contains(err.Error(), image) {
				t.Errorf("%v: %d, %v, want 0 and an error wrapping ErrNoDialect that names %s", chain, got, err, image)
			}
		}
	}
	if got, err := dialectOfChain([]string{"cmd.exe", "fish.exe"}); got != shell.Cmd || err != nil {
		t.Errorf("cmd in front of fish: %d, %v, want Cmd, nil", got, err)
	}
}

// With no shell in the chain the default is the caller's: POSIX off Windows, PowerShell on it.
//
// What it costs when this fails: a program in a shell the kit does not name gets the Windows default on Linux.
func TestDialectOfChainOrDefault(t *testing.T) {
	t.Parallel()
	for _, chain := range [][]string{nil, {"tmux", "sshd"}} {
		if got, err := dialectOfChainOr(chain, shell.POSIX, false); got != shell.POSIX || err != nil {
			t.Errorf("%v: %d, %v, want POSIX, nil", chain, got, err)
		}
	}
}

// Off Windows cmd, powershell, wsl and wslhost are not shells and are passed over, and pwsh, pwsh-preview and
// pwsh-lts stay. A name that only begins with pwsh- is no shell: the table holds whole names.
//
// What it costs when this fails: a Go tool built as cmd, which `go build ./cmd` names a binary, makes a program
// quote for the Windows shell on Linux or macOS, and a ; or a $ in the line reaches the person's shell as code.
func TestDialectOfChainOffWindows(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		chain []string
		want  shell.Dialect
		err   bool
	}{
		{"[cmd zsh]", []string{"cmd", "zsh"}, shell.POSIX, false},
		{"[cmd fish]", []string{"cmd", "fish"}, 0, true},
		{"[powershell bash]", []string{"powershell", "bash"}, shell.POSIX, false},
		{"[pwsh]", []string{"pwsh"}, shell.PowerShell, false},
		{"[pwsh-preview]", []string{"pwsh-preview"}, shell.PowerShell, false},
		{"[pwsh-lts]", []string{"pwsh-lts"}, shell.PowerShell, false},
		{"[PWSH-PREVIEW.EXE]", []string{"PWSH-PREVIEW.EXE"}, shell.PowerShell, false},
		{"[pwsh-preview bash]", []string{"pwsh-preview", "bash"}, shell.PowerShell, false},
		{"[pwsh-previewx bash]", []string{"pwsh-previewx", "bash"}, shell.POSIX, false},
		{"[pwsh- bash]", []string{"pwsh-", "bash"}, shell.POSIX, false},
		{"[cmd.exe]", []string{"cmd.exe"}, shell.POSIX, false},
		{"[wsl fish]", []string{"wsl", "fish"}, 0, true},
		{"[wslhost zsh]", []string{"wslhost", "zsh"}, shell.POSIX, false},
	} {
		got, err := dialectOfChainOr(c.chain, shell.POSIX, false)
		if got != c.want || (err != nil) != c.err || (c.err && !errors.Is(err, ErrNoDialect)) {
			t.Errorf("%s: %d, %v, want %d, error %v", c.name, got, err, c.want, c.err)
		}
	}
}
