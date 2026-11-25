//go:build linux

package ui

import (
	"os"
	"strconv"
	"testing"

	"github.com/birdtable/go-console/shell"
)

// On Linux a process started through a link is named for the link, so a PowerShell preview or LTS, which its
// package starts through /usr/bin/pwsh-preview or /usr/bin/pwsh-lts, shows as that name and not as pwsh. The test
// builds ui/testdata/runarg under a name that is no shell, links pwsh-preview and pwsh-lts to it, and runs
// ui/testdata/heredialect through each link: the real reader sees the link's name and answers PowerShell, with
// no PowerShell installed. macOS names the process for the link's target, so the row is Linux's.
//
// What it costs when this fails: a person in a PowerShell preview on Linux is handed a POSIX-quoted line, in which
// a ' or a $ is read by the wrong rules.
func TestHereDialectNamesAProcessForItsLinkOnLinux(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	probe := hereBuild(t, dir, "heredialect", "./testdata/heredialect")
	helper := hereBuild(t, dir+"/helper", "wrapper", "./testdata/runarg")
	for _, name := range []string{"pwsh-preview", "pwsh-lts"} {
		link := dir + "/" + name
		if err := os.Symlink(helper, link); err != nil {
			t.Fatalf("link %s: %v", name, err)
		}
		out, errOut, err := hereRun(t, link, probe)
		if err != nil || out != strconv.Itoa(int(shell.PowerShell)) {
			t.Errorf("through a link named %s: HereDialect printed %q, %v (stderr %q), want %d", name, out, err, errOut, shell.PowerShell)
		}
	}
}
