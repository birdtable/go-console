package ui

// The live view: a screen that draws what a command prints once, again every
// few seconds, with a few keys that run the same operation as the command
// they stand for. It is bubbletea's, and it is
// read-only unless a key says otherwise. --plain and --json print once.

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// LiveItem is one row the keys can act on: a session, by its id.
type LiveItem struct {
	ID   string
	Name string
	Link string
}

// LiveKey is one key at the bottom of the screen and what it does to the
// selected row. Act returns the line to show under the screen.
//
// A key that changes something sets Ask: the first press shows what Ask
// returns ("press s again to stop ac11") and changes nothing; the same key
// pressed again on the same row does it. Any other key takes the question back.
type LiveKey struct {
	Key  string
	Help string
	Ask  func(it LiveItem) string
	Act  func(it LiveItem) string
	// Name makes the key ask for the selected row's Name typed in full; Act runs
	// only when it was.
	Name bool
	// Exec is the program to step into for the selected row: the screen steps
	// aside, the program runs, and the screen comes back and reads again.
	Exec func(it LiveItem) *exec.Cmd
}

// Live is a screen: how to draw it, and the keys.
type Live struct {
	// Every is how long between redraws, 5 seconds when zero.
	Every time.Duration
	// It runs off the screen's own loop, so a slow read does not freeze it.
	Draw func() []LiveItem
	// Columns and Rows draw the screen as a table whose rows the keys move
	// through, in place of Draw. A failed read keeps the last rows and says
	// when they were read. The first cell of a row is its ID and its Name,
	// unless Item says otherwise.
	Columns []TableColumn
	Rows    func() ([][]string, error)
	Item    func(row []string) LiveItem
	Keys    []LiveKey
	// Frame draws what is around the screen: a heading, the row the keys act on, the note, a footer, the keys.
	// It is given the state and returns the text shown. The package draws none of it and has no word of its
	// own: with no Frame the screen is its body alone, with no keys named on it.
	Frame func(v LiveView) string
}

// LiveView is the state a Live's Frame draws from.
type LiveView struct {
	// Body is Draw's lines, or the table of Rows drawn at the screen's width.
	Body string
	// Rows is whether Body is a table of Rows, whose read time and error follow.
	Rows bool
	// ReadAt is when the rows were last read, zero before the first read; ReadErr is the last read's error.
	ReadAt  time.Time
	ReadErr error
	// Selected is the row the keys act on, nil when there is none.
	Selected *LiveItem
	// Note is what the last key said, empty when it said nothing.
	Note string
	// Typing is a key waiting for the row's name to be typed in full, nil when none is.
	Typing *LiveTyping
	// Keys are the keys the caller gave.
	Keys []LiveKey
}

// LiveTyping is a key waiting for a name: the row, the key, and what has been typed.
type LiveTyping struct {
	Item  LiveItem
	Key   LiveKey
	Typed string
}

// RunLive shows l until q, escape or ctrl-c, and returns the last screen as
// text, for the caller to leave in the scrollback. Each Draw is the session's
// Capture, so an ending the screen prints is not the run's. opts are
// bubbletea's, for a caller that runs the view on streams of its own, as a
// test with no terminal does; with none it runs on the terminal.
func (s *Session) RunLive(l Live, opts ...tea.ProgramOption) (string, error) {
	m := newLiveModel(s, l)
	end, err := tea.NewProgram(m, append(s.teaStreams(), opts...)...).Run()
	if err != nil {
		return m.body, err
	}
	if fm, ok := end.(*liveModel); ok {
		return fm.body, nil
	}
	return m.body, nil
}

type liveModel struct {
	s     *Session
	l     Live
	body  string
	items []LiveItem
	sel   int
	note  string
	// drawing is whether a Draw is under way, so a tick does not start a
	// second one behind it.
	drawing           bool
	armedKey, armedID string
	// mu keeps one Draw or Act at a time: both print through Capture.
	mu *sync.Mutex

	rows    [][]string
	readAt  time.Time
	readErr error
	width   int
	height  int
	typing  *typing
	now     func() time.Time
	run     func(c *exec.Cmd, back tea.ExecCallback) tea.Cmd
	inline  bool // the commands run where they are made, as a driven model's do
}

