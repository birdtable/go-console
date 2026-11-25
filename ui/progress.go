package ui

// Progress for work measured in items. Two shapes, because the rule in the
// package doc applies here more than anywhere: a terminal gets a line that
// rewrites itself, and a log file gets a fact every so often.

import (
	"fmt"
	"sync"
	"time"

	"github.com/dustin/go-humanize/english"
)

// A Bar reports how far along a long job is. At a terminal it is one line that
// rewrites itself; off one it prints a fact now and then.
type Bar struct {
	s       *Session
	label   string
	noun    string
	nouns   string
	total   int
	started time.Time

	mu        sync.Mutex
	done      int
	lastDraw  time.Time
	lastLog   int
	lastLogAt time.Time
	stopped   bool
	// window is where the rate and the estimate come from: the recent past,
	// not the whole run.
	window []sample
}

type sample struct {
	at   time.Time
	done int
}

// windowFor is how far back the rate looks. Long enough that a single slow
// batch does not make the number jump about, short enough that the estimate
// follows the run rather than remembering the whole of it.
const windowFor = 90 * time.Second

// logEvery and logAfter are how often a run with no terminal says something,
// whichever comes first. Often enough to see it moving in a log somebody opens
// the morning after, rare enough that 165,000 items do not become 165,000
// lines.
const (
	logEvery = 10000
	logAfter = 2 * time.Minute
)

func (s *Session) reporting() (items int, after time.Duration) {
	items, after = logEvery, logAfter
	if s == nil {
		return items, after
	}
	if s.logEvery > 0 {
		items = s.logEvery
	}
	if s.logAfter > 0 {
		after = s.logAfter
	}
	return items, after
}

// drawEvery bounds the redraw rate. The work here is the app reading files, so
// redrawing per item would spend more time painting than measuring.
const drawEvery = 120 * time.Millisecond

// NewBar starts reporting on the session's screen. total may be 0 when it is
// not known, and the bar then reports a count and a rate without a proportion.
func (s *Session) NewBar(label string, total int) *Bar {
	b := &Bar{s: s, label: label, total: total, started: s.clock(), noun: "file", nouns: "files"}
	if s.moves() {
		s.paint("\x1b[?25l") // hide the cursor while the line moves
	}
	b.draw(true)
	return b
}

// paint writes a control sequence to the terminal and discards the error,
// once, with the reason in one place: there is nothing a display can do about
// a screen it cannot write to, and the work it is reporting on carries on
// either way.
func (s *Session) paint(text string) { _, _ = fmt.Fprint(s.stdout(), text) }

// moves is whether a line may be redrawn in place for the session: a terminal
// that renders, and no `--no-color`, whose stripping would leave a redrawn
// line's old text behind.
func (s *Session) moves() bool {
	st := s.outStream()
	return st.paints() && st.interactive && !s.noColor.Load()
}

// Counting names what the bar counts, for its closing line ("12 apps in 40s").
func (b *Bar) Counting(singular, plural string) *Bar {
	b.mu.Lock()
	b.noun, b.nouns = singular, plural
	b.mu.Unlock()
	return b
}

// Add records that n more items are done.
func (b *Bar) Add(n int) {
	b.mu.Lock()
	b.done += n
	b.note(b.s.clock())
	b.mu.Unlock()
	b.draw(false)
}

// Stop clears the line and leaves a single closing fact behind. Blocking, and
// it wipes the line before returning: a stop that raced the next print left
// half a progress line above it.
func (b *Bar) Stop() {
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return
	}
	b.stopped = true
	done, elapsed := b.done, b.s.clock().Sub(b.started)
	b.mu.Unlock()

	if b.s.moves() {
		b.s.paint("\r\x1b[2K\x1b[?25h") // wipe the line, and give the cursor back
	}
	// The whole run here, not the window: this is the closing fact, and what
	// somebody wants at the end is how long the job took on average.
	overall := 0.0
	if elapsed >= time.Second && done > 0 {
		overall = float64(done) / elapsed.Seconds()
	}
	b.s.Info("%s. %s in %s%s", b.label, english.Plural(done, b.noun, b.nouns), short(elapsed), ratePart(b.s.outStream(), overall))
}

