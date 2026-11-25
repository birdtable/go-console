package ui

// The terminal face who is shown a form or a live view, the name
// typed to confirm, and the keys of the live view.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	tea "charm.land/bubbletea/v2"
)

// atTerminal is a session at a terminal 80 cells wide, its stdin a terminal
// too when in, as a person at it has.
func atTerminal(in bool) SessionOption {
	st := stream{interactive: true, width: 80}
	return onScreen(st, st, in)
}

// The terminal face is for a person at a terminal who asked for neither plain
// text nor JSON. A program driving the tool is never shown a form.
func TestOnlyAPersonAtATerminalIsAskedOrShownAScreen(t *testing.T) {
	t.Parallel()
	s, plain, json := New(atTerminal(true)), New(atTerminal(true), Plain()), New(atTerminal(true), JSON())
	if !s.CanAsk() || !s.Charm() {
		t.Fatal("at a terminal, with no flag, the Charm face is not shown")
	}
	if json.CanAsk() || json.Charm() {
		t.Error("with --json a form could be shown")
	}
	if plain.Charm() {
		t.Error("with --plain a live view could be drawn")
	}
	if !plain.CanAsk() || !plain.Accessible() {
		t.Error("with --plain a question is still asked, as plain lines")
	}
}

// Output on a terminal is not enough: a form reads its keys from standard
// input, so with input redirected nobody can answer, and nothing is drawn.
func TestRedirectedInputIsNobodyToAsk(t *testing.T) {
	t.Parallel()
	if s := New(atTerminal(false)); s.CanAsk() || s.Charm() {
		t.Error("with standard input redirected a form or a live view could be shown")
	}
}

func TestQuestionsAreAccessibleWhenAskedFor(t *testing.T) {
	t.Parallel()
	if detected("TERM=xterm").Accessible() {
		t.Error("accessible by default")
	}
	if !detected("ACCESSIBLE=1", "TERM=xterm").Accessible() {
		t.Error("ACCESSIBLE does not turn it on")
	}
	if !detected("TERM=dumb").Accessible() {
		t.Error("TERM=dumb does not turn it on")
	}
	if New(On(Detect(nil, &bytes.Buffer{}, &bytes.Buffer{}, []string{"ACCESSIBLE=1"}, EnvRules{}))).Accessible() {
		t.Error("ACCESSIBLE turned it on where the rules do not read it")
	}
}

// With no terminal to measure, COLUMNS says how wide to wrap; a terminal's
// own width wins.
func TestWidthTakesItsColumnsFromTheEnvironmentOffATerminal(t *testing.T) {
	t.Parallel()
	if w := detected().Width(); w != 0 {
		t.Errorf("no terminal and no COLUMNS is %d wide, not unknown", w)
	}
	if w := detected("COLUMNS=80").Width(); w != 80 {
		t.Errorf("COLUMNS=80 off a terminal is %d wide", w)
	}
	if w := detected("COLUMNS=500").Width(); w != maxLine {
		t.Errorf("a wide screen is %d wide, and lines stop at %d", w, maxLine)
	}
	if w := widthOf(stream{interactive: true, width: 70}); w != 70 {
		t.Errorf("a 70-wide terminal is %d wide", w)
	}
}

// answeredBy is a session at a screen reader's terminal whose person types lines: the
// real prompt, asked in plain lines, reading In a byte at a time, since huh reads
// ahead and a second question would find its line gone.
func answeredBy(lines string, out io.Writer) *Session {
	return New(append(House(), On(Terminal{Interactive: true, Accessible: true}), In(iotest.OneByteReader(strings.NewReader(lines))), Out(out))...)
}

// The name is typed in full: anything else, or nobody to type it, is no.
func TestAnyNameButTheRightOneIsNo(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	for _, c := range []struct {
		typed string
		ok    bool
		want  bool
	}{{"box:home\n", true, true}, {"claude:hom\n", true, false}, {"", false, false}, {"y\n", true, false}} {
		var out bytes.Buffer
		if got, _ := answeredBy(c.typed, &out).ConfirmName("remove box:home", "box:home", "--confirm box:home"); got != c.want {
			t.Errorf("typing %q was %v, want %v", c.typed, got, c.want)
		}
		if !strings.Contains(out.String(), "Remove box:home?") {
			t.Errorf("the answer is not written down:\n%s", out.String())
		}
	}
	if got, e := runner().ConfirmName("remove box:home", "box:home", "--confirm box:home"); got || e == nil || e.Code() != 2 {
		t.Errorf("with nobody to ask the name was typed (%v), or the default usage ending was %v, want code 2", got, e)
	}
}

// Capture catches what a person would have been shown, and leaves the run's
// own record as it was.
func TestCaptureCatchesTheLinesAndKeepsTheRecord(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	s := New(WithMessages(houseMessages()))
	var b strings.Builder
	s.Capture(&b, func() {
		s.Log("a fact")
		s.Change("stop x")
		s.End(closing{word: "done", what: "all well"})
	})
	if !strings.Contains(b.String(), "a fact") || !strings.Contains(b.String(), "would stop") || !strings.Contains(b.String(), "all well") {
		t.Errorf("caught %q", b.String())
	}
	if len(s.Changes()) != 0 || s.Changed() || s.Ended() != nil {
		t.Errorf("drawing the screen again changed how the run ended: %v %v %v", s.Changes(), s.Changed(), s.Ended())
	}
	out := capture(t, &os.Stdout, func() { s.Log("shown") })
	if !strings.Contains(out, "shown") {
		t.Error("lines still go to a buffer after Capture")
	}
}