type typing struct {
	key LiveKey
	it  LiveItem
	buf string
}

type (
	liveTick  time.Time
	liveDrawn struct {
		body   string
		items  []LiveItem
		rows   [][]string
		err    error
		at     time.Time
		isRows bool
	}
	liveActed struct{ note string }
	liveBack  struct {
		name string
		err  error
	}
)

func newLiveModel(s *Session, l Live) *liveModel {
	m := newLiveModelAt(s, l, s.clockFunc())
	m.inline = false
	return m
}

func newLiveModelAt(s *Session, l Live, now func() time.Time) *liveModel {
	if l.Every == 0 {
		l.Every = 5 * time.Second
	}
	m := &liveModel{s: s, l: l, mu: &sync.Mutex{}, now: now, run: tea.ExecProcess, width: 80, height: 24, inline: true}
	m.apply(m.draw())
	return m
}

func (m *liveModel) draw() liveDrawn {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.l.Rows != nil {
		rows, err := m.l.Rows()
		return liveDrawn{rows: rows, err: err, at: m.now(), isRows: true}
	}
	var b strings.Builder
	var items []LiveItem
	m.s.Capture(&b, func() { items = m.l.Draw() })
	return liveDrawn{body: strings.TrimRight(b.String(), "\n"), items: items}
}

func (m *liveModel) apply(d liveDrawn) {
	was := ""
	if m.sel < len(m.items) {
		was = m.items[m.sel].ID
	}
	if d.isRows {
		m.readErr = d.err
		if d.err == nil {
			m.rows, m.readAt = d.rows, d.at
			d.items = nil
			for _, r := range d.rows {
				d.items = append(d.items, m.item(r))
			}
		} else {
			d.items = m.items
		}
		m.items, m.sel = d.items, 0
	} else {
		m.body, m.items, m.sel = d.body, d.items, 0
	}
	for i, it := range d.items {
		if it.ID == was {
			m.sel = i
		}
	}
}

func (m *liveModel) item(row []string) LiveItem {
	if m.l.Item != nil {
		return m.l.Item(row)
	}
	if len(row) == 0 {
		return LiveItem{}
	}
	return LiveItem{ID: row[0], Name: row[0]}
}

func (m *liveModel) every() tea.Cmd {
	return tea.Tick(m.l.Every, func(t time.Time) tea.Msg { return liveTick(t) })
}

func (m *liveModel) redraw() tea.Cmd {
	m.drawing = true
	return m.here(func() tea.Msg { return m.draw() })
}

// here is c, or when the model's commands run where they are made, c run now and its message
// delivered: a driver returns only after every message its step caused has reached the model.
func (m *liveModel) here(c tea.Cmd) tea.Cmd {
	if m.inline {
		_, c = m.Update(c())
	}
	return c
}

// Init starts the redraw timer.
func (m *liveModel) Init() tea.Cmd { return m.every() }

// Update moves the selection, runs a key, redraws on the timer, and leaves on q.
func (m *liveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case liveTick:
		if m.drawing {
			return m, m.every()
		}
		return m, tea.Batch(m.redraw(), m.every())
	case liveDrawn:
		m.drawing = false
		m.apply(msg)
	case liveActed:
		m.note = msg.note
		return m, m.redraw()
	case liveBack:
		m.note = fmt.Sprintf(m.s.messages().LiveBack, msg.name)
		if msg.err != nil {
			m.note += ": " + msg.err.Error()
		}
		return m, m.redraw()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		if m.typing != nil {
			return m.typed(msg)
		}
		return m.key(msg.String())
	}
	return m, nil
}

func (m *liveModel) typed(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	t := m.typing
	switch k.String() {
	case keyEscape, keyInterrupt:
		m.typing, m.note = nil, m.s.messages().LiveNothingChanged
	case "enter":
		m.typing = nil
		if t.it.Name == "" || t.buf != t.it.Name {
			m.note = m.s.messages().LiveNotTheName
			return m, nil
		}
		m.note = ""
		return m, m.act(t.key, t.it)
	case "backspace":
		if r := []rune(t.buf); len(r) > 0 {
			t.buf = string(r[:len(r)-1])
		}
	default:
		t.buf += k.Text
	}
	return m, nil
}

