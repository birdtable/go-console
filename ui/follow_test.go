package ui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birdtable/go-console/uitest"
)

// followScreen is a Follow's viewport seen the way uitest drives it.
func followScreen(f *Follow) (*followModel, uitest.Screen) {
	m := newFollowModel(f)
	return m, uitest.Screen{
		Init:   m.Init,
		Update: func(msg tea.Msg) tea.Cmd { _, c := m.Update(msg); return c },
		View:   m.Frame,
	}
}

func feed(f *Follow, from, to int) {
	for i := from; i <= to; i++ {
		f.Line(fmt.Sprintf("line %02d of the stream", i))
	}
}

// Lines past the screen height scroll the first off and keep the last in view,
// at the two widths, with a golden of each.
//
// What it costs when this fails: `console -f` shows the start of a log and not
// what is happening now.
func TestAFollowKeepsTheLastLinesInView(t *testing.T) {
	t.Parallel()
	for _, w := range []int{80, 120} {
		f := New(WithMessages(houseMessages()), richTerminal()).NewFollow()
		_, s := followScreen(f)
		uitest.Run(s, w, 6)
		feed(f, 1, 20)
		res := uitest.Feed(s, followMore{})
		uitest.Golden(t, fmt.Sprintf("follow-%d", w), res.Frame)
		if !strings.Contains(res.Frame, "line 20") || strings.Contains(res.Frame, "line 01 ") ||
			!strings.Contains(res.Frame, "following") {
			t.Errorf("width %d, 20 lines on 6 rows:\n%s", w, res.Frame)
		}
	}
}

// Scrolling up stops the following: a line that comes does not move the view,
// and the footer says so; `end` follows again.
//
// What it costs when this fails: the line somebody scrolled back to read is
// pulled away by the next one.
func TestScrollingBackStopsTheFollowing(t *testing.T) {
	t.Parallel()
	f := New(WithMessages(houseMessages()), richTerminal()).NewFollow()
	_, s := followScreen(f)
	uitest.Run(s, 80, 6)
	feed(f, 1, 20)
	uitest.Feed(s, followMore{})
	res := uitest.Run(s, 80, 6, "pgup")
	uitest.Golden(t, "follow-scrolled-back", res.Frame)
	before := res.Frame
	if !strings.Contains(before, "paused") || strings.Contains(before, "line 20") {
		t.Fatalf("after pgup:\n%s", before)
	}
	feed(f, 21, 30)
	res = uitest.Feed(s, followMore{})
	if strings.Split(res.Frame, "\n")[0] != strings.Split(before, "\n")[0] || !strings.Contains(res.Frame, "paused") {
		t.Errorf("a new line moved a view that was scrolled back:\n%s", res.Frame)
	}
	res = uitest.Run(s, 80, 6, "end")
	if !strings.Contains(res.Frame, "line 30") || !strings.Contains(res.Frame, "following") {
		t.Errorf("end did not follow again:\n%s", res.Frame)
	}
}

// ctrl-c leaves and says it was an interrupt, which is not the stream ending.
func TestCtrlCInterruptsAndStopEnds(t *testing.T) {
	t.Parallel()
	f := New(WithMessages(houseMessages()), richTerminal()).NewFollow()
	m, s := followScreen(f)
	if res := uitest.Run(s, 80, 6, "ctrl+c"); !res.Quit || !m.interrupted {
		t.Errorf("ctrl-c: quit %v, interrupted %v", res.Quit, m.interrupted)
	}
	// The negative half: the end of the stream quits without an interrupt, and
	// an ordinary key does neither.
	m2, s2 := followScreen(New(WithMessages(houseMessages()), richTerminal()).NewFollow())
	if res := uitest.Run(s2, 80, 6, "j"); res.Quit || m2.interrupted {
		t.Error("an ordinary key left")
	}
	if res := uitest.Feed(s2, followStop{}); !res.Quit || m2.interrupted {
		t.Errorf("Stop: quit %v, interrupted %v", res.Quit, m2.interrupted)
	}
}