// Two sessions that Capture at once, each in a goroutine of its own, each
// catch only their own lines: the writer a Capture swaps is its
// session's, not the process's. Both are inside their Capture before either
// prints and stay there until both have printed, so a writer shared by the
// package would catch every line in whichever Capture came last.
//
// What it costs when this fails: a live view that redraws while another
// session prints, as two command tests run side by side, shows the other's
// lines on its screen and loses its own, and every test that draws must run
// alone.
func TestTwoCapturesAtOnceEachCatchOnlyTheirOwnLines(t *testing.T) {
	t.Parallel()
	const lines = 50
	var inside, printed, wg sync.WaitGroup
	inside.Add(2)
	printed.Add(2)
	caught := map[string]*strings.Builder{"a": {}, "b": {}}
	for name, b := range caught {
		s := New()
		wg.Go(func() {
			s.Capture(b, func() {
				inside.Done()
				inside.Wait()
				for i := range lines {
					s.Log("line %s %d", name, i)
				}
				printed.Done()
				printed.Wait()
			})
		})
	}
	wg.Wait()
	compared := 0
	for name, b := range caught {
		var got []string
		for line := range strings.Lines(b.String()) {
			got = append(got, strings.TrimSuffix(line, "\n"))
		}
		if len(got) != lines {
			t.Errorf("session %s caught %d lines, want its own %d:\n%s", name, len(got), lines, b.String())
			continue
		}
		for i, line := range got {
			if want := fmt.Sprintf("line %s %d", name, i); line != want {
				t.Errorf("session %s caught %q as its line %d, want %q", name, line, i, want)
			}
			compared++
		}
	}
	if compared != 2*lines {
		t.Fatalf("compared %d lines, want %d", compared, 2*lines)
	}
}

