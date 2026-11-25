package uitest_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// A line of 100 columns written at a ConPTY of 60 is measured on the screen. Measured on
// Windows: the row stays one row of 100 cells and all 100 columns are kept, as
// on a Unix pty, because the emulator (PtyScreenCols) is wider than the pty. So an overrun can be seen on
// Windows as on Unix, and a golden of one does not differ by system.
//
// What it costs when this fails: a test that looks for an overrun at a ConPTY passes or fails for a reason
// that is not the program's, or a golden made on one system fails on the other.
func TestAConPtyLeavesALineWiderThanItsColumnsOneRow(t *testing.T) {
	t.Parallel()
	const cols, wide = 60, 100
	exe := filepath.Join(t.TempDir(), "wideline.exe")
	if out, err := exec.CommandContext(t.Context(), "go", "build", "-o", exe, "../ui/testdata/wideline").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	cmd := exec.CommandContext(t.Context(), exe)
	cmd.Env = []string{"TERM=xterm-256color"}
	p, err := uitest.StartPty(t.Context(), cmd, cols, 20, false)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if code := p.Wait(); code != 0 {
		t.Fatalf("the program exited %d, want 0", code)
	}
	screen := p.Screen()
	widest, xs := 0, strings.Count(screen, "x")
	for r := range strings.SplitSeq(screen, "\n") {
		widest = max(widest, len([]rune(r)))
	}
	if widest != wide || xs != wide {
		t.Errorf("the widest row is %d cells and %d of %d columns are kept, want one row of all %d: %q", widest, xs, wide, wide, screen)
	}
}
