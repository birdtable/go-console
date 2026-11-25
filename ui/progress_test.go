package ui

// What a log says while a long job is running, which is the only thing anybody
// has to go on: the scheduled task writes to a file, and a person reading that
// file the morning after cannot see a spinner.

import (
	"os"
	"strings"
	"testing"
	"time"
)

// reportingEvery shrinks both thresholds for the session it is given to, so a claim
// about a five-hour run can be made in milliseconds.
func reportingEvery(items int, after time.Duration) SessionOption {
	return func(s *Session) { s.logEvery, s.logAfter = items, after }
}

// A job that never reaches the item threshold still has to say it is moving.
//
// What it costs when this fails: an index over 2,914 files printed
// `reading 0/2914  0%` and then nothing at all until it finished, and there is
// no way to tell that from a run that hung on the first file.
func TestALogSaysSomethingBeforeTheItemCountIsReached(t *testing.T) { //nolint:paralleltest // serial: it sets the process's environment or directory

	out := capture(t, &os.Stdout, func() {
		b := New(reportingEvery(1_000_000, 20*time.Millisecond)).NewBar("reading", 2914)
		for range 6 {
			time.Sleep(25 * time.Millisecond)
			b.Add(1)
		}
		b.Stop()
	})

	// The opening line, the closing line, and the heartbeats between them. Two
	// is the smallest number that is more than "it started and it ended".
	lines := strings.Count(out, "reading")
	if lines < 4 {
		t.Errorf("a job well under the item threshold wrote %d lines over six "+
			"batches, so a log of it is a start and a finish with silence in "+
			"between:\n%s", lines, out)
	}
	if !strings.Contains(out, "/2914") {
		t.Errorf("the lines do not say how far along it is:\n%s", out)
	}
}

// And the other half of "whichever comes first": a fast job must not turn its
// own item count into that many lines of log.
//
// What it costs when this fails: 165,000 files become 165,000 lines, and
// the log stops being something a person opens.
func TestAFastJobDoesNotWriteALineAnItem(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams

	out := capture(t, &os.Stdout, func() {
		b := New(reportingEvery(4, time.Hour)).NewBar("reading", 40)
		for range 40 {
			b.Add(1)
		}
		b.Stop()
	})

	// Forty items, a threshold of four: ten heartbeats, plus the opening and
	// closing lines. Anything near forty means the threshold is not being read.
	if lines := strings.Count(out, "reading"); lines > 13 {
		t.Errorf("forty items at a threshold of four wrote %d lines:\n%s", lines, out)
	}
}

// The rate and the estimate follow the run, rather than remembering all of it.
//
// What it costs when this fails: a tagging pass over 165,440 real files
// spent three hours competing with an Immich scan, then ran at 166 a second
// once that finished, and the bar went on saying `7 a second, about 3h 23m
// left` when the true answer was eight minutes. The three slow hours were still
// in the average and always would be. An estimate that cannot recover from a
// slow patch is one nobody believes twice.
func TestTheRateFollowsTheRunRatherThanTheWholeOfIt(t *testing.T) {
	t.Parallel()
	b := &Bar{label: "reading", total: 10000, started: time.Now().Add(-time.Hour)}

	// An hour of crawling: 600 items, which is a lifetime average well under
	// one a second.
	slowUntil := time.Now().Add(-2 * windowFor)
	b.done = 600
	b.note(slowUntil)

	// Then it speeds up: about a hundred a second for the last thirty.
	b.done = 1600
	b.note(time.Now().Add(-30 * time.Second))
	b.done = 4600
	b.note(time.Now())

	got := b.perSecond(time.Since(b.started))
	if got < 50 {
		t.Errorf("the bar reports %.1f a second after a burst of about a hundred "+
			"a second, so it is still averaging over the slow hour", got)
	}
	// And the estimate follows it: 5,800 left at that rate is minutes.
	left := remaining(stream{}, b.done, b.total, got, time.Since(b.started))
	if strings.Contains(left, "h ") {
		t.Errorf("the estimate still reads in hours: %q", left)
	}
	if left == "" {
		t.Error("no estimate at all, so this asserts nothing about it")
	}
}

// And before there is a window it still says something, from the whole run.
func TestWithNoWindowYetTheRateIsTheWholeRun(t *testing.T) {
	t.Parallel()
	b := &Bar{label: "reading", total: 100, started: time.Now().Add(-10 * time.Second)}
	b.done = 50
	if got := b.perSecond(10 * time.Second); got < 4 || got > 6 {
		t.Errorf("50 items in 10 seconds reads as %.1f a second", got)
	}
}

// The closing line counts what the bar counted: apps, for app's update
// check, not files.
func TestTheClosingLineCountsWhatWasCounted(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	out := capture(t, &os.Stdout, func() {
		b := New().NewBar("checking", 12).Counting("app", "apps")
		b.Add(12)
		b.Stop()
	})
	if !strings.Contains(out, "12 apps") || strings.Contains(out, "file") {
		t.Errorf("closing line:\n%s", out)
	}
}
