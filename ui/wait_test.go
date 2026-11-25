package ui

// A Wait is the progress line for a measurement, and the measurement is of a
// machine left alone. What has to be true is that it says enough to tell a long
// wait from a hang, and does nothing in between.

import (
	"os"
	"strings"
	"testing"
	"time"
)

// clockAt makes a Wait whose time is whatever the test says it is.
// It starts again at that time, so its first line is not measured against the
// real clock.
func clockAt(w *Wait, t *time.Time) {
	w.now = func() time.Time { return *t }
	w.started, w.lastLog = *t, *t
}

// A Wait draws only when it is told to.
//
// What it costs when this fails: a progress line with its own timer wakes the
// processor during the idle it is measuring, and over ssh sends a packet each
// time, so `measure` reports a machine busier than it is because it is
// watching it.
func TestAWaitDrawsNothingBetweenTheCommandsWakeUps(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: capture replaces os.Stdout, which the package's other tests write to
	scr := screenOpt(true, 0)
	out := capture(t, &os.Stdout, func() {
		w := New(scr).NewWait("measuring", 2*time.Second)
		time.Sleep(400 * time.Millisecond)
		w.Stop()
	})
	// One draw from NewWait, one wipe from Stop. A timer of its own would have
	// drawn a few more in 400 ms.
	if draws := strings.Count(out, "measuring"); draws != 1 {
		t.Errorf("drew %d times with nobody asking it to; it has a clock of its own:\n%q", draws, out)
	}
}

// With no terminal it says it is going, now and then, and not on every call.
//
// What it costs when this fails: a measurement run from a timer writes a line
// every five seconds for an hour, or nothing at all, and neither is a log.
func TestAWaitWithNoTerminalWritesALineAMinute(t *testing.T) { //nolint:paralleltest // serial: it sets the process's environment or directory
	// serial: capture replaces os.Stdout, which the package's other tests write to
	at := time.Unix(0, 0)
	out := capture(t, &os.Stdout, func() {
		w := New().NewWait("measuring", 10*time.Minute)
		clockAt(w, &at)
		for range 120 { // ten minutes of five-second wake-ups
			at = at.Add(5 * time.Second)
			w.Set("")
		}
		w.Stop()
	})
	lines := strings.Count(out, "measuring")
	if lines < 9 || lines > 12 {
		t.Errorf("ten minutes wrote %d lines, not about one a minute:\n%s", lines, out)
	}
}

// Past the end it says it is finishing, and never a negative time left.
//
// What it costs when this fails: a slow plug makes the last window run over,
// and the line reads "about -3s left", which looks like a fault in the tool.
func TestAWaitThatRunsOverSaysItIsFinishing(t *testing.T) {
	t.Parallel()
	w := &Wait{total: time.Minute}
	if got := w.howFar(70 * time.Second); !strings.Contains(got, "finishing") || strings.Contains(got, "-") {
		t.Errorf("70 s into a minute reads %q", got)
	}
	if got := w.howFar(20 * time.Second); !strings.Contains(got, "about 40s left") {
		t.Errorf("20 s into a minute reads %q", got)
	}
}

// A command that is not drawing holds a nil Wait, and that has to be harmless.
//
// What it costs when this fails: `measure --json`, whose stdout is a contract,
// either panics or prints a progress line into the JSON.
func TestANilWaitDoesNothing(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: capture replaces os.Stdout, which the package's other tests write to
	out := capture(t, &os.Stdout, func() {
		var w *Wait
		w.Set("anything")
		w.Stop()
	})
	if out != "" {
		t.Errorf("a nil Wait printed %q", out)
	}
}

// With the stopwatch on, stopping says how long the wait took, once; with it
// off, which is the default, it says nothing.
//
// What it costs when this fails: a person who turned the stopwatch on is not told
// how long the wait took, or one who left it off finds a time line in output a
// script reads.
func TestWaitSaysHowLong(t *testing.T) { //nolint:paralleltest // serial: it sets the process's environment or directory
	// serial: capture replaces os.Stdout, which the package's other tests write to
	for _, rich := range []bool{false, true} {
		scr := screenOpt(rich, 0)
		at := time.Unix(0, 0)
		var took time.Duration
		out := capture(t, &os.Stdout, func() {
			w := New(scr).NewWait("measuring", time.Minute).Stopwatch()
			clockAt(w, &at)
			at = at.Add(75 * time.Second)
			took = w.Stop()
			at = at.Add(time.Hour)
			if again := w.Stop(); again != took {
				t.Errorf("stopping twice said %v then %v", took, again)
			}
		})
		if took != 75*time.Second || strings.Count(out, "measuring took 1m15s") != 1 {
			t.Errorf("rich %v: took %v, said\n%q", rich, took, out)
		}
		out = capture(t, &os.Stdout, func() {
			w := New(scr).NewWait("measuring", time.Minute)
			clockAt(w, &at)
			at = at.Add(75 * time.Second)
			w.Stop()
		})
		if strings.Contains(out, "took") {
			t.Errorf("rich %v: the stopwatch is on by default:\n%q", rich, out)
		}
	}
	if (*Wait)(nil).Stopwatch().Stop() != 0 {
		t.Error("a nil Wait took time")
	}
}
