package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// maxListHeight is the most lines a menu takes, title and note included. A
// list longer than that scrolls.
const maxListHeight = 24

// listSpare is the blank lines kept under a menu's rows.
const listSpare = 2

// fitting is a menu that sizes itself to the lines its rows take at the width
// it is given. A row's blurb wraps on a narrow screen and takes more than one
// line, and a height counted one line a row scrolled the last rows out of view.
// huh learns the width only when the screen's size arrives, after the field is
// built, so the height is worked out then.
type fitting struct {
	huh.Field
	setHeight func(int)
}

// fitList is a menu of choices that shows every row up to maxListHeight, and
// scrolls past it, whatever the width. setHeight is the field's own Height.
//
// wraps is whether huh wraps a row onto the lines below it, as a single pick
// does. A many-of list does not wrap its rows, so a row stays one line and that
// height is right at any width: it is left as it is, because with no height set
// huh sizes that list without its title and note.
func fitList(f huh.Field, choices []Choice, wraps bool, setHeight func(int)) huh.Field {
	setHeight(min(pickLines(choices)+4, maxListHeight))
	if !wraps {
		return f
	}
	return &fitting{Field: f, setHeight: setHeight}
}

// WithWidth gives the field its width and then its height at that width.
func (f *fitting) WithWidth(width int) huh.Field {
	f.Field.WithWidth(width)
	f.setHeight(0)
	f.setHeight(min(lipgloss.Height(f.View())+listSpare, maxListHeight))
	return f
}

// Update keeps the menu a fitting: huh puts back what Update returns, and the
// field's own would end the fitting at the first key.
func (f *fitting) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	_, cmd := f.Field.Update(msg)
	return f, cmd
}
