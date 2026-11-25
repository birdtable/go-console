package shell_test

import (
	"bytes"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"
)

// argvProgram writes each argument it is given as hex, one a line, so what reaches it is read back exactly and the
// shell's own output encoding cannot change it.
const argvProgram = `package main

import (
	"fmt"
	"os"
)

func main() {
	for _, a := range os.Args[1:] {
		fmt.Printf("%x\n", a)
	}
}
`

// psEnv is the environment a PowerShell is started in: the tests' own and the names Windows PowerShell 5.1 reads at
// start (without SystemDrive it makes a folder named %SystemDrive% where it runs; without PATHEXT it runs a
// program not at all and says nothing).
func psEnv(t *testing.T) []string {
	t.Helper()
	root := os.Getenv("SYSTEMROOT")
	drive := filepath.VolumeName(root)
	home := t.TempDir()
	roaming, local := filepath.Join(home, "AppData", "Roaming"), filepath.Join(home, "AppData", "Local")
	for _, d := range []string{roaming, local} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return append(os.Environ(),
		"SystemDrive="+drive, "windir="+root, "USERPROFILE="+home, "APPDATA="+roaming, "LOCALAPPDATA="+local,
		"PATHEXT=.COM;.EXE;.BAT;.CMD;.PS1", "ProgramData="+drive+`\ProgramData`, "ALLUSERSPROFILE="+drive+`\ProgramData`, "ProgramFiles="+drive+`\Program Files`)
}

// The line Quote writes for PowerShell, run by Windows PowerShell 5.1 (and by pwsh 7 where it is on PATH), gives a
// native program the argv that was quoted, for the hard arguments the dialect does not refuse, the program first.
// powershell.exe not on PATH fails the test; pwsh not on PATH logs its row by name and passes it, and a live pwsh 7 run stays owed.
//
// What it costs when this fails: a program on Windows is given another argv than the line a person pasted says.
func TestQuotePowerShellGivesAProgramTheArgv(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "argv.go")
	if err := os.WriteFile(src, []byte(argvProgram), 0o600); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "argv.exe")
	if out, err := exec.CommandContext(t.Context(), "go", "build", "-o", exe, src).CombinedOutput(); err != nil {
		t.Fatalf("go build argv: %v: %s", err, out)
	}
	env := psEnv(t)
	hard := []string{`C:\dir\`, `a\b`, `a b`, `it's`, "\u2018x\u2019", "\u201ax\u201b", "$x", "`", ";", "&", "|", `a\\`, `a\\b c`, "a,b", "@x", "50%", "é", "-", "--flag=value", "(a)", "<a>", "~", "*", "-a.b", "--x=a.b", "-a:b", "-x:", "-a:-b:c"}
	for _, c := range []struct {
		name string
		argv []string
	}{
		{"powershell", []string{"powershell", "-NoProfile", "-NonInteractive", "-Command"}},
		{"pwsh", []string{"pwsh", "-NoProfile", "-NonInteractive", "-Command"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if _, err := exec.LookPath(c.argv[0]); err != nil {
				if c.name == "pwsh" {
					t.Log("pwsh is not on PATH: the pwsh row did not run, and a live pwsh 7 run is owed")
					return
				}
				t.Fatalf("%s is not on PATH, and this test needs it: %v", c.name, err)
			}
			// One program run for each argument, so a word that goes wrong cannot move the others.
			for _, a := range hard {
				want := []string{a, "z"}
				line, err := shell.Quote(shell.PowerShell, append([]string{exe}, want...))
				if err != nil {
					t.Errorf("Quote(%q): %v", want, err)
					continue
				}
				cmd := exec.CommandContext(t.Context(), c.argv[0], append(c.argv[1:], line)...)
				cmd.Dir, cmd.Env = dir, env
				var out, errOut bytes.Buffer
				cmd.Stdout, cmd.Stderr = &out, &errOut
				if err := cmd.Run(); err != nil {
					t.Errorf("%s %q: %v: %s", c.name, line, err, errOut.String())
					continue
				}
				if strings.TrimSpace(out.String()) == "" {
					t.Errorf("under %s the program did not run for %q: it printed nothing (stderr %q)", c.name, line, errOut.String())
					continue
				}
				var got []string
				for l := range strings.SplitSeq(strings.TrimSpace(strings.ReplaceAll(out.String(), "\r", "")), "\n") {
					b, err := hex.DecodeString(l)
					if err != nil {
						t.Errorf("under %s %q: output line %q is not hex", c.name, line, l)
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