// Off a terminal, a line that cannot be printed is not lost without a word: the
// error printing it met is in what Run returns.
//
// What it costs when this fails: a program that follows a stream into a closed
// pipe exits 0 with none of its lines written, and the run looks complete.
func TestALineThatCannotBePrintedIsInRunsError(t *testing.T) { //nolint:paralleltest // serial: it sets the process's streams or arguments
	// serial: it points os.Stdout at a closed file, which a parallel test would print to.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	_ = w.Close()
	saved := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = saved })
	f := New(WithMessages(houseMessages())).NewFollow()
	f.Line("one")
	f.Stop()
	if err := f.Run(); err == nil || !errors.Is(err, os.ErrClosed) {
		t.Errorf("Run = %v, and want the error printing the line met", err)
	}
}

// Off a terminal each line is printed plainly as it comes, and Run waits for
// Stop and returns nil.
func TestOffATerminalEachLineIsPrintedAsItComes(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	f := New(WithMessages(houseMessages())).NewFollow()
	out := capture(t, &os.Stdout, func() {
		go func() { time.Sleep(20 * time.Millisecond); f.Line("two"); f.Stop() }()
		f.Line("one")
		if err := f.Run(); err != nil {
			t.Errorf("Run = %v", err)
		}
		f.Line("dropped after Stop")
	})
	if out != "one\ntwo\n" {
		t.Errorf("printed %q", out)
	}
	if len(f.lines) != 0 {
		t.Errorf("off a terminal %d lines were kept", len(f.lines))
	}
}

// follows() is false for a session that is plain on a stream that renders, and for one that
// renders but is no terminal; each prints every line as it comes. The positive
// half is richTerminal, which every viewport test above runs on.
func TestAPlainOrNonInteractiveStreamPrintsEveryLine(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	for _, tc := range []struct {
		name  string
		s     *Session
		setup func()
	}{
		{"Plain", New(richTerminal(), Plain()), func() {}},
		{"not interactive", New(), func() {}},
	} {
		tc.setup()
		f := tc.s.NewFollow()
		out := capture(t, &os.Stdout, func() { f.Line("a"); f.Line("b") })
		if out != "a\nb\n" || len(f.lines) != 0 {
			t.Errorf("%s: printed %q as the lines came, kept %d", tc.name, out, len(f.lines))
		}
		f.Stop()
		if err := f.Run(); err != nil {
			t.Errorf("%s: Run = %v", tc.name, err)
		}
	}
	if !New(WithMessages(houseMessages()), richTerminal()).NewFollow().follows() {
		t.Error("a rich interactive stream does not follow, so the cases above prove nothing")
	}
}

// A line from a guest reaches the screen without its escapes and controls: on
// the plain path, in the viewport, and in what is left in the scrollback.
//
// What it costs when this fails: a VM's console writes the person's clipboard or
// clears the person's screen.
func TestALinesEscapesNeverReachTheScreen(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	const evil = "ok\x1b]52;c;ZXZpbA==\x07\x1b[31mred\x1b[0m\x1b[2J\x1b[H\ttab\r\x07end"
	check := func(where, got string) {
		t.Helper()
		for _, bad := range []string{"\x1b", "\x07", "\r", "52;c"} {
			if strings.Contains(got, bad) {
				t.Errorf("%s holds %q: %q", where, bad, got)
			}
		}
		// The viewport turns a tab into spaces, so the text is checked in its two halves.
		if !strings.Contains(got, "okred") || !strings.Contains(got, "tabend") {
			t.Errorf("%s lost the line's text: %q", where, got)
		}
	}
	f := New(WithMessages(houseMessages())).NewFollow()
	check("the plain path", capture(t, &os.Stdout, func() { f.Line(evil) }))

	f = New(WithMessages(houseMessages()), richTerminal()).NewFollow()
	_, s := followScreen(f)
	uitest.Run(s, 80, 6)
	f.Line(evil)
	check("the viewport", uitest.Feed(s, followMore{}).Frame)
	check("the kept line", f.lines[0])
	if uitest.Feed(s, followMore{}); !strings.Contains(f.lines[0], "\t") {
		t.Error("a tab was dropped with the controls")
	}
	// The negative half: the twin that keeps the line as it came would fail check.
	if strings.Contains(safeLine("x"), "\x1b") || safeLine(evil) == evil {
		t.Error("safeLine left the line alone")
	}
}

