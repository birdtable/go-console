package ui

// The pager: text longer than the screen, at a terminal, in bubbles'
// viewport, scrolled with the arrows, page keys and the mouse wheel, and
// left with q. Off a terminal, or with
// --plain, the text is printed as it is.

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

const (
	keyInterrupt = "ctrl+c"
	keyEscape    = "esc"
)

// Pages is whether text would be shown in the pager: at a terminal that
// renders, not --plain, and longer than the screen.
func (s *Session) Pages(text string) bool {
	return pages(s.outStream(), text, s.term.Height, s.plain.Load())
}

func pages(st stream, text string, h int, plain bool) bool {
	return st.paints() && st.interactive && !plain && h > 0 && strings.Count(text, "\n") >= h
}

// Page shows text in the pager until q, escape or ctrl-c, on the session's
// streams. A Terminal of unknown size is an error, before anything is drawn.
func (s *Session) Page(text string) error {
	w, h := s.term.Width, s.term.Height
	if w == 0 || h == 0 {
		return errors.New("the pager needs the terminal's size, which its Terminal does not say")
	}
	m := &pagerModel{s: s, vp: viewport.New(viewport.WithWidth(w), viewport.WithHeight(max(h-1, 1))), text: text}
	m.vp.SetContent(text)
	_, err := tea.NewProgram(m, s.teaStreams()...).Run()
	return err
}

type pagerModel struct {
	s    *Session
	vp   viewport.Model
	text string
}

// Init has nothing to start.
func (m *pagerModel) Init() tea.Cmd { return nil }

// Update fits the view to the screen, leaves on q, and scrolls on the rest.
func (m *pagerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.vp.SetWidth(msg.Width)
		m.vp.SetHeight(max(msg.Height-1, 1))
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", keyEscape, keyInterrupt:
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// View is the text and, under it, how far down it is and the key to leave.
func (m *pagerModel) View() tea.View {
	v := tea.NewView(m.vp.View() + "\n" + m.s.Note(fmt.Sprintf(m.s.messages().PageKeys, m.vp.ScrollPercent()*100)))
	v.AltScreen = true
	return v
}

// teaStreams are the options that run a bubbletea program on the session's
// streams where they were given (In, Out), bubbletea's own, the process's,
// where not, and always under the Terminal's environment, never the process's.
func (s *Session) teaStreams() []tea.ProgramOption {
	opts := []tea.ProgramOption{s.teaEnv()}
	if s.in != nil {
		opts = append(opts, tea.WithInput(s.in))
	}
	if s.out != nil {
		opts = append(opts, tea.WithOutput(s.out))
	}
	return opts
}

// teaEnv runs a bubbletea program under the Terminal's environment. Never nil,
// since bubbletea reads the process's for a nil one.
func (s *Session) teaEnv() tea.ProgramOption {
	return tea.WithEnvironment(append([]string{}, s.term.Env...))
}
