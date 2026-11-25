package ui

import tea "charm.land/bubbletea/v2"

// A FollowDriver is a Follow's viewport a program's test can draw, with
// uitest, at a width of its choosing. It is the same model Run shows, so a
// golden written with it is the viewport a person sees.
type FollowDriver struct {
	f *Follow
	m *followModel
}

// NewFollowDriver is f's viewport, ready to drive. Lines go in through the
// driver's Line, not f's: f.Line prints where there is no terminal, which a
// test is. Size it with uitest.Run, which sends the window size.
func NewFollowDriver(f *Follow) *FollowDriver {
	return &FollowDriver{f: f, m: newFollowModel(f)}
}

// Init is the model's first command.
func (d *FollowDriver) Init() tea.Cmd { return d.m.Init() }

// Update feeds one message and returns the command it asks for.
func (d *FollowDriver) Update(msg tea.Msg) tea.Cmd {
	_, c := d.m.Update(msg)
	return c
}

// View is the frame as drawn.
func (d *FollowDriver) View() string { return d.m.Frame() }

// Line adds a line the way a viewport takes one (escapes and controls
// dropped, the last 5000 kept) and returns the message that shows it, for
// uitest.Feed. After Stop it adds nothing.
func (d *FollowDriver) Line(s string) tea.Msg {
	s = safeLine(s)
	d.f.mu.Lock()
	if !d.f.stopped {
		d.f.keep(s)
	}
	d.f.mu.Unlock()
	return followMore{}
}

// Stop ends the stream and returns the message that leaves the viewport, for
// uitest.Feed: the screen asks to quit.
func (d *FollowDriver) Stop() tea.Msg {
	d.f.Stop()
	return followStop{}
}
