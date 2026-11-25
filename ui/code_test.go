package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/birdtable/go-console/uitest"
)

const sshConfig = "Host box\n  HostName 192.0.2.7\n  User you\n  IdentityFile ~/.ssh/example"

// At a terminal that renders, Code sets the text on the code ground, every line
// as wide as the widest, and the text under the paint is the text given. The
// golden is that text at the two widths: a block is not wrapped, so it is the
// same at both.
func TestCodeIsSetOnTheCodeGround(t *testing.T) {
	t.Parallel()
	for _, w := range []int{80, 120} {
		got := New(screenOpt(true, w)).Code(sshConfig)
		if !strings.Contains(got, "\x1b[") {
			t.Fatalf("width %d: Code painted nothing: %q", w, got)
		}
		if !strings.Contains(got, "48;") && !strings.Contains(got, "\x1b[4") {
			t.Errorf("width %d: Code set no background: %q", w, got)
		}
		rows := strings.Split(ansi.Strip(got), "\n")
		if len(rows) != 4 {
			t.Fatalf("width %d: %d rows", w, len(rows))
		}
		for _, r := range rows {
			if ansi.StringWidth(r) != ansi.StringWidth(rows[3]) {
				t.Errorf("width %d: rows are not as wide as the widest: %q", w, rows)
			}
		}
		shown := make([]string, len(rows))
		for i, r := range rows {
			shown[i] = strings.TrimRight(r, " ")
		}
		uitest.Golden(t, fmt.Sprintf("code-%d", w), strings.Join(shown, "\n"))
		for i, l := range strings.Split(sshConfig, "\n") {
			if strings.TrimSpace(rows[i]) != strings.TrimSpace(l) {
				t.Errorf("row %d is %q, want %q", i, rows[i], l)
			}
		}
	}
}

// Off a terminal, under NO_COLOR, TERM=dumb and --plain, Code is the text as it
// is, so a copy holds nothing else.
func TestCodeIsThePlainTextWhereNothingRenders(t *testing.T) {
	t.Parallel()
	if got := New().Code(sshConfig); got != sshConfig {
		t.Errorf("off a terminal: %q", got)
	}
	if got := detected("CLICOLOR_FORCE=1", "NO_COLOR=1").Code(sshConfig); got != sshConfig {
		t.Errorf("under NO_COLOR: %q", got)
	}
	if got := detected("CLICOLOR_FORCE=1", "TERM=dumb").Code(sshConfig); got != sshConfig {
		t.Errorf("with TERM=dumb: %q", got)
	}
	if got := New(screenOpt(true, 0), Plain()).Code(sshConfig); got != sshConfig {
		t.Errorf("with --plain: %q", got)
	}
	// The negative half: with none of those it is painted.
	if got := detected("CLICOLOR_FORCE=1", "TERM=xterm-256color").Code(sshConfig); got == sshConfig {
		t.Error("Code did not paint at a terminal that renders")
	}
}
