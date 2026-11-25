package ui

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/birdtable/go-run/exit"

	"github.com/birdtable/go-console/uitest"
)

// closing is an Ending as a program hands one in: exit.Ending's shape, built
// here because ui imports no other module of ours, in its code or its tests.
type closing struct {
	word, what, next string
	code             int
}

func (c closing) Word() string { return c.word }
func (c closing) What() string { return c.what }
func (c closing) Next() string { return c.next }
func (c closing) Code() int    { return c.code }

// The endings exit builds, word, line and code as exit's table has them.
func done() closing { return closing{word: "done", what: "ready"} }
func needsYou() closing {
	return closing{word: "needs you", what: "updates are waiting", next: "app updates"}
}
func stopped() closing {
	return closing{word: "stopped at", what: "cut short", next: "sudo app measure apps --apply"}
}
func refused() closing {
	return closing{word: "refused", what: "not Debian", next: "app help", code: 5}
}
func usage() closing {
	return closing{word: "refused", what: "say which one", next: "app run --name x", code: 2}
}
func failed() closing {
	return closing{word: "failed", what: "the step did not work", next: "app logs", code: 1}
}
func findings() closing { return closing{word: "findings", what: "3 problems in 2 files", code: 1} }
func declined() closing { return closing{word: "declined", what: "the plan was not applied", code: 6} }
func busy() closing {
	return closing{word: "busy", what: "a build is running", next: "app wait", code: 3}
}
func notReady() closing {
	return closing{word: "not ready", what: "no image yet", next: "app image pull", code: 4}
}

// End prints the word, the sentence and, last, the line to type, records the
// ending in its session for the run log, and returns the code the ending
// carries, whatever the code: ui decides none of them.
//
// What it costs when this fails: `stopped at` with no way to continue, a run
// log that does not know how a run ended, or an exit code that is not the
// one exit's table gives the word.
func TestEndPrintsTheEndingLastAndReturnsItsCode(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it reads the process's stdout and stderr.
	n := 0
	for _, e := range []closing{done(), needsYou(), stopped(), refused(), usage(), failed(), findings(), declined(), busy(), notReady(), {word: "done", what: "x", code: 7}} {
		s := New()
		var code int
		out := captureBoth(t, func() { _ = s.End(e); code = s.ExitCode() })
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if !strings.Contains(out, e.word) || !strings.Contains(out, e.what) || code != e.code {
			t.Errorf("%s: exit %d, want %d, printed %q", e.word, code, e.code, out)
		}
		if e.next != "" && !strings.Contains(lines[len(lines)-1], e.next) {
			t.Errorf("%s: the line to type is not the last line: %q", e.word, out)
		}
		if got := s.Ended(); got == nil || got.Word() != e.word || got.Code() != e.code {
			t.Errorf("%s: the run log would not know this run's ending: %v", e.word, got)
		}
		n++
	}
	if n != 11 {
		t.Fatalf("ended %d runs, want 11", n)
	}
}

// A held session's ending, as a step run inside another command, and an
// ending with no word print nothing and are not recorded, and still return
// their code.
//
// What it costs when this fails: a setup step's dry run prints its own
// closing word in the middle of the command around it, which then ends on a
// second one, or the run log keeps the step's word as the run's.
func TestAHeldEndingAndOneWithNoWordPrintNothing(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it reads the process's stdout.
	held := New(Held())
	if out := capture(t, &os.Stdout, func() {
		if _ = held.End(refused()); held.ExitCode() != 5 {
			t.Errorf("a held refusal kept code %d, want 5", held.ExitCode())
		}
	}); out != "" || held.Ended() != nil {
		t.Errorf("a held ending printed %q and recorded %v", out, held.Ended())
	}
	s := New()
	if out := capture(t, &os.Stdout, func() {
		if _ = s.End(closing{what: "x", code: 1}); s.ExitCode() != 1 {
			t.Errorf("an ending with no word kept code %d, want 1", s.ExitCode())
		}
	}); out != "" || s.Ended() != nil {
		t.Errorf("an ending with no word printed %q and recorded %v", out, s.Ended())
	}
}

