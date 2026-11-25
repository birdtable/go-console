//go:build windows

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

// The line Quote writes for Cmd, run by cmd.exe itself, gives a native program the argv that was quoted, for the hard
// arguments the dialect does not refuse — the same arguments TestQuoteCmdReadBackByArgvRule reads back with a Go model
// of cmd's rule, now read back by the real command processor. cmd.exe is started with /s so it strips only the one
// outer pair of quotes this test adds and runs the carefully carated line beneath it verbatim.
//
// What it costs when this fails: a line a person pastes into cmd.exe runs a program with another argv than Quote
// promised, and because Cmd is a dialect of a security function, a word that reaches the program wrong is a way to
// smuggle a flag or a second command past the caller.
func TestQuoteCmdIsReadBackByCmdExe(t *testing.T) {
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
	args := cmdSafe()
	if len(args) == 0 {
		t.Fatal("cmdSafe() gave no arguments, so nothing was read back by cmd.exe")
	}
	// One program run for each argument, so a word that goes wrong cannot move the others.
	for _, a := range args {
		want := []string{a, "z"}
		line, err := shell.Quote(shell.Cmd, append([]string{exe}, want...))
		if err != nil {
			t.Errorf("Quote(%q): %v", want, err)
			continue
		}
		cmd := exec.CommandContext(t.Context(), "cmd.exe")
		// /d no AutoRun, /s strip the one outer pair below and run the rest verbatim, /c run and leave.
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /d /s /c "` + line + `"`}
		cmd.Dir = dir
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		if err := cmd.Run(); err != nil {
			t.Errorf("cmd %q: %v: %s", line, err, errOut.String())
			continue
		}
		if strings.TrimSpace(out.String()) == "" {
			t.Errorf("under cmd the program did not run for %q: it printed nothing (stderr %q)", line, errOut.String())
			continue
		}
		// argv.exe prints one hex line per argument, so the lines, with the final newline dropped, are the argv —
		// an empty line is the empty argument, which is why the trailing newline is trimmed and the rest is not.
		var got []string
		for l := range strings.SplitSeq(strings.TrimSuffix(strings.ReplaceAll(out.String(), "\r", ""), "\n"), "\n") {
			b, err := hex.DecodeString(l)
			if err != nil {
				t.Errorf("under cmd %q: output line %q is not hex", line, l)
				continue
			}
			got = append(got, string(b))
		}
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Errorf("under cmd the line %q gave the program %q, want %q", line, got, want)
		}
	}
}
