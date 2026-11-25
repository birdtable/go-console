package ui

// A form's screen is drawn once it is settled and its last view stays. bubbletea v2.1
// draws huh's empty view of a finished form on exit, racing the program's end, and draws the form before
// the terminal's size arrives and after: under load the menu was erased on one run and kept on the next,
// or a row short. bubbletea v2.0.2 drew the same screen every time, so this does.
// huh's view hook runs before the content is set, so the form is run by a model of ours.

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// runKept runs f as huh's Run does, with its last screen left in place. A plain form is huh's own.
func (s *Session) runKept(f *huh.Form) error {
	if s.Accessible() {
		return f.Run()
	}
	// tea.Quit renders once more on a cancel, tea.Interrupt does not.
	f.SubmitCmd, f.CancelCmd = tea.Quit, tea.Quit
	out := s.out
	opts := []tea.ProgramOption{s.teaEnv(), tea.WithOutput(s.errW())}
	if out != nil {
		opts[1] = tea.WithOutput(out)
	}
	if s.in != nil {
		opts = append(opts, tea.WithInput(s.in))
	}
	_, err := tea.NewProgram(&keptModel{form: f}, opts...).Run()
	if errors.Is(err, tea.ErrInterrupted) || (err == nil && f.State == huh.StateAborted) {
		return huh.ErrUserAborted
	}
	return err
}

// keptModel is the program's model: a pointer, so what it remembers outlives each Update.
type keptModel struct {
	form  *huh.Form
	sized bool
	last  string
}

func (k *keptModel) Init() tea.Cmd { return k.form.Init() }

func (k *keptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		k.sized = true
	}
	_, cmd := k.form.Update(msg)
	return k, cmd
}

func (k *keptModel) View() tea.View {
	if s := k.form.View(); k.sized && s != "" {
		k.last = s + "\n" // the renderer erases the screen's last row on exit: an empty one is the closing word's
	}
	v := tea.NewView(k.last)
	v.ReportFocus = true
	return v
}