// An ending straight after the banner, or after any empty line, keeps one
// empty line above it, not two.
//
// What it costs when this fails: a command with nothing named prints its
// refusal two lines below the banner, where every other screen has one.
func TestAGapAfterAGapIsOneLine(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it reads the process's stdout, its stderr and the last line printed.
	s := New()
	out := captureBoth(t, func() {
		s.Log("")
		s.End(closing{word: "refused", what: "say which one", code: 5})
	})
	if strings.HasPrefix(out, "\n\n") || !strings.HasPrefix(out, "\n  refused") {
		t.Errorf("after an empty line, the ending printed %q", out)
	}
	out = capture(t, &os.Stdout, func() {
		s.Log("a row")
		New().End(closing{word: "done", what: "nothing runs"})
	})
	if !strings.HasPrefix(out, "a row\n\n  done") {
		t.Errorf("after a row, the ending printed %q", out)
	}
}

// EndsBlank says whether the last line its session printed was empty: after
// an empty line it is, and after a row or an ending it is not.
//
// What it costs when this fails: a help page handed the screen after an
// empty line prints a second one, or one after a row runs straight on from
// it.
func TestEndsBlankSaysWhetherTheLastLinePrintedWasEmpty(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it reads the process's stdout.
	s := New()
	n := 0
	for _, c := range []struct {
		name  string
		print func()
		blank bool
	}{
		{"an empty line", func() { s.Log("") }, true},
		{"a row", func() { s.Log("a row") }, false},
		{"an empty line again", func() { s.Log("") }, true},
		{"an ending", func() { s.End(done()) }, false},
	} {
		capture(t, &os.Stdout, c.print)
		if got := s.EndsBlank(); got != c.blank {
			t.Errorf("after %s, EndsBlank is %v, want %v", c.name, got, c.blank)
		}
		n++
	}
	if n != 4 {
		t.Fatalf("printed %d lines, want 4", n)
	}
}

// An ending that did not get there and gives no line to type is printed and
// kept with this command's help as its line: a refusal (the table's usage, busy, not ready and refused codes) always,
// and a failure, told from findings by the word of the session's Endings.
// Findings, declined and done, which have no line by design, are left as they
// are, and so is a failure in a session with no Endings, which cannot tell
// it from findings, and every ending that gives its own line.
//
// What it costs when this fails: a person stopped by a refusal or a failure
// has nothing to type next and is stuck, or a run that found problems or was
// declined is told to read the help as if it had been typed wrong.
func TestAnEndingThatStopsWithNoLineGetsTheCommandsHelp(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it reads the process's stdout and stderr.
	help := lineFor("", "", "--help")
	lineless := func(c closing) closing { c.next = ""; return c }
	n := 0
	for _, c := range []struct {
		name string
		s    *Session
		e    closing
		want string
	}{
		{"usage", runner(), lineless(usage()), help},
		{"busy", runner(), lineless(busy()), help},
		{"not ready", runner(), lineless(notReady()), help},
		{"refused", runner(), lineless(refused()), help},
		{"failed, with Endings", runner(), lineless(failed()), help},
		{"findings", runner(), findings(), ""},
		{"declined", runner(), declined(), ""},
		{"done", runner(), done(), ""},
		{"failed, with no Endings", New(), lineless(failed()), ""},
		{"refused with its own line", runner(), refused(), refused().next},
		{"failed with its own line", runner(), failed(), failed().next},
	} {
		out := captureBoth(t, func() { c.s.End(c.e) })
		got := c.s.Ended()
		if got == nil || got.Next() != c.want || got.Word() != c.e.word || got.Code() != c.e.code {
			t.Errorf("%s: kept %+v, want %s with line %q", c.name, got, c.e.word, c.want)
		}
		if printed := strings.Contains(out, "--help"); printed != (c.want == help) {
			t.Errorf("%s: printed %q, want the help line printed: %v", c.name, out, c.want == help)
		}
		n++
	}
	if n != 11 {
		t.Fatalf("ended %d sessions, want 11", n)
	}
}

