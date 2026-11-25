package ui

// The claims this package makes about its own output. Each one was a comment
// first; a test is the only form of them that fails when the code drifts.

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// capture swaps a stream for a pipe and returns what was written to it.
func capture(t *testing.T, which **os.File, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := *which
	*which = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	f()
	*which = saved
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// captureBoth is what f printed on the process's stdout, then what it
// printed on its stderr: an ending prints on one or the other by its code.
func captureBoth(t *testing.T, f func()) string {
	t.Helper()
	var errOut string
	out := capture(t, &os.Stdout, func() { errOut = capture(t, &os.Stderr, f) })
	return out + errOut
}

// screenOpt is a session on a screen width cells wide (0, unknown) that renders
// on both streams when rich, with nobody at it.
func screenOpt(rich bool, width int) SessionOption {
	return todays(On(Terminal{Unicode: true, Colour: rich, ErrColour: rich, Width: width, Dark: true}))
}

// todays is opt with the look ui had before its themes were data:
// Contrast on a dark ground, commands in backticks painted, and lines
// 96 cells at most. The goldens are drawn in it.
func todays(opt SessionOption) SessionOption {
	return func(s *Session) {
		for _, o := range []SessionOption{opt, WithTheme(Contrast()), WithMarkup(BacktickMarkup), MaxWidth(maxLine)} {
			o(s)
		}
	}
}

// maxLine is the widest line ui drew before MaxWidth.
const maxLine = 96

// onScreen is a session whose output and error streams are out and err, and whose
// input is a terminal when in: a person is at it only when all three are.
func onScreen(out, err stream, in bool) SessionOption {
	return todays(On(Terminal{
		Unicode: true, Colour: out.rich, ErrColour: err.rich, Width: out.width, Dark: true,
		Interactive: in && out.interactive && err.interactive,
	}))
}

// bannerOf is the banner b draws for the session s, as Arrive would draw it.
func bannerOf(s *Session, b Brand) string { return banner(s.outStream(), b, "") }

// --- the rule the package exists for ----------------------------------------

// A run with no terminal must emit no escapes at all. This is the whole
// contract with the scheduler log, and the one that stops a year of
// diagnostics being unreadable.
func TestNothingPaintsWhenThereIsNoTerminal(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	run := New()
	run.SetDryRun(false)

	out := capture(t, &os.Stdout, func() {
		run.Log("filed `x` to D:\\Files\\2019, because it was new")
		run.Info("read 12 files")
		run.Change("file D:\\Files\\2019\\a.jpg")
	})
	err := capture(t, &os.Stderr, func() {
		run.Warn(closing{word: "failed", what: "could not read a.jpg", next: "app logs", code: 1})
	})

	for name, s := range map[string]string{"stdout": out, "stderr": err} {
		if strings.Contains(s, "\x1b") {
			t.Errorf("%s carried an escape sequence with no terminal to render it:\n%q", name, s)
		}
		if s == "" {
			t.Errorf("%s was empty, so this test proved nothing", name)
		}
	}
}

// Without colour, backticks are the only thing marking a command as a command.
// Stripping them unconditionally produced lines somebody would paste with the
// word "run" still in them.
func TestABacktickSurvivesWithNoColourToReplaceIt(t *testing.T) {
	t.Parallel()
	got := New(todays(On(Terminal{}))).markupOn(stream{}, "next, run `app uploads reconcile --apply`")
	if !strings.Contains(got, "`app uploads reconcile --apply`") {
		t.Errorf("the backticks were dropped with nothing to replace them: %q", got)
	}
}

// --- dry run ----------------------------------------------------------------

// Dry run: the plan and the act come from one call site, so they cannot describe
// different work.
func TestADryRunSaysWouldAndAnApplyDoesNot(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	s := New(WithMessages(houseMessages()))

	s.SetDryRun(true)
	planned := capture(t, &os.Stdout, func() { s.Change("file a.jpg") })
	s.SetDryRun(false)
	applied := capture(t, &os.Stdout, func() { s.Change("file a.jpg") })

	if !strings.Contains(planned, "would file") {
		t.Errorf("a dry run did not say would: %q", planned)
	}
	if strings.Contains(applied, "would") {
		t.Errorf("an apply said would: %q", applied)
	}
	if !strings.Contains(applied, "file") {
		t.Errorf("an apply lost the verb: %q", applied)
	}
}

// "would" joins the verb rather than preceding the line. Prefixed, it took the
// gutter and pushed the real verb into the column the paths are read from.
func TestWouldJoinsTheVerbRatherThanTakingTheGutter(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	s := New(WithMessages(houseMessages()))
	s.SetDryRun(true)

	out := capture(t, &os.Stdout, func() { s.Change("quarantine D:\\Files\\a.jpg") })
	if !strings.HasPrefix(strings.TrimLeft(out, " "), "would quarantine ") {
		t.Errorf("the verb was separated from would: %q", out)
	}
	// The longest verb must still leave a gap before its object, or the two run
	// together and the line cannot be read as a column.
	if !strings.Contains(out, "quarantine D:") && !strings.Contains(out, "quarantine  D:") {
		t.Errorf("no separation between the verb and its object: %q", out)
	}
}

// A run that changed nothing must be able to say so.
func TestChangedIsOnlySetByAnActualChange(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	s := New(WithMessages(houseMessages()))
	capture(t, &os.Stdout, func() {
		s.Log("looked at 400 files")
		s.Info("none of them was new")
	})
	if s.Changed() {
		t.Error("reporting a fact was recorded as a change")
	}

	capture(t, &os.Stdout, func() { s.Change("file a.jpg") })
	if !s.Changed() {
		t.Error("a reported mutation was not recorded as a change")
	}
}

// --- streams ----------------------------------------------------------------

// Warn paints against stderr's own detection. With the roles bound to
// stdout, `app uploads reconcile > sweep.log` at a terminal printed its failures
// unpainted while the terminal could have coloured them.
func TestAWarningIsPaintedAgainstStderrNotStdout(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it reads the process's stderr.
	// stdout redirected, stderr a terminal that paints
	errTerm := onScreen(stream{}, stream{rich: true}, false)
	got := capture(t, &os.Stderr, func() {
		New(errTerm).Warn(closing{word: "failed", what: "a.jpg moved while it was being read", next: "app logs", code: 1})
	})
	if !strings.Contains(got, "\x1b") {
		t.Errorf("stderr was a terminal and the warning was not painted: %q", got)
	}
}

// --- layout -----------------------------------------------------------------

// Zero width means "do not reflow", not "reflow to zero", which would put one
// word on each line of every log file.
func TestWrapLeavesTextAloneAtAnUnknownWidth(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a file that was filed ", 12)
	if got := New().Wrap(2, long); got != long {
		t.Errorf("text was reflowed with no screen to reflow it to:\n%q", got)
	}
}

// --- asking -----------------------------------------------------------------

// The whole reason `uploads reconcile` can run from a scheduler. A prompt on a stream nobody
// is watching would wait for an answer that is never coming.
func TestNothingAsksWhenThereIsNobodyToAsk(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	if New().CanAsk() {
		t.Fatal("a stream with no terminal reported that it could ask a question")
	}

	var confirmed bool
	var ending Ending
	out := capture(t, &os.Stderr, func() {
		confirmed, ending = runner().Confirm("format /dev/sdb", "", "--yes")
	})
	if confirmed || ending == nil || ending.Code() != 2 {
		t.Errorf("a question nobody could answer was answered %v on their behalf, or the default usage ending was %v, want code 2", confirmed, ending)
	}
	// And it says which flag would have answered it, because the alternative is
	// a refusal somebody can do nothing about.
	if ending == nil || !strings.Contains(ending.Next(), "--yes") {
		t.Errorf("the refusal does not name --yes: %v (printed %q)", ending, out)
	}
}

// A menu with nobody to show it to must return nothing rather than block, so
// the caller falls back to printing.
func TestPickReturnsNothingWithNoTerminal(t *testing.T) {
	t.Parallel()
	got, ok := New().Pick("What now?", []Choice{{Label: "a", Value: "a"}}, "--what")
	if ok || got != "" {
		t.Errorf("a menu answered itself with %q", got)
	}
}

// An empty menu is not a menu. Offering one would show a box with nothing in it
// and wait for a keypress.
func TestPickRefusesAnEmptyList(t *testing.T) {
	t.Parallel()
	if _, ok := New().Pick("What now?", nil, "--what"); ok {
		t.Error("an empty menu was offered")
	}
}

// A run that answered in advance should read like one somebody sat through, so
// a transcript of an unattended run is legible months later.
func TestAnAssumedAnswerIsStillWrittenDown(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	out := capture(t, &os.Stdout, func() { New(WithMessages(houseMessages())).Assumed("rewrite the tags on 30 files") })
	for _, want := range []string{"Rewrite the tags on 30 files?", "yes"} {
		if !strings.Contains(out, want) {
			t.Errorf("the record does not contain %q:\n%s", want, out)
		}
	}
}

// richTerminal is a session at a terminal that renders, width unknown, on both
// streams: where a test needs a person at the screen (a live view, a moving
// line), which no environment can say.
func richTerminal() SessionOption {
	return todays(On(Terminal{Unicode: true, Colour: true, ErrColour: true, Interactive: true, Dark: true}))
}

// detected is a session on two buffers and no input, with the Terminal Detect
// gives under env and the standard rules: what a program that calls Detect
// gets in a pipe.
func detected(env ...string) *Session {
	t := Detect(nil, &bytes.Buffer{}, &bytes.Buffer{}, env, StandardEnv())
	t.Dark = true
	return New(todays(On(t)))
}

// detectedTerm is the Terminal Detect gives two buffers and no input under env
// and the standard rules.
func detectedTerm(env ...string) Terminal {
	return Detect(nil, &bytes.Buffer{}, &bytes.Buffer{}, env, StandardEnv())
}

// todaysStream is a rich stream in today's look (todays), on a dark ground.
func todaysStream() stream { return New(screenOpt(true, 0)).outStream() }

// A flag is never split at its hyphen or its equals sign, at any width and at
// any column the flag could land on.
func TestWrapNeverBreaksAFlag(t *testing.T) {
	t.Parallel()
	const flag = "--no-clobber-existing-files"
	for _, width := range []int{40, 60, 80} {
		for col := range width {
			text := strings.Repeat("a", col) + " " + flag + " and then some more words after it"
			got := New(On(Terminal{Width: width})).Wrap(2, text)
			if !strings.Contains(got, flag) {
				t.Fatalf("width %d, column %d: the flag was split:\n%s", width, col, got)
			}
			for l := range strings.SplitSeq(got, "\n") {
				if strings.HasSuffix(l, "-") {
					t.Fatalf("width %d, column %d: a line ends in a hyphen: %q", width, col, l)
				}
			}
		}
	}
}

func TestWrapKeepsAnEqualsFlagWhole(t *testing.T) {
	t.Parallel()
	got := New(On(Terminal{Width: 20})).Wrap(2, "run it with --out=dir/sub-dir and look")
	if !strings.Contains(got, "--out=dir/sub-dir") {
		t.Errorf("--out=dir was split:\n%s", got)
	}
	if got != "run it with\n  --out=dir/sub-dir\n  and look" {
		t.Errorf("wrapped as %q", got)
	}
}

func TestWrapLetsAWordWiderThanTheLineOverflow(t *testing.T) {
	t.Parallel()
	word := strings.Repeat("x", 30)
	got := New(On(Terminal{Width: 20})).Wrap(0, "a "+word+" b")
	if got != "a\n"+word+"\nb" {
		t.Errorf("wrapped as %q", got)
	}
}

func TestWrapKeepsNewlinesAndIndentsContinuations(t *testing.T) {
	t.Parallel()
	got := New(On(Terminal{Width: 12})).Wrap(2, "one two three\nfour five six")
	if want := "one two\n  three\n  four five\n  six"; got != want {
		t.Errorf("wrapped as %q, want %q", got, want)
	}
}
