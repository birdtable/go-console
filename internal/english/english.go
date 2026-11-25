// Package english is the English the first program used, as a ui.Messages for tests and examples. It is a fixture,
// not a default: ui has no words of its own. It is internal, so it is no part of the module's API.
package english

import "github.com/birdtable/go-console/ui"

// Messages is a fixture for tests and examples, not a default: ui has no words of its own, and a
// program gives its Messages as it gives its Endings. It is the English the first program used.
func Messages() ui.Messages {
	return ui.Messages{
		PlainHelp:      "no live view or pager, and questions as plain lines",
		AccessibleHelp: "questions asked as plain lines for a screen reader",
		YesHelp:        "answer yes to every question before a change",
		JSONHelp:       "one JSON document on stdout, the rest on stderr",
		NoColorHelp:    "no colour, as NO_COLOR asks",

		Yes: "Yes", No: "No",
		AnsweredYes: "yes", AnsweredNo: "no",
		AnsweredYesBy: "yes, by %s", AnsweredYesAdvance: "yes, in advance", AnsweredYesTyped: "yes, typed %s",
		YesFlagName:     "the program's yes",
		TypeNameTitle:   "Type %s to %s",
		TypeNameAdvance: "Or answer in advance with %s",
		TypeNameExactly: "type %s exactly, or press escape to leave it",
		NobodyToAsk:     "this would %s, and there is no terminal here to ask on",
		CouldNotAsk:     "this would %s, and the question could not be asked (%s)",

		CouldNotAskNothingDone: "%s, so nothing was done",
		FormNoTerminal:         "this needs answers, and there is no terminal here to ask on",
		FormNotAnswered:        "nothing was answered",
		FormCouldNot:           "the questions could not be asked (%s)",

		DryWould: "would %s",

		LiveNothingChanged:    "nothing was changed",
		LiveNotTheName:        "that is not the name, so nothing was changed",
		LiveNothingSelected:   "nothing is selected",
		LiveNothingToStepInto: "nothing to step into",
		LiveNoName:            "this row has no name to type, so nothing was changed",
		LiveBack:              "back from %s",

		HelpArgs: "[topic or command]", HelpShort: "Help about a command, or a lesson on a topic", HelpUnknownTopic: "Unknown help topic %#q",
		NameOnCommandLine: "Or name it on the command line: %s", AnswerWith: "Or answer with %s",
		MenuCouldNotShow: "could not show the menu (%s), so here is the help instead", CouldNotAskFor: "could not ask for %s (%s)", MenuNothingPicked: "nothing was picked",
		InternalError:   "the command stopped on an internal error",
		FollowFollowing: "following", FollowPaused: "paused, end follows", FollowKeys: " · ↑/↓ scroll · ctrl-c leave", PageKeys: "%3.0f%% · ↑/↓ scroll · q leave",
		HelpFlagHelp: "help for %s", HelpCommandArgs: "[command]", HelpAnyShort: "Help about any command", VersionLine: "version %s",
	}
}