// A viewport keeps the last followKeep lines and no more; off a terminal
// nothing is kept.
func TestAFollowKeepsTheLastLinesOnly(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	f := New(WithMessages(houseMessages()), richTerminal()).NewFollow()
	feed(f, 1, followKeep+50)
	if len(f.lines) != followKeep || f.lines[0] != "line 51 of the stream" {
		t.Errorf("kept %d lines, the first %q", len(f.lines), f.lines[0])
	}
	g := New(WithMessages(houseMessages())).NewFollow()
	capture(t, &os.Stdout, func() { feed(g, 1, 300) })
	if len(g.lines) != 0 {
		t.Errorf("off a terminal %d lines were kept", len(g.lines))
	}
}

// Lines from several goroutines are printed whole and all of them, and none
// after Stop.
func TestLinesFromSeveralGoroutinesAllPrintBeforeStopReturns(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	f := New(WithMessages(houseMessages())).NewFollow()
	out := capture(t, &os.Stdout, func() {
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				for range 50 {
					f.Line(strings.Repeat("x", 30))
				}
			})
		}
		wg.Wait()
		f.Stop()
		f.Line("late")
	})
	if n := strings.Count(out, strings.Repeat("x", 30)+"\n"); n != 400 || strings.Contains(out, "late") {
		t.Errorf("%d whole lines printed, late line in output: %v", n, strings.Contains(out, "late"))
	}
}

// runOnATestTerminal runs f.Run on a program whose input and output are a pipe
// and a buffer, 80 by 6, and returns what it printed to the screen, what it left
// on standard output and the error. send is called once the program is up.
func runOnATestTerminal(t *testing.T, f *Follow, send func(in io.Writer)) (screen, left string, err error) {
	t.Helper()
	in, w := io.Pipe()
	var scr bytes.Buffer
	f.opts = []tea.ProgramOption{tea.WithInput(in), tea.WithOutput(&scr), tea.WithWindowSize(80, 6)}
	t.Cleanup(func() { _ = w.Close() })
	done := make(chan struct{})
	left = capture(t, &os.Stdout, func() {
		go func() { time.Sleep(200 * time.Millisecond); send(w); close(done) }()
		err = f.Run()
		<-done
	})
	return scr.String(), left, err
}

const altOn, altOff = "\x1b[?1049h", "\x1b[?1049l"

// Run on a terminal: ctrl-c returns ErrInterrupted, Stop returns nil with each
// line left in the scrollback once, a Stop before Run returns at once, and the
// alternate screen is entered and left each time.
func TestRunOnATerminal(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	f := New(WithMessages(houseMessages()), richTerminal()).NewFollow()
	f.Line("before")
	scr, left, err := runOnATestTerminal(t, f, func(in io.Writer) { f.Line("during"); _, _ = in.Write([]byte("\x03")) })
	if !errors.Is(err, ErrInterrupted) {
		t.Errorf("ctrl-c: Run = %v", err)
	}
	if left != "before\nduring\n" || !strings.Contains(scr, altOn) || !strings.Contains(scr, altOff) {
		t.Errorf("ctrl-c: left %q, alt screen on %v, off %v", left, strings.Contains(scr, altOn), strings.Contains(scr, altOff))
	}

	f = New(WithMessages(houseMessages()), richTerminal()).NewFollow()
	f.Line("one")
	scr, left, err = runOnATestTerminal(t, f, func(io.Writer) { f.Line("two"); f.Stop() })
	if err != nil || left != "one\ntwo\n" || !strings.Contains(scr, altOff) {
		t.Errorf("Stop: Run = %v, left %q", err, left)
	}

	// Stop before Run: Run returns at once, with the lines left.
	f = New(WithMessages(houseMessages()), richTerminal()).NewFollow()
	f.Line("early")
	f.Stop()
	start := time.Now()
	_, left, err = runOnATestTerminal(t, f, func(io.Writer) {})
	if err != nil || left != "early\n" || time.Since(start) > 3*time.Second {
		t.Errorf("Stop before Run: Run = %v, left %q after %v", err, left, time.Since(start))
	}

	// A SIGINT arrives as tea's own error, which is an interrupt too.
	f = New(WithMessages(houseMessages()), richTerminal()).NewFollow()
	_, _, err = runOnATestTerminal(t, f, func(io.Writer) {
		f.mu.Lock()
		p := f.prog
		f.mu.Unlock()
		p.Send(tea.InterruptMsg{})
	})
	if !errors.Is(err, ErrInterrupted) {
		t.Errorf("SIGINT: Run = %v", err)
	}
}
