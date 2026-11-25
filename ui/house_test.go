package ui

import (
	"github.com/birdtable/go-run/exit"
)

// The house policy the tests run with: the codes and words and the flag spellings the program that
// first used ui gave, now handed in by the caller as any program's are. The second-consumer test
// (secondconsumer_test.go) runs with values deliberately unlike these.

// kit is a table of the house's kinds, with a method for each, so a test says `houseEnds().Done("...")`.
type kit struct{ exit.Table }

func (k kit) Done(what string) exit.Ending     { return k.End("done", what, "") }
func (k kit) Findings(what string) exit.Ending { return k.End("findings", what, "") }
func (k kit) Failed(what, next string) exit.Ending {
	return k.End("failed", what, next)
}
func (k kit) Busy(what, next string) exit.Ending { return k.End("busy", what, next) }
func (k kit) NotReady(what, next string) exit.Ending {
	return k.End("notready", what, next)
}
func (k kit) Refused(what, next string) exit.Ending { return k.End("refused", what, next) }

// Code and Word are those of the kind with that name.
func (k kit) Code(name string) int    { return k.of(name).Code }
func (k kit) Word(name string) string { return k.of(name).Word }
func (k kit) of(name string) exit.Kind {
	for _, c := range k.Kinds() {
		if c.Name == name {
			return c
		}
	}
	panic("no kind " + name)
}

// houseEnds is the house exit table.
func houseEnds() kit {
	t, err := exit.New(
		exit.Kind{Name: "done", Word: "done", Code: 0},
		exit.Kind{Name: "needsyou", Word: "needs you", Code: 0},
		exit.Kind{Name: "stoppedat", Word: "stopped at", Code: 0},
		exit.Kind{Name: "findings", Word: "findings", Code: 1},
		exit.Kind{Name: "failed", Word: "failed", Code: 1},
		exit.Kind{Name: "usage", Word: "refused", Code: 2},
		exit.Kind{Name: "busy", Word: "busy", Code: 3},
		exit.Kind{Name: "notready", Word: "not ready", Code: 4},
		exit.Kind{Name: "refused", Word: "not allowed", Code: 5},
		exit.Kind{Name: "declined", Word: "declined", Code: 6},
	)
	if err != nil {
		panic(err)
	}
	return kit{t}
}

// houseEndKinds is the names of the house's kinds that ui ends on.
func houseEndKinds() EndKinds {
	return EndKinds{Failed: "failed", Usage: "usage", Declined: "declined", Busy: "busy", NotReady: "notready", Refused: "refused", Roles: houseRoles()}
}

// houseEndings is the session option that gives a session the house's table and kinds.
func houseEndings() SessionOption { return Endings(houseEnds().Table, houseEndKinds()) }

// houseFlags is the house spelling of the flags.
func houseFlags() Flags {
	return Flags{Plain: "plain", Accessible: "accessible", Yes: "yes", JSON: "json", NoColor: "no-color"}
}

// House is the two session options that give a session the house endings and flag names.
func House() []SessionOption {
	return []SessionOption{houseEndings(), FlagNames(houseFlags()), WithMessages(houseMessages())}
}

// houseMessages is the English the first program used, given by the caller as any program's words are.
func houseMessages() Messages {
	return Messages{
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

// houseRoles is the colour the house gives each of its kinds: good for done, amber for the endings that
// wait on somebody or something, red for those that did not get there.
func houseRoles() map[string]EndRole {
	return map[string]EndRole{
		"done": EndGood, "needsyou": EndAttn, "stoppedat": EndAttn, "busy": EndAttn, "notready": EndAttn, "declined": EndAttn,
		"findings": EndBad, "failed": EndBad, "usage": EndBad, "refused": EndBad,
	}
}
