package shell_test

import (
	"bytes"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/birdtable/go-console/shell"
)

// The line Quote writes for Windows, run by cmd.exe, by Windows PowerShell 5.1 and by pwsh 7 in Legacy, in Standard and
// in Windows $PSNativeCommandArgumentPassing mode, gives a native program the argv that was quoted: the words of the issue's paths and every
// character one of the shells reads as its own. The program is named bare, as the first word of the line must be, and is found on PATH.
// powershell.exe or cmd.exe not on PATH fails the test; pwsh not on PATH logs its rows by name and passes them, and a live pwsh 7 run stays owed.
//
// What it costs when this fails: a fix line pasted into cmd or PowerShell runs a program with another argv than Quote promised, or a second command.
func TestQuoteWindowsLineIsReadBackByCmdAndPowerShell(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "argv.go")
	if err := os.WriteFile(src, []byte(argvProgram), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.CommandContext(t.Context(), "go", "build", "-o", filepath.Join(dir, "argv.exe"), src).CombinedOutput(); err != nil {
		t.Fatalf("go build argv: %v: %s", err, out)
	}
	env := append(psEnv(t), "PATH="+dir+";"+os.Getenv("PATH"))
	words := []string{`C:\w\a;b\x`, `C:\w\a&b\x`, `C:\w\a|b\x`, "a;b", "a^b", "(x)", "@x", "a,b", "#x", "-x:y", "a b", `C:\dir\`, `a\b c`, "<a>", "a=b"}
	psRun := func(exe, prefix string) func(string) *exec.Cmd {
		return func(line string) *exec.Cmd {
			return exec.CommandContext(t.Context(), exe, "-NoProfile", "-NonInteractive", "-Command", prefix+line)
		}
	}
	for _, c := range []struct {
		name, exe string
		run       func(line string) *exec.Cmd
	}{
		{"cmd", "cmd.exe", func(line string) *exec.Cmd {
			c := exec.CommandContext(t.Context(), "cmd.exe")
			// /d no AutoRun, /s strip the one outer pair added here and run the rest verbatim, /c run and leave.
			c.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /d /s /c "` + line + `"`}
			return c
		}},
		{"powershell", "powershell", psRun("powershell", "")},
		{"pwsh Legacy", "pwsh", psRun("pwsh", "$PSNativeCommandArgumentPassing='Legacy'; ")},
		{"pwsh Standard", "pwsh", psRun("pwsh", "$PSNativeCommandArgumentPassing='Standard'; ")},
		{"pwsh Windows", "pwsh", psRun("pwsh", "$PSNativeCommandArgumentPassing='Windows'; ")},
	} {
		t.Run(strings.ReplaceAll(c.name, " ", "_"), func(t *testing.T) {
			t.Parallel()
			if _, err := exec.LookPath(c.exe); err != nil {
				if c.exe == "pwsh" {
					t.Logf("pwsh is not on PATH: the %s row did not run, and a live pwsh 7 run is owed", c.name)
					return
				}
				t.Fatalf("%s is not on PATH, and this test needs it: %v", c.exe, err)
			}
			// One program run for each word, so a word that goes wrong cannot move the others.
			for _, a := range words {
				want := []string{a, "z"}
				line, err := shell.Quote(shell.Windows, append([]string{"argv"}, want...))
				if err != nil {
					t.Errorf("Quote(%q): %v", want, err)
					continue
				}
				cmd := c.run(line)
				cmd.Dir, cmd.Env = dir, env
				var out, errOut bytes.Buffer
				cmd.Stdout, cmd.Stderr = &out, &errOut
				if err := cmd.Run(); err != nil {
					t.Errorf("under %s %q: %v: %s", c.name, line, err, errOut.String())
					continue
				}
				if strings.TrimSpace(out.String()) == "" {
					t.Errorf("under %s the program did not run for %q: it printed nothing (stderr %q)", c.name, line, errOut.String())
					continue
				}
				var got []string
				for l := range strings.SplitSeq(strings.TrimSuffix(strings.ReplaceAll(out.String(), "\r", ""), "\n"), "\n") {
					b, err := hex.DecodeString(l)
					if err != nil {
						t.Errorf("under %s %q: output line %q is not hex", c.name, line, l)
						continue
					}
					got = append(got, string(b))
				}
				if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
					t.Errorf("under %s the line %q gave the program %q, want %q", c.name, line, got, want)
				}
			}
		})
	}
}