// With stdout a pipe, a done ending (code 0) prints on stdout and every other
// on stderr, so `app | grep` reads no failure as output; one on stderr is
// painted as stderr can be, a terminal while stdout is not. Under --json
// neither prints anything, on either stream.
//
// What it costs when this fails: a script that pipes a run's stdout reads
// "failed" or "refused" as a line of output, or a failure at a terminal is
// printed unpainted because stdout was redirected.
func TestAnEndingThatDidNotGetThereGoesToStderr(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it sets the process's streams and reads its stdout and stderr.
	errRich := onScreen(stream{}, stream{rich: true}, false)
	n := 0
	for _, c := range []struct {
		e        closing
		toStderr bool
	}{
		{done(), false}, {needsYou(), false}, {stopped(), false},
		{failed(), true}, {findings(), true}, {usage(), true}, {refused(), true}, {busy(), true}, {notReady(), true}, {declined(), true},
	} {
		var stderr string
		stdout := capture(t, &os.Stdout, func() { stderr = capture(t, &os.Stderr, func() { New(errRich, houseEndings()).End(c.e) }) })
		on, off := stdout, stderr
		if c.toStderr {
			on, off = stderr, stdout
		}
		if !strings.Contains(on, c.e.word) || !strings.Contains(on, c.e.what) || off != "" {
			t.Errorf("%s: printed %q on stdout and %q on stderr, want it all on stderr: %v", c.e.word, stdout, stderr, c.toStderr)
		}
		if painted := strings.Contains(stderr, "\x1b["); painted != c.toStderr {
			t.Errorf("%s: painted on stderr %v, want %v: %q", c.e.word, painted, c.toStderr, stderr)
		}
		n++
	}
	var stderr string
	stdout := capture(t, &os.Stdout, func() { stderr = capture(t, &os.Stderr, func() { New(JSON()).End(failed()) }) })
	if stdout != "" || stderr != "" {
		t.Errorf("under --json a failure printed %q on stdout and %q on stderr, want nothing", stdout, stderr)
	}
	if n != 10 {
		t.Fatalf("ended %d runs, want 10", n)
	}
}

// Each ending is painted by the role its program gave its kind: done good; needs you, stopped at, busy, not
// ready and declined amber; failed, findings, usage and refused red Findings and declined are painted apart.
//
// What it costs when this fails: a run that only waits on somebody reads red
// in a column of two hundred, or one that failed reads green.
func TestEachEndingIsPaintedByItsKind(t *testing.T) {
	t.Parallel()
	good, attn, bad := colour(done()), colour(needsYou()), colour(failed())
	if good == attn || attn == bad || good == bad || !strings.Contains(bad, "\x1b[") {
		t.Fatalf("the three paints are not apart: %q, %q, %q", good, attn, bad)
	}
	n := 0
	for _, c := range []struct {
		e    closing
		want string
	}{
		{done(), good}, {needsYou(), attn}, {stopped(), attn}, {busy(), attn}, {notReady(), attn}, {declined(), attn},
		{failed(), bad}, {findings(), bad}, {usage(), bad}, {refused(), bad},
	} {
		if got := colour(c.e); got != c.want {
			t.Errorf("%s (code %d) is painted %q, want %q", c.e.word, c.e.code, got, c.want)
		}
		n++
	}
	if n != 10 {
		t.Fatalf("painted %d endings, want 10", n)
	}
}

// Findings, declined, busy and not ready each have a golden of the line they
// print.
//
// What it costs when this fails: a closing line's look moves and no reviewer
// sees it.
func TestFourClosingWordsHaveGoldens(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it reads the process's stdout.
	// The goldens start after a line of output, as a run's ending does, so
	// each opens with its gap: a new session's last line is not blank.
	n := 0
	for _, e := range []closing{findings(), declined(), busy(), notReady()} {
		out := captureBoth(t, func() { New().End(e) })
		uitest.Golden(t, "ending-"+strings.ReplaceAll(e.word, " ", "-"), out)
		n++
	}
	if n != 4 {
		t.Fatalf("checked %d endings, want 4", n)
	}
}

// colour is the paint an ending is given by the house's kinds, at a terminal 80 cells wide that renders:
// Paint and End's one implementation, paintOn.
func colour(e Ending) string {
	return colourBy(New(houseEndings()), e)
}

// colourBy is the paint run gives e, without the word.
func colourBy(run *Session, e Ending) string {
	return strings.ReplaceAll(run.paintOn(stream{rich: true, width: 80}, e, "w"), "w", "")
}

