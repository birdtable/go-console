// Package uitest drives a screen with keys at a fixed size and compares what
// it drew with a golden file, so a key that does the wrong thing fails a
// test instead of passing because the frame it would have drawn still looks
// right. Run has no terminal: an instant command (a read) is run and fed back, and a wait (a tick) is dropped after settle of wall time, the one wall-clock wait here.
// StartPty runs a program at a pty on Linux, macOS and Windows and reads its screen.
//
// It is built on bubbletea and huh (the models it drives) and on xpty and vt (the pty and the screen), and it
// adds the screen and wait helpers those leave out. The test configures it: the size
// (cols and rows), the keys it sends, and mute.
package uitest

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/birdtable/go-console/uitest/golden"
)

// Screen is what a test drives: a bubbletea model seen through three
// functions, so a huh form (whose Update returns its own model type) and a
// live view are driven the same way.
type Screen struct {
	Init   func() tea.Cmd
	Update func(tea.Msg) tea.Cmd
	View   func() string
}

// Model is a bubbletea model whose Update returns only a command, as a driver
// of ui's own (a live view, a follow) has it. It is declared here so uitest does
// not import ui.
type Model interface {
	Init() tea.Cmd
	Update(tea.Msg) tea.Cmd
	View() string
}

// ModelScreen is m as a Screen.
func ModelScreen(m Model) Screen {
	return Screen{Init: m.Init, Update: m.Update, View: m.View}
}

// FormScreen is a huh form a program built, for Ask, as a Screen.
func FormScreen(f *huh.Form) Screen {
	return Screen{
		Init:   f.Init,
		Update: func(msg tea.Msg) tea.Cmd { _, c := f.Update(msg); return c },
		View:   f.View,
	}
}

// LiveScreen is a live view a program built with ui.NewLiveDriver as a Screen.
func LiveScreen(d Model) Screen { return ModelScreen(d) }

// Result is what a run ended with.
type Result struct {
	// Frame is the last view, with its styling and the spaces ending its lines
	// stripped.
	Frame string
	Quit  bool
}

// settle is how long a command gets to answer before it is taken for a timer.
const settle = 250 * time.Millisecond

// Run gives s a size, then each of keys in turn (see Key), running the
// commands that answer at once, and returns the last frame.
func Run(s Screen, width, height int, keys ...string) Result {
	var res Result
	pending := []tea.Cmd{}
	if s.Init != nil {
		pending = append(pending, s.Init())
	}
	pending = append(pending, s.Update(tea.WindowSizeMsg{Width: width, Height: height}))
	drain(s, pending, &res)
	pending = nil
	for _, k := range keys {
		for _, msg := range Key(k) {
			pending = append(pending, s.Update(msg))
		}
		drain(s, pending, &res)
		pending = nil
	}
	res.Frame = flat(s.View())
	return res
}

// Feed sends one message to s and runs what answers at once. For a message a
// key cannot stand for, such as the end of a program the screen handed over to.
func Feed(s Screen, msg tea.Msg) Result {
	var res Result
	drain(s, []tea.Cmd{s.Update(msg)}, &res)
	res.Frame = flat(s.View())
	return res
}

func flat(view string) string {
	lines := strings.Split(ansi.Strip(view), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

func drain(s Screen, cmds []tea.Cmd, res *Result) {
	for len(cmds) > 0 {
		c := cmds[0]
		cmds = cmds[1:]
		if c == nil {
			continue
		}
		msg := answer(c)
		switch m := msg.(type) {
		case nil:
		case tea.QuitMsg:
			res.Quit = true
		case tea.BatchMsg:
			cmds = append(cmds, m...)
		default:
			cmds = append(cmds, s.Update(m))
		}
	}
}

func answer(c tea.Cmd) tea.Msg {
	got, up := make(chan tea.Msg, 1), make(chan struct{})
	go func() { close(up); got <- c() }()
	<-up // the settle is counted from when the command runs, not from when a busy machine got to it
	select {
	case m := <-got:
		return m
	case <-time.After(settle):
		return nil
	}
}

func named(k string) (tea.KeyPressMsg, bool) {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}, true
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}, true
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}, true
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, true
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}, true
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}, true
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}, true
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}, true
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, true
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}, true
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}, true
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}, true
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}, true
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}, true
	}
	return tea.KeyPressMsg{}, false
}

// Key is the messages for one step: a name ("enter", "down", "ctrl+c"), or
// any other text, typed one character at a time.
func Key(k string) []tea.Msg {
	if m, ok := named(k); ok {
		return []tea.Msg{m}
	}
	if rest, ok := strings.CutPrefix(k, "ctrl+"); ok && len([]rune(rest)) == 1 {
		return []tea.Msg{tea.KeyPressMsg{Code: []rune(rest)[0], Mod: tea.ModCtrl}}
	}
	var out []tea.Msg
	for _, r := range k {
		out = append(out, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return out
}

// T is the part of *testing.T that Golden uses, so a test can hand it a
// recorder and see that a planted golden fails.
type T interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// Golden compares got, its trailing spaces and newlines trimmed, with
// testdata/<name>.golden, and with -update writes the file instead: golden.File
// with the trim a screen frame wants. A mismatch names the file and the first
// line that differs.
func Golden(t T, name, got string) {
	t.Helper()
	golden.File(t, name+".golden", []byte(strings.TrimRight(got, " \n")+"\n"))
}
