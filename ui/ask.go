package ui

// Asking, and the rule behind all of it: a refusal this tool could resolve
// should be an offer instead. Show what is in the way, say what you would do,
// ask, then do it.

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"charm.land/huh/v2"
)

// CanAsk reports whether a question could be answered here.
//
// Checked before every question, and the reason `uploads reconcile` can run from Task
// Scheduler at all: a prompt on a stream nobody is watching doesn't wait for an
// answer that isn't coming.
//
// Never with JSON: an answer in JSON is for a program, and a program is never
// shown a form.
func (s *Session) CanAsk() bool {
	return s.inTerminal() && s.outStream().interactive && s.errStream().interactive && !s.json.Load()
}

// Confirm asks before something large or hard to undo.
//
// Opens on no, and detail names what stands to change. --yes answers it yes
// without asking. Off a terminal, with no --yes, it refuses rather than guess
// on somebody's behalf: the ending is the session's usage refusal, naming
// flag, for the caller to End on (the session's Endings). A session given no Endings
// warns instead, and the ending is nil.
func (s *Session) Confirm(what, detail, flag string) (bool, Ending) {
	return s.ask(what, detail, flag, false)
}

func (s *Session) ask(what, detail, flag string, openOnYes bool) (bool, Ending) {
	if detail != "" {
		s.Info("%s", detail)
	}
	if s.yes.Load() {
		s.answered(what, fmt.Sprintf(s.messages().AnsweredYesBy, s.yesFlag()))
		return true, nil
	}
	if !s.CanAsk() {
		return false, s.nobodyToAsk(what, flag)
	}

	yes := openOnYes
	err := s.runField(huh.NewConfirm().
		Title(capitalise(what) + "?").
		Affirmative(s.messages().Yes).
		Negative(s.messages().No).
		Value(&yes))
	if err != nil {
		// Escape or Ctrl-C. Not the default: the default is what return gives
		// you, and backing out of a question is a different act.
		if !errors.Is(err, huh.ErrUserAborted) {
			if e := s.couldNotAsk(what, flag, err); e != nil {
				return false, e
			}
		}
		s.answered(what, s.messages().AnsweredNo)
		return false, nil
	}
	s.answered(what, s.yesNo(yes))
	return yes, nil
}

func (s *Session) yesNo(yes bool) string {
	if yes {
		return s.messages().AnsweredYes
	}
	return s.messages().AnsweredNo
}

func (s *Session) nobodyToAsk(what, flag string) Ending {
	if s.hasEnds {
		return s.usage(fmt.Sprintf(s.messages().NobodyToAsk, what),
			s.commandLine(flagText(flag)))
	}
	// Say what would have happened and what to pass, once, without hanging.
	s.warn("warning: ", fmt.Sprintf(s.messages().NobodyToAsk, what), s.answerLine(flag))
	return nil
}

func (s *Session) couldNotAsk(what, flag string, err error) Ending {
	why := fmt.Sprintf(s.messages().CouldNotAsk, what, err.Error())
	if s.hasEnds {
		return s.usage(why, s.answerLine(flag))
	}
	s.warn("warning: ", fmt.Sprintf(s.messages().CouldNotAskNothingDone, why), s.answerLine(flag))
	return nil
}

func (s *Session) answerLine(flag string) string {
	if flag == "" {
		return s.plainLine()
	}
	return s.commandLine(flagText(flag))
}

// commandLine is this command with flag after it: under Execute, the path of
// the command the run reached, the names of its commands as cobra's
// CommandPath gives them; outside it, the program's name (Program). It is never built
// from the words a person typed, so no value given to the command, a token
// as a positional word or after a flag, is printed back, to the screen, the
// run log or a --json document.
func (s *Session) commandLine(flag string) string {
	path := ""
	if p := s.path.Load(); p != nil {
		path = *p
	}
	return lineFor(path, s.program, flag)
}

func lineFor(path, program, flag string) string {
	if path == "" {
		path = program
	}
	if path == "" {
		return flag
	}
	return path + " " + flag
}

// Choice is one row in a menu: what it does, and what to run when it is picked.
type Choice struct {
	Label string
	Blurb string
	Value string
	// Gap puts a blank line under this row, to separate one group of rows from
	// the next.
	//
	// A blank line rather than a heading, because huh's list has no notion of a
	// row you cannot land on: a heading would be selectable, and arrowing onto
	// something that cannot be chosen is worse than no divider at all.
	Gap bool
}

