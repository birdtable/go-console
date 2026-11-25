package shell_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/birdtable/go-console/shell"
)

func TestDialectNamesRoundTrip(t *testing.T) {
	t.Parallel()
	want := map[shell.Dialect]string{shell.POSIX: "posix", shell.PowerShell: "powershell", shell.Cmd: "cmd", shell.Windows: "windows"}
	ds := shell.Dialects()
	if len(ds) != len(want) {
		t.Fatalf("Dialects() = %v, want %d members", ds, len(want))
	}
	for _, d := range ds {
		if d.String() != want[d] {
			t.Errorf("%d.String() = %q, want %q", int(d), d.String(), want[d])
		}
		got, err := shell.ParseDialect(d.String())
		if err != nil || got != d {
			t.Errorf("ParseDialect(%q) = %v, %v, want %v", d.String(), got, err, d)
		}
	}
}

func TestDialectsOrderAndFreshSlice(t *testing.T) {
	t.Parallel()
	ds := shell.Dialects()
	if len(ds) != 4 || ds[0] != shell.POSIX || ds[1] != shell.PowerShell || ds[2] != shell.Cmd || ds[3] != shell.Windows {
		t.Fatalf("Dialects() = %v, want POSIX, PowerShell, Cmd, Windows", ds)
	}
	ds[0] = shell.Cmd
	if shell.Dialects()[0] != shell.POSIX {
		t.Error("a change to the returned slice reached the next call")
	}
}

func TestParseDialectIsCaseInsensitive(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]shell.Dialect{"PowerShell": shell.PowerShell, "POSIX": shell.POSIX, "Cmd": shell.Cmd, "pOsIx": shell.POSIX, "WINDOWS": shell.Windows} {
		if got, err := shell.ParseDialect(name); err != nil || got != want {
			t.Errorf("ParseDialect(%q) = %v, %v, want %v", name, got, err, want)
		}
	}
}

func TestParseDialectRefuses(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", "auto", "bash", " posix", "posix ", "cmd\n", "pwsh"} {
		got, err := shell.ParseDialect(name)
		if got != 0 || !errors.Is(err, shell.ErrDialect) {
			t.Errorf("ParseDialect(%q) = %v, %v, want 0 and ErrDialect", name, got, err)
			continue
		}
		if !strings.Contains(err.Error(), fmt.Sprintf("%q", name)) || !strings.Contains(err.Error(), "want posix, powershell, cmd or windows") {
			t.Errorf("ParseDialect(%q) error %q does not quote the name and list the four", name, err)
		}
	}
}

func TestDialectStringOutOfRange(t *testing.T) {
	t.Parallel()
	for d, want := range map[shell.Dialect]string{0: "Dialect(0)", shell.Windows + 1: "Dialect(5)", -1: "Dialect(-1)", 99: "Dialect(99)"} {
		if got := d.String(); got != want {
			t.Errorf("Dialect(%d).String() = %q, want %q", int(d), got, want)
		}
	}
}
