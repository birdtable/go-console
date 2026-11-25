package ui

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birdtable/go-console/uitest"
	"github.com/dustin/go-humanize"
)

// screenOf is a live model seen the way uitest drives it.
func screenOf(m *liveModel) uitest.Screen {
	return uitest.Screen{
		Init:   m.Init,
		Update: func(msg tea.Msg) tea.Cmd { _, c := m.Update(msg); return c },
		View:   m.Frame,
	}
}

func liveCols() []TableColumn {
	return []TableColumn{{Title: "id", Priority: 9}, {Title: "state", Priority: 5}}
}

type clock struct{ t time.Time }

// noon is a clock that stands still, so a frame with a time in it is the same twice.
func noon() time.Time { return time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC) }

func (c *clock) now() time.Time { return c.t }

func TestLiveSaysWhenRead(t *testing.T) {
	t.Parallel()
	c := &clock{time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)}
	var fail error
	m := newLiveModelAt(New(WithMessages(houseMessages())), Live{
		Frame: HouseFrame, Columns: liveCols(),
		Rows: func() ([][]string, error) {
			if fail != nil {
				return nil, fail
			}
			return [][]string{{"ac11", "running"}, {"b202", "stopped"}}, nil
		},
	}, c.now)
	s := screenOf(m)
	res := uitest.Run(s, 60, 20)
	uitest.Golden(t, "live-read", res.Frame)
	if !strings.Contains(res.Frame, "read at 12:00:00") || strings.Contains(res.Frame, "could not read") {
		t.Errorf("a good read:\n%s", res.Frame)
	}
	// A failed read keeps the rows and says when they were read, not now.
	fail, c.t = errors.New("box did not answer"), c.t.Add(time.Minute)
	res = uitest.Feed(s, liveTick(c.t))
	uitest.Golden(t, "live-read-failed", res.Frame)
	for _, want := range []string{"ac11", "b202", "box did not answer", "read at 12:00:00"} {
		if !strings.Contains(res.Frame, want) {
			t.Errorf("%q is missing after a failed read:\n%s", want, res.Frame)
		}
	}
	if strings.Contains(res.Frame, "12:01:00") {
		t.Errorf("a failed read was dated as read:\n%s", res.Frame)
	}
}

func TestLiveKeyAsksForTheName(t *testing.T) {
	t.Parallel()
	var acted []string
	m := newLiveModelAt(New(WithMessages(houseMessages())), Live{
		Frame: HouseFrame, Columns: liveCols(),
		Rows: func() ([][]string, error) { return [][]string{{"ac11", "running"}, {"b202", "stopped"}}, nil },
		Keys: []LiveKey{{Key: "x", Help: "remove", Name: true, Act: func(it LiveItem) string {
			acted = append(acted, it.ID)
			return "removed " + it.ID
		}}},
	}, noon)
	s := screenOf(m)
	res := uitest.Run(s, 60, 20, "down", "x")
	uitest.Golden(t, "live-asks-name", res.Frame)
	if !strings.Contains(res.Frame, "type b202 to remove it") || len(acted) != 0 {
		t.Errorf("the first press acted or did not ask:\n%s", res.Frame)
	}
	// The wrong name changes nothing.
	res = uitest.Run(s, 60, 20, "ac11", "enter")
	if len(acted) != 0 || !strings.Contains(res.Frame, "nothing was changed") {
		t.Errorf("a wrong name acted (%v):\n%s", acted, res.Frame)
	}
	// The row's own name does it, once.
	res = uitest.Run(s, 60, 20, "x", "b202", "enter")
	if len(acted) != 1 || acted[0] != "b202" || !strings.Contains(res.Frame, "removed b202") {
		t.Errorf("the right name did not act (%v):\n%s", acted, res.Frame)
	}
	// Escape takes the question back.
	uitest.Run(s, 60, 20, "x", "b20", "esc", "enter")
	if len(acted) != 1 {
		t.Errorf("escape did not take the question back: %v", acted)
	}
}

