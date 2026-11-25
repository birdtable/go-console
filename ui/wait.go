package ui

// Progress for work measured in time rather than items: a measurement that runs
// for a window it already knows.
//
// A Bar would not do: it redraws on a clock of its own, eight times a second,
// and a measurement of a machine at rest cannot have a process waking that
// often in it. So a Wait has no clock. It redraws when the command tells it to,
// which is when the command wakes anyway.

import (
	"fmt"
	"time"
)

// A Wait says how far through a known stretch of time a command is.
type Wait struct {
	s       *Session
	label   string
	total   time.Duration
	started time.Time
	now     func() time.Time
	lastLog time.Time
	stopped bool
	timed   bool
	took    time.Duration
}

// waitLogEvery is how often a run with no terminal says it is still going.
// Rarer than a Bar's, because nothing here moves faster than a window.
const waitLogEvery = time.Minute

// NewWait starts reporting on a stretch of total, on the session's screen. It
// draws once now.
func (s *Session) NewWait(label string, total time.Duration) *Wait {
	w := &Wait{s: s, label: label, total: total, now: s.clockFunc()}
	w.started = w.now()
	w.Set("")
	return w
}

// Set redraws the line with what is happening now ("files alone", "settling").
// Safe on a nil Wait, so a command can hold one only when it is drawing.
func (w *Wait) Set(what string) {
	if w == nil || w.stopped {
		return
	}
	now := w.now()
	line := w.label
	if what != "" {
		line += "  " + what
	}
	line += "  " + w.s.Note(w.howFar(now.Sub(w.started)))
	if !w.s.moves() {
		if w.lastLog.IsZero() || now.Sub(w.lastLog) >= waitLogEvery {
			w.lastLog = now
			w.s.Info("%s", line)
		}
		return
	}
	w.s.paint("\r\x1b[2K  " + w.s.Note("…") + " " + line)
}

// howFar is elapsed against the whole, and what is left once that is worth
// saying. Past the end it says so rather than counting down below zero: a
// meter that answers slowly makes a window run long.
func (w *Wait) howFar(elapsed time.Duration) string {
	if w.total <= 0 {
		return short(elapsed)
	}
	if elapsed >= w.total {
		return short(elapsed) + ", finishing"
	}
	s := short(elapsed) + " of " + short(w.total)
	if left := w.total - elapsed; elapsed >= 5*time.Second {
		s += fmt.Sprintf(", about %s left", short(left))
	}
	return s
}

// Stopwatch makes Stop say how long the wait took, in a line of its own. Off
// by default, so a command that never said it keeps its output. Safe on nil.
func (w *Wait) Stopwatch() *Wait {
	if w != nil {
		w.timed = true
	}
	return w
}

// Stop wipes the line, so what prints next starts at the margin, and says how
// long it took when Stopwatch asked for that. It returns the time the wait
// took, 0 on nil. Safe on nil and safe twice.
func (w *Wait) Stop() time.Duration {
	if w == nil {
		return 0
	}
	if w.stopped {
		return w.took
	}
	took := w.now().Sub(w.started)
	w.stopped, w.took = true, took
	if w.s.moves() {
		w.s.paint("\r\x1b[2K")
	}
	if w.timed {
		w.s.Info("%s took %s", w.label, took.Round(time.Second))
	}
	return took
}