// Two sessions that end and change at once, each in goroutines of its own,
// while more read their records, each keep only their own ending and changes: the record is the session's, so two runs in one process, as
// two command tests, never see each other's, and a session is read under its
// lock.
//
// What it costs when this fails: a command test that runs beside another
// reads the other's closing word or changes in its run log, and every such
// test must run alone; or a run log read while the run ends is torn.
func TestTwoSessionsAtOnceEachKeepTheirOwnRecord(t *testing.T) {
	t.Parallel()
	a, b := New(WithMessages(houseMessages()), JSON()), New(WithMessages(houseMessages()), JSON())
	var wg sync.WaitGroup
	rounds := 0
	for _, run := range []struct {
		s      *Session
		e      closing
		change string
	}{{a, done(), "write /a"}, {b, refused(), "write /b"}} {
		wg.Go(func() {
			for i := range 200 {
				run.s.Change("%s%d", run.change, i)
			}
		})
		wg.Go(func() {
			for range 200 {
				run.s.End(run.e)
			}
		})
		wg.Go(func() {
			for range 200 {
				_, _ = run.s.Ended(), run.s.Changes()
			}
		})
		rounds++
	}
	wg.Wait()
	if rounds != 2 {
		t.Fatalf("started %d sessions, want 2", rounds)
	}
	if a.Ended() != done() || b.Ended() != refused() {
		t.Errorf("the sessions ended on %v and %v, want each its own", a.Ended(), b.Ended())
	}
	for name, s := range map[string]*Session{"write /a": a, "write /b": b} {
		if got := s.Changes(); len(got) != 200 || got[199] != fmt.Sprintf("%s%d", name, 199) {
			t.Errorf("the session of %s kept %d changes, want 200 of its own: %q", name, len(got), got)
		}
	}
}

// A session that ends while another draws in its Capture, in goroutines of
// their own, keeps its ending, and the one that draws keeps none: what holds
// an ending back is that session's own drawing, not the package's.
//
// What it costs when this fails: a command test that ends while another
// test's live view redraws loses its closing word from its run log, and
// sometimes, as the two happen to overlap.
func TestASessionThatEndsWhileAnotherDrawsKeepsItsEnding(t *testing.T) {
	t.Parallel()
	rounds := 0
	for range 50 {
		drawer, ender := New(JSON()), New(JSON())
		drawing, ended := make(chan struct{}), make(chan struct{})
		var wg sync.WaitGroup
		wg.Go(func() {
			var b strings.Builder
			drawer.Capture(&b, func() {
				drawer.End(done())
				close(drawing)
				<-ended
			})
		})
		wg.Go(func() {
			<-drawing
			ender.End(refused())
			close(ended)
		})
		wg.Wait()
		if drawer.Ended() != nil || ender.Ended() != refused() {
			t.Fatalf("the drawing session kept %v and the ending one %v, want nothing and its own", drawer.Ended(), ender.Ended())
		}
		rounds++
	}
	if rounds != 50 {
		t.Fatalf("ran %d rounds, want 50", rounds)
	}
}

// An ending on stderr is laid out for stderr, its markup and its width: with
// stdout a terminal that paints and stderr a file, the word, the sentence and
// the line to type reach stderr with no escape, its backticks kept, and not
// wrapped to stdout's width. With stderr the terminal, the same ending is
// painted there.
//
// What it costs when this fails: `app 2>log` at a terminal fills the log with
// colour codes, or wraps its lines to a screen the log is not on.
func TestAnEndingOnStderrIsLaidOutForStderrNotStdout(t *testing.T) {
	// serial: it sets the process's streams and environment.
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLUMNS", "")
	e := closing{word: "failed", what: "run `app init` first, then read app.toml and the notes beside it", next: "app edit app.toml", code: 1}
	outAt30 := onScreen(stream{rich: true, width: 30}, stream{}, false)
	var stderr string
	stdout := capture(t, &os.Stdout, func() { stderr = capture(t, &os.Stderr, func() { New(outAt30).End(e) }) })
	lines := strings.Split(strings.Trim(stderr, "\n"), "\n")
	if strings.Contains(stderr, "\x1b") || stdout != "" {
		t.Errorf("stderr a file got escapes laid out for stdout, or stdout got the ending: %q, %q", stderr, stdout)
	}
	if len(lines) != 2 || !strings.Contains(lines[0], "`app init`") || !strings.Contains(lines[0], "app.toml") || !strings.Contains(lines[1], "app edit app.toml") {
		t.Errorf("the ending on stderr was wrapped to stdout's width or lost its words: %q", stderr)
	}
	// The negative half: stderr the terminal that paints, stdout a file.
	errRich := onScreen(stream{}, stream{rich: true}, false)
	stderr = capture(t, &os.Stderr, func() { New(errRich).End(e) })
	if !strings.Contains(stderr, "\x1b") || strings.Contains(stderr, "`app init`") {
		t.Errorf("stderr a terminal got the ending unpainted, so the check above proves nothing: %q", stderr)
	}
}