func keys(m *liveModel, ks ...string) {
	for _, k := range ks {
		var msg tea.KeyPressMsg
		switch k {
		case "up":
			msg = tea.KeyPressMsg{Code: tea.KeyUp}
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		m.Update(msg)
	}
}

// run does what bubbletea would with the command an Update returned.
func run(m *liveModel, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if msg := cmd(); msg != nil {
		if _, ok := msg.(tea.BatchMsg); ok {
			return // the timer and a draw; the test sends the draw itself
		}
		_, next := m.Update(msg)
		run(m, next)
	}
}

func press(m *liveModel, ks ...string) {
	for _, k := range ks {
		keys(m, k)
	}
}

// The live view draws what the command prints, moves its selection, and runs
// a key's operation on the selected row only on the second press.
func TestTheLiveViewAsksBeforeItActsOnTheSelectedRow(t *testing.T) {
	t.Parallel()
	var drawn int
	var stopped []string
	s := New(WithMessages(houseMessages()))
	m := newLiveModel(s, Live{
		Frame: HouseFrame, Draw: func() []LiveItem {
			drawn++
			s.Log("two sessions run")
			return []LiveItem{{ID: "ac11", Link: "https://example.test/ac11"}, {ID: "bd22"}}
		},
		Keys: []LiveKey{
			{Key: "s", Help: "stop",
				Ask: func(it LiveItem) string { return "press s again to stop " + it.ID },
				Act: func(it LiveItem) string { stopped = append(stopped, it.ID); return "stopped " + it.ID }},
			{Key: "o", Help: "link", Act: func(it LiveItem) string { return "Open " + it.Link }},
		},
	})
	if f := m.Frame(); !strings.Contains(f, "two sessions run") || !strings.Contains(f, "selected ac11") || !strings.Contains(f, "s stop") {
		t.Errorf("the first screen is:\n%s", f)
	}
	// one s alone changes nothing
	press(m, "down")
	_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if cmd != nil || len(stopped) != 0 || !strings.Contains(m.Frame(), "press s again to stop bd22") {
		t.Fatalf("one s: stopped %v, cmd %v:\n%s", stopped, cmd != nil, m.Frame())
	}
	// another key takes the question back, and a second s asks again
	press(m, "up")
	_, cmd = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if cmd != nil || len(stopped) != 0 {
		t.Fatalf("s after moving stopped %v", stopped)
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if cmd == nil {
		t.Fatal("s twice on the same row did nothing")
	}
	run(m, cmd)
	if strings.Join(stopped, ",") != "ac11" || !strings.Contains(m.Frame(), "stopped ac11") && drawn < 2 {
		t.Errorf("stopped %v, drawn %d:\n%s", stopped, drawn, m.Frame())
	}
	press(m, "o")
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	run(m, cmd)
	if !strings.Contains(m.Frame(), "Open https://example.test/ac11") {
		t.Errorf("the link key said:\n%s", m.Frame())
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"}); cmd == nil {
		t.Error("q does not leave")
	}
}

// A slow read does not freeze the screen: the tick starts the draw as a
// command and the keys still answer while it runs; a tick during a draw
// starts no second one.
func TestASlowDrawDoesNotFreezeTheLiveView(t *testing.T) {
	t.Parallel()
	started, release := make(chan struct{}), make(chan struct{})
	first := true
	s := New(WithMessages(houseMessages()))
	m := newLiveModel(s, Live{Frame: HouseFrame, Every: time.Millisecond, Draw: func() []LiveItem { // a short tick: the batch below runs every command in it, and a real 5 s one is waited out
		if first {
			first = false
		} else {
			close(started)
			<-release
		}
		s.Log("rows")
		return []LiveItem{{ID: "a"}, {ID: "b"}}
	}})
	_, cmd := m.Update(liveTick{})
	if cmd == nil || !m.drawing {
		t.Fatal("a tick did not start a draw")
	}
	done := make(chan struct{})
	go func() {
		// the draw command runs beside the loop
		if batch, ok := cmd().(tea.BatchMsg); ok {
			for _, c := range batch {
				if msg := c(); msg != nil {
					if _, ok := msg.(liveDrawn); ok {
						m.apply(msg.(liveDrawn))
					}
				}
			}
		}
		close(done)
	}()
	<-started
	press(m, "down") // answered while the read is under way
	_, again := m.Update(liveTick{})
	if m.sel != 1 || again == nil {
		t.Errorf("selection %d while reading", m.sel)
	}
	close(release)
	<-done
}

// The live view's lines are the plain face's lines: it keeps the screen in
// 80 cells, key help included.
func TestTheLiveViewFitsEightyColumns(t *testing.T) {
	t.Parallel()
	s := New(WithMessages(houseMessages()))
	m := newLiveModel(s, Live{
		Frame: HouseFrame, Draw: func() []LiveItem { s.Log("one row"); return []LiveItem{{ID: "ac11"}} },
		Keys: []LiveKey{{Key: "s", Help: "stop"}, {Key: "o", Help: "link"}},
	})
	for line := range strings.SplitSeq(m.Frame(), "\n") {
		if len([]rune(line)) > 80 {
			t.Errorf("%d cells wide: %q", len([]rune(line)), line)
		}
	}
}

// The secret a person types is never shown and never kept: the replier
// answers by title, and off a terminal it is one line of the input.
func TestAskSecretReadsOneLineOffATerminal(t *testing.T) {
	t.Parallel()
	got, err := New(In(strings.NewReader(" the-code \nsecond line\n"))).Secret("The code", 64)
	if err != nil || got != "the-code" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := New(In(strings.NewReader(strings.Repeat("a", 80)+"\n"))).Secret("The code", 64); err == nil {
		t.Error("a line over the cap was taken")
	}
}

// With --json nothing is asked: the secret must be piped, and a terminal on
// standard input is refused rather than prompted at.
func TestPipedSecretNeverPromptsAtATerminal(t *testing.T) {
	t.Parallel()
	f, err := os.CreateTemp(t.TempDir(), "in")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	terminal := func(any) bool { return true }
	if got, err := pipedSecret(f, 64, terminal); err == nil || got != "" || !strings.Contains(err.Error(), "pipe") {
		t.Errorf("a terminal: %q, %v", got, err)
	}
	if _, err := f.WriteString("the-code\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	if got, err := PipedSecret(f, 64); err != nil || got != "the-code" {
		t.Errorf("a pipe: %q, %v", got, err)
	}
}

// AskForText takes the typed value its check passes, refuses one the check
// refuses, and off a terminal, with nobody to type, asks nothing and returns
// no value.
//
// What it costs when this fails: a malformed value reaches whatever uses it
// three steps later, or a scheduled run waits on a prompt nobody sees.
func TestAskForTextTakesWhatItsCheckPassesAndAsksNothingOffATerminal(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it reads the process's stdout.
	s := New()
	refuseBad := func(v string) error {
		if v == "bad" {
			return errors.New("not that")
		}
		return nil
	}
	n := 0
	for _, c := range []struct {
		name, typed string
		ok          bool
		check       func(string) error
		want        string
		wantOK      bool
	}{
		{"a value with no check", "box\n", true, nil, "box", true},
		{"a value the check passes", "box\n", true, refuseBad, "box", true},
		{"a value the check refuses, then one it passes", "bad\nbox\n", true, refuseBad, "box", true},
		{"backed out", "", false, refuseBad, "", false},
	} {
		var out bytes.Buffer
		if got, ok := answeredBy(c.typed, &out).AskForText("host", "", "", c.check); got != c.want || ok != c.wantOK {
			t.Errorf("%s: AskForText = %q, %v, want %q, %v", c.name, got, ok, c.want, c.wantOK)
		}
		n++
	}
	if n != 4 {
		t.Fatalf("asked %d times, want 4", n)
	}
	if got, ok := s.AskForText("host", "", "app", nil); got != "" || ok {
		t.Errorf("off a terminal AskForText = %q, %v, want nothing asked", got, ok)
	}
}
