package ui

import (
	"math"
	"testing"
)

// What it costs when this fails: a bar longer than its column pushes the
// figure beside it out of line, or a half-full point reads as empty.
func TestAMeterIsItsShareOfItsWidthToAnEighth(t *testing.T) {
	t.Parallel()
	at := stream{rich: true, interactive: true, width: 80}
	cases := []struct {
		frac  float64
		width int
		want  string
	}{
		{0, 10, ""},
		{1, 4, "████"},
		{0.5, 4, "██"},
		{0.5, 1, "▌"},
		{1.0 / 8, 1, "▏"},
		{2, 3, "███"},
		{-1, 3, ""},
		{math.NaN(), 3, ""},
		{0.5, 0, ""},
	}
	for _, c := range cases {
		if got := meterOn(at, c.frac, c.width); got != c.want {
			t.Errorf("New().Meter(%g, %d) = %q, want %q", c.frac, c.width, got, c.want)
		}
		if got := cells(meterOn(at, c.frac, c.width)); got > max(c.width, 0) {
			t.Errorf("New().Meter(%g, %d) is %d cells wide", c.frac, c.width, got)
		}
	}
}

// What it costs when this fails: a log or a script reading a pipe gets
// block characters where it wanted figures.
func TestAMeterIsDrawnOnlyAtAKnownTerminal(t *testing.T) {
	t.Parallel()
	for _, s := range []stream{{}, {width: 80}} {
		if got := meterOn(s, 1, 4); got != "" || chartedOn(s) {
			t.Errorf("%+v draws %q", s, got)
		}
	}
	for _, s := range []stream{{rich: true}, {interactive: true}} {
		if got := meterOn(s, 1, 4); got != "████" {
			t.Errorf("%+v draws %q", s, got)
		}
	}
}

// COLUMNS fits a table and wrapped text, but it is no terminal: a pipe with
// COLUMNS exported still gets the figures and no bar.
func TestCOLUMNSDoesNotTurnABarOnInAPipe(t *testing.T) {
	t.Parallel()
	scr := screenOpt(false, 80)
	if New(scr).Width() != 80 {
		t.Fatalf("COLUMNS=80 off a terminal is %d wide, so the check below proves nothing", New(scr).Width())
	}
	if got := New(scr).Meter(0.5, 8); got != "" {
		t.Errorf("a pipe with COLUMNS=80 drew a bar: %q", got)
	}
	if New(scr).Charted() {
		t.Error("New(scr).Charted() is true in a pipe with COLUMNS")
	}
}
