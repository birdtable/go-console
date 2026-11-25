package ui

// A block of text a person copies, such as the config a command prints for them
// to paste: in the theme's code-block ground at a terminal that renders, the
// text as it is anywhere else.

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Code is text as a code block: every line padded to the widest and set in the
// theme's Codeblock ground, with a cell of margin either side. Off a terminal,
// under NO_COLOR, TERM=dumb and --plain, it is text unchanged, so a copy of it
// holds nothing but the text. A trailing newline is dropped at a terminal, and
// a tab counts as no width, so a block with tabs pads short.
func (s *Session) Code(text string) string {
	if !s.outStream().paints() || s.plain.Load() {
		return text
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	wide := 0
	for _, l := range lines {
		wide = max(wide, ansi.StringWidth(l))
	}
	st := lipgloss.NewStyle()
	if ground := s.outStream().colour(nameGround); ground != nil {
		st = st.Background(ground)
	}
	for i, l := range lines {
		pad := strings.Repeat(" ", wide-ansi.StringWidth(l))
		lines[i] = st.Render(" " + l + pad + " ")
	}
	return strings.Join(lines, "\n")
}