func TestLiveSuspendsAndReturns(t *testing.T) {
	t.Parallel()
	reads := 0
	m := newLiveModelAt(New(WithMessages(houseMessages())), Live{
		Frame: HouseFrame, Columns: liveCols(),
		Rows: func() ([][]string, error) { reads++; return [][]string{{"ac11", "running"}}, nil },
		Keys: []LiveKey{
			{Key: "e", Help: "enter", Exec: func(it LiveItem) *exec.Cmd { return exec.CommandContext(t.Context(), "app", "shell", it.ID) }},
			{Key: "n", Help: "none", Exec: func(LiveItem) *exec.Cmd { return nil }},
		},
	}, noon)
	var ran *exec.Cmd
	m.run = func(c *exec.Cmd, back tea.ExecCallback) tea.Cmd {
		ran = c
		return func() tea.Msg { return back(nil) }
	}
	s := screenOf(m)
	before := reads
	res := uitest.Run(s, 60, 20, "e")
	if ran == nil || strings.Join(ran.Args, " ") != "app shell ac11" {
		t.Fatalf("stepped into %v", ran)
	}
	uitest.Golden(t, "live-back", res.Frame)
	if !strings.Contains(res.Frame, "back from ac11") || reads <= before {
		t.Errorf("did not come back and read again (reads %d, before %d):\n%s", reads, before, res.Frame)
	}
	// A key with nothing to step into says so and hands nothing over.
	ran = nil
	res = uitest.Run(s, 60, 20, "n")
	if ran != nil || !strings.Contains(res.Frame, "nothing to step into") {
		t.Errorf("an empty command was run (%v):\n%s", ran, res.Frame)
	}
}

func TestLiveWithNoRowsActsOnNothing(t *testing.T) {
	t.Parallel()
	m := newLiveModelAt(New(WithMessages(houseMessages())), Live{
		Frame: HouseFrame, Columns: liveCols(),
		Rows: func() ([][]string, error) { return nil, nil },
		Keys: []LiveKey{{Key: "x", Help: "remove", Name: true, Act: func(LiveItem) string { t.Error("acted on nothing"); return "" }}},
	}, noon)
	res := uitest.Run(screenOf(m), 60, 20, "x", "enter")
	if !strings.Contains(res.Frame, "nothing is selected") {
		t.Errorf("no rows, no word:\n%s", res.Frame)
	}
}

func TestLiveKeysMoveAndLeave(t *testing.T) {
	t.Parallel()
	m := newLiveModelAt(New(WithMessages(houseMessages())), Live{
		Frame: HouseFrame, Columns: liveCols(),
		Rows: func() ([][]string, error) { return [][]string{{"ac11", "a"}, {"b202", "b"}, {"c303", "c"}}, nil },
	}, noon)
	res := uitest.Run(screenOf(m), 60, 20, "down", "j", "down", "up", "q")
	if !res.Quit || !strings.Contains(res.Frame, "selected b202") {
		t.Errorf("quit %v:\n%s", res.Quit, res.Frame)
	}
}

// The pager, driven: it scrolls with the keys and leaves on q. Pages is only
// true at a real terminal, so this is the positive run the plain tests cannot
// make.
func TestThePagerScrollsAndLeaves(t *testing.T) {
	t.Parallel()
	var text []string
	for i := 1; i <= 40; i++ {
		text = append(text, "line "+humanize.Comma(int64(i)))
	}
	m := &pagerModel{s: New(WithMessages(houseMessages()))}
	m.vp.SetContent(strings.Join(text, "\n"))
	s := uitest.Screen{
		Init:   m.Init,
		Update: func(msg tea.Msg) tea.Cmd { _, c := m.Update(msg); return c },
		View:   func() string { return m.View().Content },
	}
	top := uitest.Run(s, 30, 10)
	if !strings.Contains(top.Frame, "line 1\n") || strings.Contains(top.Frame, "line 20") {
		t.Fatalf("the top of the page:\n%s", top.Frame)
	}
	uitest.Golden(t, "pager-top", top.Frame)
	down := uitest.Run(s, 30, 10, "pgdown")
	if strings.Contains(down.Frame, "line 1\n") || !strings.Contains(down.Frame, "line 10") {
		t.Errorf("page down did not scroll:\n%s", down.Frame)
	}
	if down.Quit {
		t.Error("scrolling left the pager")
	}
	if !uitest.Run(s, 30, 10, "q").Quit {
		t.Error("q did not leave the pager")
	}
}

