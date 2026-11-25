package ui

import (
	"fmt"
	"reflect"
	"strings"
)

// Messages is every sentence ui says of its own, in the program's words and language. A field
// whose comment names verbs (%s) is a format: the caller keeps the verbs and their order the
// comment gives, and moves the words around them. There is no default and no English in ui:
// Execute refuses a session made without Messages, and one with a field left empty, and a method
// of a session without them panics, as it does without Endings. A product starts from the English set in
// internal/english/english.go: it is internal, so it cannot be imported, and the product copies the file and
// edits the words into its own.
type Messages struct {
	// Flag help, shown by --help for the shared flags the program named.
	PlainHelp, AccessibleHelp, YesHelp, JSONHelp, NoColorHelp string

	// Yes and No are a question's two answers, on the form and in its echo.
	Yes, No string
	// AnsweredYes and AnsweredNo are the echo of a question's answer.
	AnsweredYes, AnsweredNo string
	// AnsweredYesBy is the echo of a yes the program's yes flag gave (%s: the flag), AnsweredYesAdvance
	// of one that came in advance, AnsweredYesTyped of a name typed (%s: the name).
	AnsweredYesBy, AnsweredYesAdvance, AnsweredYesTyped string
	// YesFlagName names the yes flag in a message when the program gave none.
	YesFlagName string
	// TypeNameTitle is ConfirmName's title (%s: the name, %s: what), TypeNameAdvance its
	// description (%s: the flag), TypeNameExactly the refusal of a wrong name (%s: the name).
	TypeNameTitle, TypeNameAdvance, TypeNameExactly string
	// NobodyToAsk is the refusal of a question with nobody to ask (%s: what would happen).
	NobodyToAsk string
	// CouldNotAsk is the refusal of a question that failed (%s: what would happen, %s: the error),
	// and CouldNotAskNothingDone the same said as a warning, where nothing was done.
	CouldNotAsk, CouldNotAskNothingDone string
	// FormNoTerminal, FormNotAnswered and FormCouldNot are Ask's refusals; FormCouldNot takes the error (%s).
	FormNoTerminal, FormNotAnswered, FormCouldNot string

	// DryWould is a dry run's change line, the verb under it (%s: the verb): "would file".
	DryWould string

	// LiveNothingChanged, LiveNotTheName, LiveNothingSelected, LiveNothingToStepInto and LiveNoName
	// are the notes of a live view; LiveBack is the note after stepping into a row (%s: the row).
	LiveNothingChanged, LiveNotTheName, LiveNothingSelected, LiveNothingToStepInto, LiveNoName, LiveBack string

	// HelpArgs follows `help` in the help command's use line, HelpShort is its one line, and HelpUnknownTopic is the line for a
	// topic it has not (%s: the topic).
	HelpArgs, HelpShort, HelpUnknownTopic string
	// NameOnCommandLine and AnswerWith are the notes under a question that names its flag (%s: the flag).
	NameOnCommandLine, AnswerWith string
	// MenuCouldNotShow is the warning of a menu that failed (%s: the error), CouldNotAskFor of a question
	// (%s: the question, %s: the error), MenuNothingPicked the refusal of a menu left without a pick.
	MenuCouldNotShow, CouldNotAskFor, MenuNothingPicked string
	// InternalError is the sentence of a command that panicked.
	InternalError string
	// HelpFlagHelp is the help flag's description (%s: the command), HelpCommandArgs follows `help` in the plain help
	// command's use line, HelpAnyShort is its one line. VersionLine is what --version says before the version (%s:
	// the version).
	// FollowFollowing and FollowPaused are the state of a follow view, FollowKeys the key help after it, PageKeys the
	// pager's line (%3.0f: the percent read, which the format keeps).
	FollowFollowing, FollowPaused, FollowKeys, PageKeys      string
	HelpFlagHelp, HelpCommandArgs, HelpAnyShort, VersionLine string
}

// WithMessages is the option that gives a session its Messages.
func WithMessages(m Messages) SessionOption {
	return func(s *Session) { s.msgs, s.hasMsgs = m, true }
}

// missing is the name of the first field of m left empty, or "".
func (m Messages) missing() string {
	v := reflect.ValueOf(m)
	for i := range v.NumField() {
		if strings.TrimSpace(v.Field(i).String()) == "" {
			return v.Type().Field(i).Name
		}
	}
	return ""
}

// messages is the session's Messages, or a panic naming what is missing.
func (s *Session) messages() Messages {
	if !s.hasMsgs {
		panic("ui: the session has no Messages: make it with ui.New(ui.WithMessages(m))") //nolint:forbidigo // a programmer error: Execute refuses a session without Messages before any command runs
	}
	return s.msgs
}

// checkMessages is Execute's refusal of a session without Messages, or with one field empty.
func (s *Session) checkMessages() error {
	if !s.hasMsgs {
		return fmt.Errorf("ui: Execute needs the session's messages: make it with ui.New(ui.WithMessages(m))")
	}
	if f := s.msgs.missing(); f != "" {
		return fmt.Errorf("ui: the session's Messages has no %s: every message is the program's to give", f)
	}
	return nil
}
