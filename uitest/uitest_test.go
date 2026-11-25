package uitest

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// counter is a screen with two keys: j adds one, q leaves.
type counter struct {
	n    int
	w, h int
	// never is closed when the test ends: Init's command waits on it, a command that does not
	// answer before Run returns, as a tick would not.
	never chan struct{}
}

// newCounter is a counter whose waiting command ends with the test.
func newCounter(t *testing.T) *counter {
	t.Helper()
	c := &counter{never: make(chan struct{})}
	t.Cleanup(func() { close(c.never) })
	return c
}

func (c *counter) screen() Screen {
	return Screen{
		Init: func() tea.Cmd { return func() tea.Msg { <-c.never; return nil } },
		Update: func(m tea.Msg) tea.Cmd {
			switch m := m.(type) {
			case tea.WindowSizeMsg:
				c.w, c.h = m.Width, m.Height
			case tea.KeyPressMsg:
				switch m.String() {
				case "j":
					c.n++
					return func() tea.Msg { return tea.BatchMsg{func() tea.Msg { return "again" }} }
				case "q":
					return tea.Quit
				}
			case string:
				c.n += 10
			}
			return nil
		},
		View: func() string { return fmt.Sprintf("n=%d %dx%d", c.n, c.w, c.h) },
	}
}

func TestUITestDrivesKeys(t *testing.T) { //nolint:paralleltest // serial: moved as it stands, not yet made parallel
	c := newCounter(t)
	res := Run(c.screen(), 40, 10, "j", "j", "q")
	if res.Frame != "n=22 40x10" || !res.Quit {
		t.Errorf("got %+v", res)
	}
	// The negative half: with no keys nothing moved and nothing left.
	c = newCounter(t)
	if res := Run(c.screen(), 40, 10); res.Frame != "n=0 40x10" || res.Quit {
		t.Errorf("no keys moved the screen: %+v", res)
	}
}

func TestKeysAreNamedOrTyped(t *testing.T) { //nolint:paralleltest // serial: moved as it stands, not yet made parallel
	if got := Key("enter"); len(got) != 1 || got[0].(tea.KeyPressMsg).String() != "enter" {
		t.Errorf("enter was %v", got)
	}
	for _, name := range []string{"home", "end"} {
		if got := Key(name); len(got) != 1 || got[0].(tea.KeyPressMsg).String() != name {
			t.Errorf("Key(%q) = %v", name, got)
		}
	}
	if got := Key("ctrl+c"); len(got) != 1 || got[0].(tea.KeyPressMsg).String() != "ctrl+c" {
		t.Errorf("ctrl+c was %v", got)
	}
	got := Key("ab")
	if len(got) != 2 || got[1].(tea.KeyPressMsg).String() != "b" {
		t.Errorf("typed text was %v", got)
	}
}

type recorder struct {
	failed, fatal bool
	msg           string
}

func (r *recorder) Helper()                   {}
func (r *recorder) Errorf(f string, a ...any) { r.failed, r.msg = true, fmt.Sprintf(f, a...) }
func (r *recorder) Fatalf(string, ...any)     { r.fatal = true }

// A planted golden that does not match the frame fails; one that does passes.
func TestAPlantedGoldenFails(t *testing.T) { //nolint:paralleltest // serial: moved as it stands, not yet made parallel
	r := &recorder{}
	Golden(r, "planted", "a frame that is not the golden")
	if !r.failed {
		t.Error("a frame that differs from its golden passed")
	}
	r = &recorder{}
	Golden(r, "planted", "n=22 40x10")
	if r.failed || r.fatal {
		t.Error("a frame equal to its golden failed")
	}
	r = &recorder{}
	Golden(r, "missing", "x")
	if !r.fatal {
		t.Error("a frame with no golden passed")
	}
}

// A golden's name cannot step out of testdata, even to come back in: a name with .. in it fails,
// though the file it would reach is there and matches.
//
// What it costs when this fails: a test hands Golden a name built from its input, and -update
// writes, or a run reads, a file anywhere the test's account can reach.
func TestAGoldenNameCannotLeaveTestdata(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	Golden(r, "../testdata/planted", "n=22 40x10")
	if !r.fatal {
		t.Error("a name that leaves testdata and comes back was read")
	}
}

// A mismatch names the golden file and the first line that differs, with its number.
//
// What it costs when this fails: a person fixing a screen is told only that a frame differs, and
// reads two whole frames to find the line.
func TestAMismatchNamesTheGoldenAndTheFirstLineThatDiffers(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	Golden(r, "planted", "n=22 40x10\nan extra line")
	for _, want := range []string{"testdata/planted.golden", "line 2", `got  "an extra line"`, `want ""`} {
		if !strings.Contains(r.msg, want) {
			t.Errorf("the message %q does not say %q", r.msg, want)
		}
	}
}