// When the first read fails nothing has been read, and the screen says that
// rather than dating the failure as a read.
func TestLiveSaysNothingWasReadWhenTheFirstReadFails(t *testing.T) {
	t.Parallel()
	m := newLiveModelAt(New(WithMessages(houseMessages())), Live{
		Frame: HouseFrame, Columns: liveCols(),
		Rows: func() ([][]string, error) { return nil, errors.New("box did not answer") },
	}, noon)
	res := uitest.Run(screenOf(m), 60, 20)
	uitest.Golden(t, "live-first-read-failed", res.Frame)
	if !strings.Contains(res.Frame, "nothing has been read yet") || strings.Contains(res.Frame, "12:00:00") {
		t.Errorf("the first read failed:\n%s", res.Frame)
	}
}

// A destroying key on a row with no name has nothing to type, so a bare enter
// must not do it.
func TestLiveKeyRefusesARowWithNoName(t *testing.T) {
	t.Parallel()
	m := newLiveModelAt(New(WithMessages(houseMessages())), Live{
		Frame: HouseFrame, Columns: liveCols(),
		Rows: func() ([][]string, error) { return [][]string{{"ac11", "running"}}, nil },
		Item: func(row []string) LiveItem { return LiveItem{ID: row[0]} },
		Keys: []LiveKey{{Key: "x", Help: "remove", Name: true, Act: func(LiveItem) string {
			t.Error("acted on a row with no name")
			return ""
		}}},
	}, noon)
	res := uitest.Run(screenOf(m), 60, 20, "x", "enter")
	if !strings.Contains(res.Frame, "no name to type") {
		t.Errorf("a row with no name:\n%s", res.Frame)
	}
}

// RunLive shows the screen until q and returns the last one, for the
// scrollback; what Draw printed is caught, and an ending Draw printed is not
// the session's.
//
// What it costs when this fails: a live view leaves nothing behind when it
// closes, prints its frame into the run's own output, or a run log reads the
// word its screen showed as the run's ending.
func TestRunLiveShowsTheScreenUntilQAndKeepsNoEndingItDrew(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it reads the process's stdout.
	var frames bytes.Buffer
	opts := []tea.ProgramOption{
		tea.WithInput(strings.NewReader("q")), tea.WithOutput(&frames),
		tea.WithoutSignals(), tea.WithWindowSize(80, 24),
	}
	s := New(WithMessages(houseMessages()))
	out := capture(t, &os.Stdout, func() {
		body, err := s.RunLive(Live{Frame: HouseFrame, Draw: func() []LiveItem {
			s.Log("two sessions run")
			s.End(done())
			return nil
		}}, opts...)
		if err != nil || !strings.Contains(body, "two sessions run") {
			t.Errorf("RunLive returned %q and %v, want the last screen", body, err)
		}
	})
	if s.Ended() != nil || out != "" || frames.Len() == 0 {
		t.Errorf("the session kept %v, the run printed %q, and the view drew %d bytes", s.Ended(), out, frames.Len())
	}
}

// A step whose read, key or program takes longer than uitest's settle still reaches the frame before
// Feed or Run returns: the driven model runs its commands where it makes them, so no message is left
// to arrive after the frame is read. The read here is held back by a timer, as a loaded machine
// holds back a goroutine, so the test is red without load.
//
// What it costs when this fails: a push tier goes red at random under load, on a frame that still
// shows the read before the one a tick asked for.
func TestAFailedReadSlowerThanTheSettleIsInTheFrameWhenFeedReturns(t *testing.T) {
	t.Parallel()
	gate := make(chan struct{})
	time.AfterFunc(400*time.Millisecond, func() { close(gate) })
	var reads atomic.Int32
	m := newLiveModelAt(New(WithMessages(houseMessages())), Live{
		Frame: HouseFrame, Columns: liveCols(),
		Rows: func() ([][]string, error) {
			if reads.Add(1) == 1 {
				return [][]string{{"ac11", "running"}}, nil
			}
			<-gate
			return nil, errors.New("box did not answer")
		},
	}, noon)
	res := uitest.Feed(screenOf(m), liveTick(noon()))
	if !strings.Contains(res.Frame, "box did not answer") {
		t.Errorf("the failed read is not in the frame Feed returned:\n%s", res.Frame)
	}
}
