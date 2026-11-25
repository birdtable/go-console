package ui

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/birdtable/go-console/shell"
)

// shellEnv is the environment a shell is started in: the tests' own, which the test main stripped to a few
// names, and the names Windows PowerShell 5.1 reads at start, taken from SYSTEMROOT and the tests' HOME, and PATHEXT.
// Without the first PowerShell reads a literal %SystemDrive% and makes a folder of that name where it runs;
// without PATHEXT it silently does not run '& <path>.exe' at all (exit 0, no output), where cmd does not care.
func shellEnv(t *testing.T) []string {
	t.Helper()
	root := os.Getenv("SYSTEMROOT")
	drive := filepath.VolumeName(root)
	home := os.Getenv("HOME")
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

// HereDialect on Windows names the shell the program was started from: cmd under cmd, PowerShell under Windows
// PowerShell 5.1. The test builds ui/testdata/heredialect and runs it under each, in a folder of its own and in
// an environment a shell can start in. A shell that is not on PATH fails the test, not skips it, except bash (Git for Windows), whose row logs that it did not run when bash is absent or is the WSL launcher in System32, and runs the bash path that LookPath found. pwsh 7 is not
// run live (no host has it): TestDialectOfChain covers its name.
//
// What it costs when this fails: a program on Windows quotes the line it prints for the wrong shell, and the
// person who pastes it gets a line that means something else or none.
func TestHereDialectNamesTheShellOnWindows(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exe := filepath.Join(dir, "heredialect.exe")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", exe, "./testdata/heredialect")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build heredialect: %v: %s", err, out)
	}
	env := shellEnv(t)
	bash, bashErr := exec.LookPath("bash")
	if bashErr == nil && strings.EqualFold(filepath.Dir(bash), filepath.Join(os.Getenv("SystemRoot"), "System32")) {
		bashErr = fmt.Errorf("%s is the WSL launcher, which cannot open a C:/ path", bash)
	}
	for _, c := range []struct {
		name     string
		argv     []string
		want     shell.Dialect
		optional bool   // not usable: logged by name, and a live run is owed
		cmdLine  string // the whole command line, when Go's quoting would not survive cmd /c
	}{
		{"cmd", []string{"cmd", "/c", exe}, shell.Cmd, false, ""},
		{"powershell", []string{"powershell", "-NoProfile", "-NonInteractive", "-Command", "& '" + exe + "'"}, shell.PowerShell, false, ""},
		{"cmd, then bash", []string{"cmd"}, shell.POSIX, true,
			// cmd /c strips the outer pair of quotes, so a bash under "Program Files" needs a second pair.
			`cmd /c ""` + filepath.ToSlash(bash) + `" -c '` + filepath.ToSlash(exe) + `'"`},
	} {
		need, err := c.argv[0], error(nil)
		if c.optional {
			need, err = "bash", bashErr
		} else {
			_, err = exec.LookPath(need)
		}
		if err != nil && c.optional {
			t.Logf("%s is not usable, so the row '%s' did not run, and a live run is owed: %v", need, c.name, err)
			continue
		} else if err != nil {
			t.Errorf("%s is not on PATH, and this test needs it: %v", c.name, err)
			continue
		}
		cmd := exec.CommandContext(t.Context(), c.argv[0], c.argv[1:]...)
		cmd.Dir, cmd.Env = dir, env
		if c.cmdLine != "" {
			cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: c.cmdLine}
		}
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		if err := cmd.Run(); err != nil {
			t.Errorf("%s: %v: %s", c.name, err, errOut.String())
			continue
		}
		if strings.TrimSpace(out.String()) == "" {
			t.Errorf("under %s the program did not run: it printed nothing (stderr %q)", c.name, errOut.String())
			continue
		}
		got, err := strconv.Atoi(strings.TrimSpace(out.String()))
		if err != nil || shell.Dialect(got) != c.want {
			t.Errorf("under %s HereDialect is %q, want %d; stderr: %q", c.name, out.String(), c.want, errOut.String())
		}
	}
}

// Windows wires the judge that makes a parent younger than its child, or one the snapshot names and OpenProcess
// refuses, a chain that cannot be trusted: ErrNoDialect unless a known shell was found nearer.
//
// What it costs when this fails: Windows takes another system's judge, and PowerShell's single quotes are printed for a
// line pasted into cmd.exe, which does not read them, so a word runs as a command.
func TestDialectOfParents(t *testing.T) {
	t.Parallel()
	exe := func(names ...string) map[int]proc {
		m := map[int]proc{}
		for i, n := range names {
			p := proc{name: n, start: int64(100 - i)}
			if i+1 < len(names) {
				p.ppid = i + 2
			}
			m[i+1] = p
		}
		return m
	}
	younger := map[int]proc{1: {name: "program.exe", ppid: 2, start: 1}, 2: {name: "cmd.exe", ppid: 3, start: 5}, 3: {name: "explorer.exe", start: 4}}
	olderCmd := exe("program.exe", "cmd.exe", "explorer.exe")
	youngerNode := exe("program.exe", "node.exe", "x.exe")
	youngerNode[3] = proc{name: "x.exe", start: 900}
	youngerAfterCmd := exe("program.exe", "cmd.exe", "x.exe")
	youngerAfterCmd[3] = proc{name: "x.exe", start: 900}
	unopenable := exe("program.exe", "node.exe", "other.exe")
	unopenable[3] = proc{name: "other.exe", unread: true}
	unopenableAfterCmd := exe("program.exe", "cmd.exe", "other.exe")
	unopenableAfterCmd[3] = proc{name: "other.exe", unread: true}
	for _, c := range []struct {
		name  string
		procs map[int]proc
		want  shell.Dialect
		err   bool
	}{
		{"a younger parent, no known shell nearer", younger, 0, true},
		{"a younger parent beyond a node, no known shell", youngerNode, 0, true},
		{"a younger parent with cmd.exe nearer", youngerAfterCmd, shell.Cmd, false},
		{"an unopenable parent, no known shell nearer", unopenable, 0, true},
		{"an unopenable parent with cmd.exe nearer", unopenableAfterCmd, shell.Cmd, false},
		{"every parent older", olderCmd, shell.Cmd, false},
	} {
		d, err := dialectOfParents(1, fake(c.procs))
		if d != c.want || (err != nil) != c.err || (c.err && !errors.Is(err, ErrNoDialect)) {
			t.Errorf("%s: %v, %v; want %v, error %v", c.name, d, err, c.want, c.err)
		}
	}
}

// Pid 0, which every snapshot names and OpenProcess refuses, reads as named and unread.
//
// What it costs when this fails: a process the walk cannot check reads as checked, and the chain past it is trusted.
func TestSnapshotProcUnread(t *testing.T) {
	t.Parallel()
	p, ok := snapshotProc(map[int]int{0: 0}, map[int]string{0: "[System Process]"}, 0)
	if !ok || !p.unread || p.name != "[System Process]" {
		t.Errorf("pid 0: %+v, %v; want named and unread", p, ok)
	}
	if _, ok := snapshotProc(map[int]int{}, map[int]string{}, 0); ok {
		t.Error("a pid the snapshot does not name read as ok")
	}
}
