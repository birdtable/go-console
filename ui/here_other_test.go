//go:build !windows

package ui

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"
)

// hereRun runs a command and answers what it printed, what it said on stderr and how it ended.
func hereRun(t *testing.T, name string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), name, args...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err = cmd.Run()
	return strings.TrimSpace(out.String()), errOut.String(), err
}

// hereRefused runs a command that ends in the heredialect program and wants an error naming the shell and no output.
func hereRefused(t *testing.T, name, shellName string, argv ...string) {
	t.Helper()
	out, errOut, err := hereRun(t, argv[0], argv[1:]...)
	if err == nil || out != "" || !strings.Contains(errOut, shellName+" has no dialect") {
		t.Errorf("%s: HereDialect printed %q, %v (stderr %q), want an error naming %s and no output", name, out, err, errOut, shellName)
	}
}

// herePOSIX runs a command that ends in the heredialect program and wants POSIX.
func herePOSIX(t *testing.T, name string, argv ...string) {
	t.Helper()
	out, errOut, err := hereRun(t, argv[0], argv[1:]...)
	if err != nil || out != strconv.Itoa(int(shell.POSIX)) {
		t.Errorf("%s: HereDialect printed %q, %v (stderr %q), want %d", name, out, err, errOut, shell.POSIX)
	}
}

// hereBuild builds a Go program of ui/testdata under the given name in a folder of its own.
func hereBuild(t *testing.T, dir, name, pkg string) string {
	t.Helper()
	exe := dir + "/" + name
	if out, err := exec.CommandContext(t.Context(), "go", "build", "-o", exe, pkg).CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v: %s", pkg, err, out)
	}
	return exe
}

// HereDialect off Windows names the shell the program was started from by the parent chain. The test builds
// ui/testdata/heredialect and ui/testdata/runarg, a program that runs its arguments as a child, and names the
// second after a shell, so the reader is pinned on a host with no such shell. Run through a program named
// fish, with a program named for no shell between, it answers an error that names fish: that is two levels of the
// real reader. Run through one named cmd behind that fish, as `go build ./cmd` names a binary, it still answers fish: cmd is no
// shell off Windows, and the real chain behind the test is not the test's to know. Under the real sh behind that fish it
// answers fish too: a sh with no job control shares the program's process group and is passed over, and what lies
// behind a passed-over sh is the test's to fix, so the row no longer stands on sh alone (TestHereDialectPassesOverItsOwnGroup has the POSIX row). /bin/csh and /bin/tcsh, which macOS ships, are run live, and a real fish
// is run when it is on PATH; where it is not, the row logs and a live run is owed.
//
// What it costs when this fails: a program in fish, csh or tcsh quotes the line it prints for POSIX, and a
// backslash or a ! in it reaches the person's shell as code; or a program a Go tool started quotes for cmd,
// and a ; or a $ in it reaches the person's shell as code.
func TestHereDialectReadsTheParentChainOffWindows(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	probe := hereBuild(t, dir, "heredialect", "./testdata/heredialect")
	fish := hereBuild(t, dir+"/fish", "fish", "./testdata/runarg")
	unnamed := hereBuild(t, dir+"/unnamed", "wrapper", "./testdata/runarg")
	cmd := hereBuild(t, dir+"/cmd", "cmd", "./testdata/runarg")
	refused := func(name, shellName string, argv ...string) { t.Helper(); hereRefused(t, name, shellName, argv...) }
	refused("fish, then the real sh, which shares the program's group", "fish", fish, "sh", "-c", `"$1"; s=$?; exit $s`, "sh", probe)
	refused("fish, then a program with no shell's name", "fish", fish, unnamed, probe)
	refused("fish directly", "fish", fish, probe)
	refused("fish, then a program named cmd", "fish", fish, cmd, probe)
	for _, sh := range []string{"csh", "tcsh"} {
		if _, err := exec.LookPath("/bin/" + sh); err != nil {
			t.Logf("/bin/%s is not there, so its row did not run (macOS ships it), and a live run is owed: %v", sh, err)
			continue
		}
		refused("under /bin/"+sh, sh, "/bin/"+sh, "-c", probe)
	}
	if path, err := exec.LookPath("fish"); err != nil {
		t.Logf("fish is not on PATH, so the row 'fish' did not run, and a live fish run is owed: %v", err)
	} else {
		refused("under the real fish", "fish", path, "-c", probe+"; exit $status")
	}
}

// Off Windows a chain with no known shell, or none at all, is POSIX and nil, and cmd is passed over.
//
// What it costs when this fails: a program on Linux or macOS in a shell the kit does not name quotes its line
// for the Windows default, PowerShell.
func TestDialectOfGroupsIsPOSIXByDefault(t *testing.T) {
	t.Parallel()
	for _, chain := range [][]string{nil, {"tmux", "sshd"}, {"cmd"}} {
		if got, err := dialectOfGroups(chain, nil); got != shell.POSIX || err != nil {
			t.Errorf("%v: %d, %v, want POSIX, nil", chain, got, err)
		}
	}
}

// HereDialect on Linux and macOS passes over a POSIX shell in the program's own process group, read live. The test
// builds ui/testdata/runarg as csh and as sh, copies and not links, since macOS names a process for a link's
// target; with -g a helper starts its child in a new group (Setpgid). csh starts sh in a new group and sh starts
// the program in its own, as a wrapper script under a person's csh does: the answer wraps ErrNoDialect and names
// csh. sh starts the program in a new group, as a person's sh with job control does: POSIX, and the walk ends there,
// so the real chain behind the test does not matter in either row. No real csh or fish is needed.
//
// What it costs when this fails: the group is read from the wrong field or not at all, and a wrapper script's sh
// in front of csh or fish answers POSIX, so a backslash or a ! in the pasted line reaches the person's shell as
// code; or a person's own sh is passed over and the line is quoted for whatever lies behind it.
func TestHereDialectPassesOverItsOwnGroup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	probe := hereBuild(t, dir, "heredialect", "./testdata/heredialect")
	csh := hereBuild(t, dir+"/csh", "csh", "./testdata/runarg")
	sh := hereBuild(t, dir+"/sh", "sh", "./testdata/runarg")
	hereRefused(t, "csh, then sh in a new group, then the program in sh's group", "csh", csh, "-g", sh, probe)
	herePOSIX(t, "sh, then the program in a new group", sh, "-g", probe)
	herePOSIX(t, "csh, then sh in csh's group, then the program in a new group", csh, sh, "-g", probe)
}
