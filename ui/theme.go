package ui

// The theme the questions are asked in.
//
// Everything this tool prints goes through the four colours in style.go, chosen
// against measured contrast on both grounds. The questions did not: huh ships
// its own palette, so the two moments a person is actually being asked something
// were the two moments that looked like a different program, in colours nothing
// here had checked.

import (
	"image/color"

	"charm.land/fang/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// theme is the theme the stream st is painted in, as huh wants it.
//
// Built per call rather than once, because huh asks for the styles with the
// ground it is rendering against and nothing here should assume that answer is
// the one detect() found. Off a rich terminal it is the unpainted base theme:
// NO_COLOR is honoured in the questions as much as in the output.
func theme(st stream) huh.Theme {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		s := huh.ThemeBase(isDark)
		if st.ascii {
			s.Focused.Base = s.Focused.Base.BorderStyle(lipgloss.ASCIIBorder())
			s.Focused.Card = s.Focused.Base
		}
		if !st.paints() {
			return s
		}
		var (
			action = orNone(st.colour(nameAction))
			good   = orNone(st.colour(nameGood))
			bad    = orNone(st.colour(nameBad))
		)

		s.Focused.Title = s.Focused.Title.Foreground(action).Bold(true)
		s.Focused.NoteTitle = s.Focused.NoteTitle.Foreground(action).Bold(true)
		s.Focused.Base = s.Focused.Base.BorderForeground(action)
		s.Focused.Card = s.Focused.Base
		s.Focused.SelectSelector = s.Focused.SelectSelector.Foreground(action)
		s.Focused.MultiSelectSelector = s.Focused.MultiSelectSelector.Foreground(action)
		s.Focused.SelectedOption = s.Focused.SelectedOption.Foreground(action)
		s.Focused.SelectedPrefix = s.Focused.SelectedPrefix.Foreground(action)

		// Faint, the same as ui.Note, because a blurb beside a row is the thing
		// somebody's eye should skip when they already know what they want.
		s.Focused.Description = s.Focused.Description.Faint(true)
		s.Blurred.Description = s.Blurred.Description.Faint(true)

		// Going ahead is green and stopping is not coloured, so the shape of a
		// confirm matches every other place this tool says a thing happened.
		s.Focused.FocusedButton = s.Focused.FocusedButton.Background(good).Bold(true)
		s.Focused.ErrorIndicator = s.Focused.ErrorIndicator.Foreground(bad)
		s.Focused.ErrorMessage = s.Focused.ErrorMessage.Foreground(bad)

		s.Blurred = s.Focused
		s.Blurred.Base = s.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
		s.Blurred.Card = s.Blurred.Base
		return s
	})
}

// helpColours is the theme of the stream st as fang wants it, for `--help` and for the
// errors fang renders.
//
// It uses this package's own detected ground, not the one fang offers: with
// colour forced into a pipe fang reads the ground as light and detect()
// defaults to dark, which would paint the help in one half of the palette and
// everything under it in the other.
func helpColours(st stream) fang.ColorScheme {
	var (
		action = orNone(st.colour(nameAction))
		good   = orNone(st.colour(nameGood))
		bad    = orNone(st.colour(nameBad))
		attn   = orNone(st.colour(nameAttn))
		base   = color.Color(lipgloss.NoColor{})
		dim    = orNone(st.colour(nameMuted))
		header = color.Color(lipgloss.NoColor{})
	)
	if st.colour(nameBad) != nil {
		header = lipgloss.Color("15")
	}
	return fang.ColorScheme{
		Base:  base,
		Title: action,
		// A command and a flag are the two things on the page somebody is
		// looking for, so they get the colour this tool uses everywhere else
		// for "here is the thing to type".
		Command:        action,
		Program:        action,
		Flag:           good,
		Argument:       base,
		Description:    base,
		Help:           dim,
		Dash:           dim,
		Comment:        dim,
		DimmedArgument: dim,
		FlagDefault:    dim,
		QuotedString:   attn,
		Codeblock:      orNone(st.colour(nameGround)),
		ErrorHeader:    [2]color.Color{header, bad},
		ErrorDetails:   bad,
	}
}

// fang offers its own reading of the ground, which is not taken: the Terminal says it.
func helpScheme(st stream) func(lipgloss.LightDarkFunc) fang.ColorScheme {
	return func(lipgloss.LightDarkFunc) fang.ColorScheme { return helpColours(st) }
}

func orNone(c color.Color) color.Color {
	if c == nil {
		return lipgloss.NoColor{}
	}
	return c
}
