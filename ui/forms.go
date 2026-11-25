package ui

// The Charm face of a question: how huh is run, and the one question that is
// not a yes or a no, typing a name to confirm something that cannot be undone.
//
// Every form here is reached only behind a flag that answers it, so a script
// or a UI never meets one.

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// Accessible is whether the session's questions are asked as plain lines:
// --plain, --accessible, or a Terminal that says so (Detect reads ACCESSIBLE and
// TERM=dumb for it).
func (s *Session) Accessible() bool {
	return s.plain.Load() || s.accessible.Load() || s.term.Accessible
}

// The environment goes first: huh's WithProgramOptions replaces the options
// WithInput and WithOutput add, huh's stderr default among them.
func (s *Session) onStreams(f *huh.Form) *huh.Form {
	if s.out == nil {
		f = f.WithProgramOptions(s.teaEnv(), tea.WithOutput(s.errW()))
	} else {
		f = f.WithProgramOptions(s.teaEnv()).WithOutput(s.out)
	}
	if s.in != nil {
		f = f.WithInput(s.in)
	}
	return f
}

// Charm is whether this run is shown the Charm face: a terminal to draw on,
// and nobody who asked for plain text or JSON. Otherwise the plain face, the
// same operation with nothing drawn and nothing asked.
//
// Asked of the real streams, never of a test's stand-in for a person, so a
// test that answers questions is never handed a screen to draw.
func (s *Session) Charm() bool {
	return s.inTerminal() && s.outStream().interactive && s.errStream().interactive && !s.plain.Load() && !s.json.Load()
}

func (s *Session) runField(f huh.Field) error {
	return s.runKept(s.onStreams(huh.NewForm(huh.NewGroup(f))).
		WithShowHelp(false).
		WithTheme(theme(s.outStream())).
		WithAccessible(s.Accessible()))
}

// ConfirmName asks for the name of the thing that is about to go, typed in
// full, and reports whether it was typed. For what cannot be undone, where
// yes or no is too easy to say; flag is what answers it in advance, and --yes
// does not. Off a terminal it returns false without asking, with the
// session's usage refusal naming flag as Confirm does, or nil with no
// Endings, and the caller says what to pass.
func (s *Session) ConfirmName(what, name, flag string) (bool, Ending) {
	if !s.CanAsk() {
		if s.hasEnds {
			return false, s.nobodyToAsk(what, flag)
		}
		return false, nil
	}
	var got string
	err := s.runField(huh.NewInput().
		Title(fmt.Sprintf(s.messages().TypeNameTitle, name, what)).
		Description(fmt.Sprintf(s.messages().TypeNameAdvance, flag)).
		Validate(func(typed string) error {
			if strings.TrimSpace(typed) != name {
				return fmt.Errorf(s.messages().TypeNameExactly, name)
			}
			return nil
		}).
		Value(&got))
	if err != nil {
		if !errors.Is(err, huh.ErrUserAborted) {
			if e := s.couldNotAsk(what, flag, err); e != nil {
				return false, e
			}
		}
		s.answered(what, s.messages().AnsweredNo)
		return false, nil
	}
	s.answered(what, fmt.Sprintf(s.messages().AnsweredYesTyped, name))
	return strings.TrimSpace(got) == name, nil
}

func questionAbove(question, note string) string {
	var lines []string
	for _, l := range []string{question, note} {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

// Ask runs f, a form the program built, in the house theme and on the session's
// streams, plainly when the session is accessible. flag, and any more, are what
// answer the form in advance: where nobody can answer (a pipe, a redirect, CI,
// --json) the form is not run and the ending is the usage refusal, whose line to
// type names them. It is nil when the form was answered. A form backed out of
// ends declined, and one that could not be shown ends the usage refusal too.
func (s *Session) Ask(f *huh.Form, flag string, more ...string) Ending {
	flags := make([]string, 0, 1+len(more))
	for _, fl := range append([]string{flag}, more...) {
		if fl != "" {
			flags = append(flags, flagText(fl))
		}
	}
	refuse := func(why string) Ending {
		next := s.commandLine(strings.Join(flags, " "))
		if len(flags) == 0 {
			next = s.plainLine()
		}
		if s.hasEnds {
			return s.usage(why, next)
		}
		return s.usage(why, next)
	}
	if !s.CanAsk() {
		return refuse(s.messages().FormNoTerminal)
	}
	err := s.runKept(s.onStreams(f).
		WithShowHelp(false).
		WithTheme(theme(s.outStream())).
		WithAccessible(s.Accessible()))
	switch {
	case err == nil:
		return nil
	case errors.Is(err, huh.ErrUserAborted):
		return s.declined(s.messages().FormNotAnswered)
	}
	return refuse(fmt.Sprintf(s.messages().FormCouldNot, err.Error()))
}