// Pick offers a list and returns the chosen Value. Typing narrows the list to
// the rows that hold what was typed.
//
// Off a terminal it returns false without asking, and the caller prints
// whatever it would have printed before menus existed.
//
// flag is what answers the pick without a terminal ("<id>", "--workspace"),
// shown under the title so a person learns the shorter way: a pick with no
// answer a script can give is not allowed.
func (s *Session) Pick(title string, choices []Choice, flag string) (string, bool) {
	if !s.CanAsk() || len(choices) == 0 {
		return "", false
	}
	var got string
	err := s.runField(pickField(s.outStream(), s.messages(), title, choices, flag, &got))
	if err != nil {
		// Backing out is an answer and says nothing. Anything else is the menu
		// failing, and the caller's fallback is to print the help, so without
		// this the screen goes from a list of things to do straight to a page
		// of usage with no word about why.
		if !errors.Is(err, huh.ErrUserAborted) {
			s.warn("warning: ", fmt.Sprintf(s.messages().MenuCouldNotShow, err.Error()), s.answerLine(flag))
		}
		return "", false
	}
	return got, got != ""
}

// The list filters from the first key, and huh draws its filter prompt where a select's
// title goes, so the title is never on the screen. The question is also the first line of the
// description, which is drawn above the rows whether or not a filter is typed, and which huh's
// accessible mode does not print, so the question is shown once there too.
func pickField(st stream, m Messages, title string, choices []Choice, flag string, got *string) huh.Field {
	sel := huh.NewSelect[string]().
		Title(title).
		Description(questionAbove(title, flagNote(m, flag))).
		Options(pickOptions(st, choices)...).
		Filtering(true).
		Value(got)
	return fitList(sel, choices, true, func(h int) { sel.Height(h) })
}

func flagNote(m Messages, flag string) string {
	switch {
	case flag == "":
		return ""
	case strings.HasPrefix(flag, "<"):
		return fmt.Sprintf(m.NameOnCommandLine, flag)
	default:
		return fmt.Sprintf(m.AnswerWith, flagText(flag))
	}
}

func flagText(flag string) string {
	if flag == "" || strings.HasPrefix(flag, "-") || strings.HasPrefix(flag, "<") {
		return flag
	}
	return "--" + flag
}

func pickOptions(st stream, choices []Choice) []huh.Option[string] {
	widest := 0
	for _, c := range choices {
		widest = max(widest, len([]rune(c.Label)))
	}
	opts := make([]huh.Option[string], 0, len(choices))
	for _, c := range choices {
		label := c.Label
		if c.Blurb != "" {
			// Two past the widest, so the widest still gets its gap from the
			// padding rather than from Column's own safety space.
			label = Column(c.Label, widest+2) + roleNote().on(st, c.Blurb)
		}
		// After the styling, not inside it, or the blank line carries the
		// note's colour and its reset lands on the following row.
		if c.Gap {
			label += "\n"
		}
		opts = append(opts, huh.NewOption(label, c.Value))
	}
	return opts
}

// pickLines is the lines the rows take: every gap is a line on screen that
// len(choices) does not count, and a height short by the number of gaps
// scrolls the last rows out of sight.
func pickLines(choices []Choice) int {
	lines := len(choices)
	for _, c := range choices {
		if c.Gap {
			lines++
		}
	}
	return lines
}

// Assumed records a question that wasn't asked because the answer arrived in
// advance, so a transcript of a `--yes` run reads like one somebody sat through.
func (s *Session) Assumed(what string) { s.answered(what, s.messages().AnsweredYesAdvance) }

// answered echoes the decision into the scrollback. Huh erases its own prompt,
// and the record of what somebody agreed to should outlive it.
func (s *Session) answered(question, answer string) {
	s.Log("%s %s", s.Note(capitalise(question)+"?"), s.Bold(answer))
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return string(unicode.ToUpper(r[0])) + string(r[1:])
}

// AskForText asks for one value and returns what was typed.
//
// check runs as the person types, so a malformed URL is refused at the field
// rather than three steps later by whatever tried to use it. Off a terminal it
// returns false without asking, and the caller says what it would have written.
func (s *Session) AskForText(title, description, initial string, check func(string) error) (string, bool) {
	if !s.CanAsk() {
		return "", false
	}
	got := initial
	err := s.runField(huh.NewInput().
		Title(title).
		Description(description).
		Validate(orNoCheck(check)).
		Value(&got))
	if err != nil {
		if !errors.Is(err, huh.ErrUserAborted) {
			s.warn("warning: ", fmt.Sprintf(s.messages().CouldNotAskFor, title, err.Error()), s.answerLine(""))
		}
		return "", false
	}
	got = strings.TrimSpace(got)
	s.answered(title, pickedOrNot(got, got != ""))
	return got, got != ""
}

func pickedOrNot(got string, ok bool) string {
	if !ok || got == "" {
		return "nothing"
	}
	return got
}

// huh calls a field's validator on every keystroke without checking it for nil,
// so a caller with nothing to check would panic at a terminal and nowhere else:
// the seam the tests drive returns before huh is reached. A field with nothing
// to check is an ordinary thing to want, so it is answered here rather than
// forbidden.
func orNoCheck(check func(string) error) func(string) error {
	if check == nil {
		return func(string) error { return nil }
	}
	return check
}
