package ui

// The wordmark.
//
// Drawn from a small hand-made font (brand.go) rather than a figlet library, which
// is 400 KB for two lines of text, and none of the stock fonts fits
// inside eighty columns at this many letters.

import "strings"

// Arrive draws the banner as somebody enters the tool, and reports whether
// there is now one on screen.
//
// Off a terminal it draws nothing and says so, so a log file and a scheduled
// run stay clean.
//
// The banner is drawn at most once per run, its session, whoever asks for it
// (Session.shown).
func (s *Session) Arrive(b Brand, version string) bool {
	drew := false
	s.shown.Do(func() {
		if text := banner(s.outStream(), b, version); text != "" {
			s.Log("")
			s.say(text)
			s.Log("")
			drew = true
			s.bannerOnScreen.Store(true)
		}
	})
	return drew || s.bannerOnScreen.Load()
}

// bannerWidth is the widest line of the wordmark with what is beside it,
// without the mark. Measured, not written down: a constant goes stale the day
// the tagline or the build string changes.
func bannerWidth(wm, beside []string) int {
	w := 0
	for i, line := range wm {
		n := cells(line)
		if i < len(beside) && beside[i] != "" {
			n += 3 + cells(beside[i])
		}
		w = max(w, n)
	}
	return w
}

// banner is the wordmark for the terminal st, as Arrive draws it, or "" when
// there is no room or no terminal: this is decoration. No information lives here.
func banner(st stream, b Brand, version string) string {
	if !b.valid() {
		return ""
	}
	wm := wordmarkOf(b.Name)
	if st.ascii {
		wm = asciiForm(wm)
	}
	// A width of 0 is unknown, not narrow, which is what style.go means by it
	// everywhere else.
	beside := b.besides(version)
	full := bannerWidth(wm, beside)
	width := widthOf(st)
	if !st.paints() || (width > 0 && width < full) {
		return ""
	}
	mark, bw := b.markRows(), b.markWidth()
	if bw == 0 || st.ascii || (width > 0 && width < full+bw) {
		mark = nil
	}
	h := max(len(mark), len(wm))
	wmTop, markTop := (h-len(wm))/2, (h-len(mark))/2
	var out []string
	for i := range h {
		var row string
		if j := i - wmTop; j >= 0 && j < len(wm) {
			row = roleAction().on(st, wm[j])
			if beside[j] != "" {
				row += "   " + roleNote().on(st, beside[j])
			}
		}
		k := i - markTop
		hasMark := len(mark) > 0 && k >= 0 && k < len(mark)
		switch {
		case hasMark && row == "":
			row = b.drawMark(st, mark[k])
		case hasMark:
			row = Column(b.drawMark(st, mark[k]), bw) + row
		case row != "" && len(mark) > 0:
			row = strings.Repeat(" ", bw) + row
		}
		out = append(out, row)
	}
	return strings.Join(out, "\n")
}
