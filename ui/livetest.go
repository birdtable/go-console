package ui

import (
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
)

// LiveDriver is a Live with no terminal behind it, for a test: the same model
// RunLive runs, with a clock and a stand-in for handing the terminal to a
// program.
type LiveDriver struct{ m *liveModel }

// NewLiveDriver is l, ready to drive, drawn in the session's Capture as
// RunLive draws it. now is the clock a read is dated with.
// stepInto stands for the program an Exec key hands the terminal to: it gets
// the command (which is never started) and returns what that program would
// have ended with. Nil means every program ends cleanly.
func (s *Session) NewLiveDriver(l Live, now func() time.Time, stepInto func(*exec.Cmd) error) *LiveDriver {
	m := newLiveModelAt(s, l, now)
	m.run = func(c *exec.Cmd, back tea.ExecCallback) tea.Cmd {
		return func() tea.Msg {
			var err error
			if stepInto != nil {
				err = stepInto(c)
			}
			return back(err)
		}
	}
	return &LiveDriver{m: m}
}

// Init is the redraw timer's first command.
func (d *LiveDriver) Init() tea.Cmd { return d.m.Init() }

// Update feeds one message and returns the command it asks for.
func (d *LiveDriver) Update(msg tea.Msg) tea.Cmd {
	_, c := d.m.Update(msg)
	return c
}

// View is the frame as drawn.
func (d *LiveDriver) View() string { return d.m.Frame() }

// Tick is the message the redraw timer sends at t, for uitest.Feed.
func (d *LiveDriver) Tick(t time.Time) tea.Msg { return liveTick(t) }
