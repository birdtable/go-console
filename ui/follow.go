package ui

// A stream that is followed, as `tail -f` does: the caller appends lines, a
// terminal shows them in a viewport that stays at the bottom while it is there,
// and anywhere else each line is printed as it comes. The viewport scrolls with
// the keys Page uses, and scrolling up stops the following until `end`.

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// followKeep is how many lines a viewport keeps: the last of a stream that
// does not end. Off a terminal nothing is kept, each line being printed.
const followKeep = 5000

// ErrInterrupted is what Run returns when ctrl-c ended it: control is the
// caller's, which exits 130 as a shell does for an interrupt.
var ErrInterrupted = errors.New("interrupted")

// A Follow is a stream of lines on show. Line, Stop and Run may be called from
// different goroutines.
type Follow struct {
	s       *Session
	mu      sync.Mutex
	lines   []string
	stopped bool
	prog    *tea.Program
	done    chan struct{}
	// printErr is the first error printing a line met, which Run returns: off a terminal Line
	// prints each line itself, with no error of its own to give.
	printErr error
	// opts are the options a package test gives the program Run starts, in
	// place of the terminal: its input, its output and its size.
	opts []tea.ProgramOption
}

// NewFollow is a Follow of the session's with nothing in it yet.
func (s *Session) NewFollow() *Follow { return &Follow{s: s, done: make(chan struct{})} }

func (f *Follow) follows() bool {
	st := f.s.outStream()
	return st.paints() && st.interactive && !f.s.plain.Load() && !f.s.json.Load()
}

type (
	followMore struct{}
	followStop struct{}
)

// Line adds one line. What a guest or a server wrote is not trusted with the
// terminal: escape sequences and every control but tab are dropped first, a
// carriage return included. Off a terminal, or plain, the line is printed at
// once, through the session, and kept nowhere; at one it appears in the
// viewport, the last followKeep lines being kept. Under `--json` it is printed
// nowhere, so a guest's line that is a JSON document never goes ahead of the
// run's Result.
func (f *Follow) Line(s string) {
	s = safeLine(s)
	f.mu.Lock()
	if f.stopped {
		f.mu.Unlock()
		return
	}
	if !f.follows() {
		// Under the lock, so lines print in the order they came and Stop waits
		// for one already taken.
		if _, err := fmt.Fprintln(f.s.stdout(), s); err != nil && f.printErr == nil {
			f.printErr = err
		}
		f.mu.Unlock()
		return
	}
	f.keep(s)
	prog := f.prog
	f.mu.Unlock()
	if prog != nil {
		prog.Send(followMore{})
	}
}

// The caller holds the lock.
func (f *Follow) keep(s string) {
	f.lines = append(f.lines, s)
	if over := len(f.lines) - followKeep; over > 0 {
		f.lines = append([]string(nil), f.lines[over:]...)
	}
}

// safeLine is s without escape sequences and without the characters Name quotes, tab
// aside: controls, format characters such as a bidi override, the line and paragraph
// separators, the characters a display may ignore and the variation selectors, an emoji's
// joiner and selector kept.
func safeLine(s string) string {
	s = ansi.Strip(s)
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == '\t' || !hidden(s, i, r, n) {
			b.WriteRune(r)
		}
		i += n
	}
	return b.String()
}

// Stop says the stream has ended: Run returns nil. A line after it is dropped.
func (f *Follow) Stop() {
	f.mu.Lock()
	if f.stopped {
		f.mu.Unlock()
		return
	}
	f.stopped = true
	prog := f.prog
	close(f.done)
	f.mu.Unlock()
	if prog != nil {
		prog.Send(followStop{})
	}
}

// Run shows the stream until Stop (nil) or ctrl-c (ErrInterrupted). At a
// terminal what was shown is left in the scrollback afterwards, as it is for a
// pipe; off one Run only waits for Stop, the lines having been printed already.
// A line that could not be printed is in the error Run returns.
func (f *Follow) Run() error {
	if !f.follows() {
		<-f.done
		f.mu.Lock()
		err := f.printErr
		f.mu.Unlock()
		if err != nil {
			return fmt.Errorf("a line could not be printed: %w", err)
		}
		return nil
	}
	m := newFollowModel(f)
	f.mu.Lock()
	p := tea.NewProgram(m, append(f.s.teaStreams(), f.opts...)...)
	f.prog = p
	stopped := f.stopped
	f.mu.Unlock()
	if stopped {
		m.stopping = true
	}
	_, err := p.Run()
	f.mu.Lock()
	text := strings.Join(f.lines, "\n")
	f.prog = nil
	f.mu.Unlock()
	var printed error
	if text != "" {
		if _, perr := fmt.Fprintln(f.s.stdout(), text); perr != nil {
			printed = fmt.Errorf("what was shown could not be left in the scrollback: %w", perr)
		}
	}
	if m.interrupted || errors.Is(err, tea.ErrInterrupted) {
		err = ErrInterrupted
	}
	if printed == nil {
		return err
	}
	return errors.Join(err, printed)
}

type followModel struct {
	f           *Follow
	vp          viewport.Model
	interrupted bool
	stopping    bool
	ended       bool
}

func newFollowModel(f *Follow) *followModel {
	w, h := f.s.term.Width, f.s.term.Height
	if w == 0 || h == 0 {
		w, h = 80, 24
	}
	vp := viewport.New(viewport.WithWidth(w), viewport.WithHeight(max(h-1, 1)))
	vp.SoftWrap = true
	m := &followModel{f: f, vp: vp}
	m.refill()
	return m
}

func (m *followModel) refill() {
	m.f.mu.Lock()
	text := strings.Join(m.f.lines, "\n")
	m.f.mu.Unlock()
	atEnd := m.vp.AtBottom()
	m.vp.SetContent(text)
	if atEnd {
		m.vp.GotoBottom()
	}
}

func (m *followModel) Init() tea.Cmd {
	if m.stopping {
		return tea.Quit
	}
	return nil
}

func (m *followModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case followMore:
		m.refill()
		return m, nil
	case followStop:
		m.refill()
		m.ended = true
		return m, tea.Quit
	case tea.WindowSizeMsg:
		// Whether it was following is read before the new size rewraps the
		// lines, or a narrower screen reads as scrolled back.
		atEnd := m.vp.AtBottom()
		m.vp.SetWidth(msg.Width)
		m.vp.SetHeight(max(msg.Height-1, 1))
		m.refill()
		if atEnd {
			m.vp.GotoBottom()
		}
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case keyInterrupt:
			m.interrupted = true
			return m, tea.Quit
		case "end", "G":
			m.vp.GotoBottom()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m *followModel) Frame() string {
	msgs := m.f.s.messages()
	state := msgs.FollowFollowing
	if !m.vp.AtBottom() {
		state = msgs.FollowPaused
	}
	return m.vp.View() + "\n" + m.f.s.Note(state+msgs.FollowKeys)
}

func (m *followModel) View() tea.View {
	v := tea.NewView(m.Frame())
	v.AltScreen = true
	return v
}