func (b *Bar) draw(force bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return
	}
	done, elapsed := b.done, b.s.clock().Sub(b.started)

	if !b.s.moves() {
		items, after := b.s.reporting()
		if force || done-b.lastLog >= items || b.s.clock().Sub(b.lastLogAt) >= after {
			b.lastLog, b.lastLogAt = done, b.s.clock()
			b.s.Info("%s %s", b.label, b.countPart(done, elapsed))
		}
		return
	}
	if !force && b.s.clock().Sub(b.lastDraw) < drawEvery {
		return
	}
	b.lastDraw = b.s.clock()
	b.s.paint("\r\x1b[2K  " + b.s.Note(spinner(b.s.outStream(), elapsed)) + " " + b.label + "  " + b.countPart(done, elapsed))
}

func (b *Bar) countPart(done int, elapsed time.Duration) string {
	per := b.perSecond(elapsed)
	st := b.s.outStream()
	if b.total <= 0 {
		return fmt.Sprintf("%d%s", done, ratePart(st, per))
	}
	return fmt.Sprintf("%d/%d  %s%s%s", done, b.total,
		roleNote().on(st, percent(done, b.total)), ratePart(st, per), remaining(st, done, b.total, per, elapsed))
}

func percent(done, total int) string {
	return fmt.Sprintf("%.0f%%", 100*float64(done)/float64(total))
}

func ratePart(st stream, perSecond float64) string {
	if perSecond <= 0 {
		return ""
	}
	return roleNote().on(st, fmt.Sprintf("  %.0f a second", perSecond))
}

// remaining is the part a person is actually waiting for, and the part most
// likely to be wrong, so it is only offered once there is enough behind it to
// be worth saying.
func remaining(st stream, done, total int, perSecond float64, elapsed time.Duration) string {
	if done < 200 || done >= total || elapsed < 5*time.Second || perSecond <= 0 {
		return ""
	}
	left := time.Duration(float64(total-done)/perSecond) * time.Second
	return roleNote().on(st, "  about "+short(left)+" left")
}

func short(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// spinner is hand-rolled rather than taken from bubbles.
//
// Bubbles' spinner runs inside a Bubble Tea program, and Bubble Tea's renderer
// queries the terminal on startup, DECRQM for synchronised output, the Kitty
// keyboard protocol, with the replies arriving on stdin. A progress display
// has no reader for them, so they land on screen in the middle of the output.
func spinner(st stream, elapsed time.Duration) string {
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	if st.ascii {
		frames = []rune(`|/-\`)
	}
	return string(frames[int(elapsed/(80*time.Millisecond))%len(frames)])
}

// note records where the bar is now and forgets what has fallen out of the
// window. Called with the lock held.
//
// Bounded by time rather than by count, because the caller decides how often it
// adds: `app index` reports every 500 files and `app retag` every one,
// and a fixed-length ring would be four minutes of history for one and four
// seconds for the other.
func (b *Bar) note(now time.Time) {
	b.window = append(b.window, sample{at: now, done: b.done})
	cut := now.Add(-windowFor)
	// Everything older than the cut goes, and at least two samples stay, or
	// there is no pair to measure between. Keeping one straddling sample
	// instead would let a three-hour stall sit in the window for ever, which is
	// the whole defect this replaced.
	drop := 0
	for drop < len(b.window)-2 && b.window[drop].at.Before(cut) {
		drop++
	}
	b.window = b.window[drop:]
}

// perSecond is the rate over the recent window, falling back to the whole run
// until there is enough of a window to mean anything.
//
// Called with the lock held.
func (b *Bar) perSecond(elapsed time.Duration) float64 {
	if len(b.window) >= 2 {
		first, last := b.window[0], b.window[len(b.window)-1]
		if over := last.at.Sub(first.at); over >= time.Second {
			if moved := last.done - first.done; moved > 0 {
				return float64(moved) / over.Seconds()
			}
		}
	}
	if elapsed < time.Second || b.done == 0 {
		return 0
	}
	return float64(b.done) / elapsed.Seconds()
}
