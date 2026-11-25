package ui

// A list: lipgloss's at a terminal that renders, dashes off one.

import (
	"strings"

	"charm.land/lipgloss/v2/list"
)

// List is items one to a line: lipgloss's list with a bullet at a terminal
// that renders, a dash off one.
func (s *Session) List(items ...string) string {
	if len(items) == 0 {
		return ""
	}
	clean := make([]string, len(items))
	for i, it := range items {
		clean[i] = Name(it)
	}
	if !s.outStream().paints() {
		return "- " + strings.Join(clean, "\n- ")
	}
	shown := make([]any, len(clean))
	for i, it := range clean {
		shown[i] = it
	}
	return list.New(shown...).
		Enumerator(list.Bullet).
		EnumeratorStyle(mutedStyle(s.outStream()).PaddingRight(1)).
		String()
}