func (m *liveModel) act(k LiveKey, it LiveItem) tea.Cmd {
	return m.here(func() tea.Msg {
		m.mu.Lock()
		defer m.mu.Unlock()
		return liveActed{k.Act(it)}
	})
}

func (m *liveModel) key(k string) (tea.Model, tea.Cmd) {
	armedKey, armedID := m.armedKey, m.armedID
	m.armedKey, m.armedID = "", ""
	switch k {
	case "q", keyEscape, keyInterrupt:
		return m, tea.Quit
	case "up", "k":
		if m.sel > 0 {
			m.sel--
		}
		return m, nil
	case "down", "j":
		if m.sel < len(m.items)-1 {
			m.sel++
		}
		return m, nil
	}
	for _, lk := range m.l.Keys {
		if lk.Key != k {
			continue
		}
		if m.sel >= len(m.items) {
			m.note = m.s.messages().LiveNothingSelected
			return m, nil
		}
		return m.press(lk, m.items[m.sel], armedKey == k && armedID == m.items[m.sel].ID)
	}
	return m, nil
}

func (m *liveModel) press(lk LiveKey, it LiveItem, armed bool) (tea.Model, tea.Cmd) {
	if lk.Exec != nil {
		c := lk.Exec(it)
		if c == nil {
			m.note = m.s.messages().LiveNothingToStepInto
			return m, nil
		}
		m.note = ""
		return m, m.here(m.run(c, func(err error) tea.Msg { return liveBack{name: it.ID, err: err} }))
	}
	if lk.Name {
		if it.Name == "" {
			m.note = m.s.messages().LiveNoName
			return m, nil
		}
		m.typing = &typing{key: lk, it: it}
		return m, nil
	}
	if lk.Ask != nil && !armed {
		m.armedKey, m.armedID = lk.Key, it.ID
		m.note = lk.Ask(it)
		return m, nil
	}
	m.note = ""
	return m, m.act(lk, it)
}

func (m *liveModel) tableView() string {
	keep, widths, _ := fitColumns(m.l.Columns, m.rows, m.width-2, 2)
	cols := make([]table.Column, len(keep))
	for i, c := range keep {
		cols[i] = table.Column{Title: Name(m.l.Columns[c].Title), Width: widths[c]}
	}
	rows := make([]table.Row, len(m.rows))
	for r, row := range m.rows {
		for _, c := range keep {
			cell := ""
			if c < len(row) {
				cell = Name(row[c])
			}
			rows[r] = append(rows[r], cell)
		}
	}
	st := table.DefaultStyles()
	st.Header = mutedStyle(m.s.outStream()).Padding(0, 1, 0, 0)
	st.Cell = lipgloss.NewStyle().Padding(0, 1, 0, 0)
	st.Selected = lipgloss.NewStyle().Bold(true)
	wide := 0
	for _, c := range keep {
		wide += widths[c] + 2
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithStyles(st), table.WithWidth(wide),
		table.WithHeight(max(2, min(len(rows)+1, m.height-6))), table.WithFocused(true))
	t.SetCursor(m.sel)
	return t.View()
}

// Frame is the screen as drawn: what the caller's Frame makes of the state, or the body alone when it gave none.
func (m *liveModel) Frame() string {
	v := LiveView{Rows: m.l.Rows != nil, ReadAt: m.readAt, ReadErr: m.readErr, Note: m.note, Keys: m.l.Keys}
	if v.Rows {
		v.Body = m.tableView()
	} else {
		v.Body = m.body
	}
	if m.sel < len(m.items) {
		it := m.items[m.sel]
		v.Selected = &it
	}
	if t := m.typing; t != nil {
		v.Typing = &LiveTyping{Item: t.it, Key: t.key, Typed: t.buf}
	}
	if m.l.Frame == nil {
		return v.Body
	}
	return m.l.Frame(v)
}

// View is the frame, on the alternate screen.
func (m *liveModel) View() tea.View {
	v := tea.NewView(m.Frame())
	v.AltScreen = true
	return v
}
