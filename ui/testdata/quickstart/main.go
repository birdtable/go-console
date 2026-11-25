package main

import (
	"context"
	"os"

	"github.com/birdtable/go-run/exit"
	"github.com/spf13/cobra"

	"github.com/birdtable/go-console/ui"
)

func main() {
	table, err := exit.New(
		exit.Kind{Name: "done", Word: "done", Code: 0},
		exit.Kind{Name: "failed", Word: "failed", Code: 1},
		exit.Kind{Name: "usage", Word: "refused", Code: 2},
	)
	if err != nil {
		panic(err)
	}
	kinds := ui.EndKinds{Failed: "failed", Usage: "usage", Refused: "usage", Declined: "failed", Busy: "failed", NotReady: "failed"}
	flags := ui.Flags{Plain: "plain", JSON: "json", Yes: "yes", NoColor: "no-color"}

	newRoot := func(run *ui.Session) *cobra.Command {
		return &cobra.Command{Use: "app", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
			run.Info("two services")
			run.Log(run.Table(
				[]ui.TableColumn{{Title: "name", Priority: 2}, {Title: "state", Priority: 1}},
				[][]string{{"web", "running"}, {"worker", "stopped"}},
			))
			return run.End(table.End("done", "listed 2 services", ""))
		}}
	}
	os.Exit(ui.Main(context.Background(), newRoot, ui.Brand{Name: "app"},
		ui.WithSession(ui.Endings(table, kinds), ui.FlagNames(flags), ui.WithMessages(words()))))
}

// words is every sentence ui says of its own, in the program's language.
func words() ui.Messages {
	return ui.Messages{
		PlainHelp: "no live view or pager", AccessibleHelp: "questions as plain lines", YesHelp: "answer yes to every question",
		JSONHelp: "one JSON document on stdout", NoColorHelp: "no colour",
		Yes: "Yes", No: "No", AnsweredYes: "yes", AnsweredNo: "no", AnsweredYesBy: "yes, by %s",
		AnsweredYesAdvance: "yes, in advance", AnsweredYesTyped: "yes, typed %s", YesFlagName: "yes",
		TypeNameTitle: "Type %s to %s", TypeNameAdvance: "Or answer in advance with %s", TypeNameExactly: "type %s exactly",
		NobodyToAsk: "this would %s, and there is no terminal to ask on", CouldNotAsk: "this would %s, and the question failed (%s)",
		CouldNotAskNothingDone: "%s, so nothing was done", FormNoTerminal: "this needs answers, and there is no terminal",
		FormNotAnswered: "nothing was answered", FormCouldNot: "the questions failed (%s)", DryWould: "would %s",
		LiveNothingChanged: "nothing was changed", LiveNotTheName: "that is not the name", LiveNothingSelected: "nothing is selected",
		LiveNothingToStepInto: "nothing to step into", LiveNoName: "this row has no name", LiveBack: "back from %s",
		HelpArgs: "[topic or command]", HelpShort: "Help about a command", HelpUnknownTopic: "Unknown help topic %#q",
		NameOnCommandLine: "Or name it on the command line: %s", AnswerWith: "Or answer with %s",
		MenuCouldNotShow: "could not show the menu (%s)", CouldNotAskFor: "could not ask for %s (%s)", MenuNothingPicked: "nothing was picked",
		InternalError: "the command stopped on an internal error", FollowFollowing: "following", FollowPaused: "paused",
		FollowKeys: " · ↑/↓ scroll · ctrl-c leave", PageKeys: "%3.0f%% · ↑/↓ scroll · q leave",
		HelpFlagHelp: "help for %s", HelpCommandArgs: "[command]", HelpAnyShort: "Help about any command", VersionLine: "version %s",
	}
}