// What an ending says comes from outside as often as not (a file name, an
// error), so its sentence and its line to type are printed as Name prints
// them, on stdout and on stderr, and so is a warning's line: no escape,
// return or newline of theirs reaches the screen raw.
//
// What it costs when this fails: a file named to erase the line rewrites a
// failed run's last words as "done", or a hidden escape hides the line to
// type, in the one place a person reads to decide what to do.
func TestAnEndingsWordsFromOutsidePrintAsNamePrintsThem(t *testing.T) {
	// serial: it sets the process's streams and environment.
	t.Setenv("COLUMNS", "")
	ran := 0
	for _, data := range forged() {
		for _, p := range []struct {
			name   string
			stream **os.File
			lines  int
			print  func(s *Session)
		}{
			{"End's sentence on stdout", &os.Stdout, 1, func(s *Session) { s.End(closing{word: "done", what: data}) }},
			{"End's line on stdout", &os.Stdout, 2, func(s *Session) { s.End(closing{word: "needs you", what: "x", next: "app " + data}) }},
			{"End's sentence on stderr", &os.Stderr, 2, func(s *Session) { s.End(closing{word: "failed", what: data, next: "app logs", code: 1}) }},
			{"End's line on stderr", &os.Stderr, 2, func(s *Session) { s.End(closing{word: "failed", what: "x", next: "app " + data, code: 1}) }},
			{"Warn's line", &os.Stderr, 2, func(s *Session) { s.Warn(closing{word: "failed", what: "x", next: "app " + data, code: 1}) }},
		} {
			out := strings.Trim(capture(t, p.stream, func() { p.print(New()) }), "\n")
			if c := controls(strings.ReplaceAll(out, "\n", "")); len(c) > 0 || strings.Count(out, "\n")+1 != p.lines {
				t.Errorf("%s(%q): control bytes %q, or not %d lines, in %q", p.name, data, c, p.lines, out)
			}
			if !strings.Contains(out, Name(data)) {
				t.Errorf("%s(%q): %q does not say it as Name does, %q", p.name, data, out, Name(data))
			}
			ran++
		}
	}
	if ran != 5*len(forged()) || ran == 0 {
		t.Fatalf("printed %d endings, want %d", ran, 5*len(forged()))
	}
}

// Warn fills a refusal's or a failure's missing line as End does, with the
// command's help, and leaves findings with none.
//
// What it costs when this fails: a warning that a step failed leaves a person
// with nothing to type, though the same failure as an ending would give one.
func TestALinelessWarnGetsTheCommandsHelpAsEndDoes(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it sets the process's streams and reads its stderr.
	s := runner()
	path := "app go"
	s.path.Store(&path)
	n := 0
	for _, c := range []struct {
		e    closing
		help bool
	}{
		{closing{word: "failed", what: "the step did not work", code: 1}, true},
		{closing{word: "refused", what: "not Debian", code: 5}, true},
		{closing{word: "findings", what: "3 problems", code: 1}, false},
	} {
		got := capture(t, &os.Stderr, func() { s.Warn(c.e) })
		if strings.Contains(got, "app go --help") != c.help {
			t.Errorf("%s: warned %q, want the help: %v", c.e.word, got, c.help)
		}
		n++
	}
	if n != 3 {
		t.Fatalf("warned %d times, want 3", n)
	}
}

// A failure ended inside a Capture is drawn into the Capture's writer, as a
// done ending is, and nothing of it reaches the process's stderr.
//
// What it costs when this fails: a screen drawn again for a live view leaves
// its failure on the terminal under the view, or the view shows no ending.
func TestAFailureEndedInsideACaptureIsDrawnThere(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it reads the process's stdout and stderr.
	n := 0
	for _, e := range []closing{failed(), done()} {
		s := New()
		var b strings.Builder
		var stdout string
		stderr := capture(t, &os.Stderr, func() {
			stdout = capture(t, &os.Stdout, func() { s.Capture(&b, func() { s.End(e) }) })
		})
		if !strings.Contains(b.String(), e.word) || stderr != "" || stdout != "" {
			t.Errorf("%s: the Capture caught %q, stderr got %q and stdout %q", e.word, b.String(), stderr, stdout)
		}
		n++
	}
	if n != 2 {
		t.Fatalf("captured %d endings, want 2", n)
	}
}

// A failure on stderr under `--no-color` holds no escape at a stderr that
// paints, and the same failure without it is painted.
//
// What it costs when this fails: a person who asked for no colour gets it on
// every failure, the one line they most need to read in a log.
func TestAFailureOnStderrUnderNoColorHoldsNoEscape(t *testing.T) {
	// serial: it sets the process's streams and environment.
	errRich := onScreen(stream{}, stream{rich: true}, false)
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	plain := New(errRich)
	plain.noColor.Store(true)
	if got := capture(t, &os.Stderr, func() { plain.End(failed()) }); strings.Contains(got, "\x1b") || !strings.Contains(got, "failed") {
		t.Errorf("under `--no-color` the failure printed %q", got)
	}
	if got := capture(t, &os.Stderr, func() { New(errRich).End(failed()) }); !strings.Contains(got, "\x1b") {
		t.Errorf("with colour the failure printed no escape, so the check above proves nothing: %q", got)
	}
}

// A run's Result for an error with no ending says code 1, a failure's, and
// for no error and no ending code 0.
//
// What it costs when this fails: a script reads code 0 from a run that
// failed before it could end, and goes on as if it had worked.
func TestAResultOfAnErrorWithNoEndingSaysCodeOne(t *testing.T) {
	t.Parallel()
	if r := New(JSON()).Result(errors.New("it broke")); r.Code != 1 || r.Error == nil || r.Error.Msg != "it broke" {
		t.Errorf("an error with no ending answered code %d and error %+v, want 1 and it broke", r.Code, r.Error)
	}
	if r := New(JSON()).Result(nil); r.Code != 0 || r.Error != nil {
		t.Errorf("no error and no ending answered code %d and error %+v, want 0 and none", r.Code, r.Error)
	}
}

// An ending is painted by its kind and never by its code: a table whose codes are
// the house's turned about, done on 3 and busy on 0, paints done good and busy amber all the same, and a
// kind given no role is painted in none.
//
// What it costs when this fails: a program with codes of its own, as sysexits' 64 and 75, reads a run
// that waits in the colour of one that failed, or a failure in the colour of a done.
func TestAnEndingIsPaintedByItsKindWhateverItsCode(t *testing.T) {
	t.Parallel()
	tab, err := exit.New(
		exit.Kind{Name: "ok", Word: "all done", Code: 3},
		exit.Kind{Name: "wait", Word: "waiting", Code: 0},
		exit.Kind{Name: "bad", Word: "broke", Code: 6},
		exit.Kind{Name: "meh", Word: "shrug", Code: 2},
	)
	if err != nil {
		t.Fatal(err)
	}
	run := New(Endings(tab, EndKinds{Failed: "bad", Usage: "bad", Declined: "wait", Busy: "wait", NotReady: "wait", Refused: "bad",
		Roles: map[string]EndRole{"ok": EndGood, "wait": EndAttn, "bad": EndBad}}))
	good, attn, bad := colourBy(run, tab.End("ok", "x", "")), colourBy(run, tab.End("wait", "x", "")), colourBy(run, tab.End("bad", "x", ""))
	if good == attn || attn == bad || good == bad || !strings.Contains(bad, "\x1b[") {
		t.Fatalf("the three paints are not apart: %q, %q, %q", good, attn, bad)
	}
	if got := colourBy(run, tab.End("meh", "x", "")); got != "" {
		t.Errorf("a kind with no role is painted %q, want none", got)
	}
	if got := colourBy(run, closing{word: "no such word", code: 1}); got != "" {
		t.Errorf("a word of no kind is painted %q, want none", got)
	}
}
