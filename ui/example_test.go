package ui_test

import (
	"context"
	"fmt"

	"github.com/birdtable/go-run/exit"
	"github.com/spf13/cobra"

	"github.com/birdtable/go-console/internal/english"
	"github.com/birdtable/go-console/ui"
)

// endKit is a program's table of endings with the names ui ends on, and a method for the kinds the
// tests end on.
type endKit struct {
	exit.Table
	kinds ui.EndKinds
}

// Endings is the session option that gives a session the table and the names.
func (k endKit) Endings() ui.SessionOption    { return ui.Endings(k.Table, k.kinds) }
func (k endKit) Done(what string) exit.Ending { return k.End("done", what, "") }
func (k endKit) Failed(what, next string) exit.Ending {
	return k.End("failed", what, next)
}

// ends and flags are a program's own codes, words and flag spellings, handed to ui and never defaulted by it.
func ends() endKit {
	t, err := exit.New(
		exit.Kind{Name: "done", Word: "done", Code: 0},
		exit.Kind{Name: "failed", Word: "failed", Code: 1},
		exit.Kind{Name: "usage", Word: "refused", Code: 2},
		exit.Kind{Name: "busy", Word: "busy", Code: 3},
		exit.Kind{Name: "declined", Word: "declined", Code: 6},
	)
	if err != nil {
		panic(err)
	}
	return endKit{t, ui.EndKinds{Failed: "failed", Usage: "usage", Declined: "declined", Busy: "busy", NotReady: "busy", Refused: "usage", Roles: map[string]ui.EndRole{"done": ui.EndGood, "busy": ui.EndAttn, "declined": ui.EndAttn, "failed": ui.EndBad, "usage": ui.EndBad}}}
}

// messages is a program's own words for what ui says, handed to ui and never defaulted by it.
func messages() ui.Messages { return english.Messages() }

func flags() ui.Flags { return ui.Flags{Plain: "plain", JSON: "json", Yes: "yes"} }

// A program's main is one line, os.Exit(ui.Main(ctx, newRoot, brand)): Main makes the session on
// the process's terminal, runs the tree and returns the ending's code.
func ExampleMain() {
	newRoot := func(run *ui.Session) *cobra.Command {
		root := &cobra.Command{Use: "app", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
			run.Info("hello")
			return run.End(ends().Done("said hello"))
		}}
		root.SetArgs([]string{})
		return root
	}
	code := ui.Main(context.Background(), newRoot, ui.Brand{Name: "app"}, ui.WithSession(ends().Endings(), ui.WithMessages(messages()), ui.FlagNames(flags())))
	fmt.Println("exit", code)
	// Output:
	// hello
	//
	//   done       said hello
	// exit 0
}

// A command prints a line and a table through its session and ends the run through End. At a
// terminal the table is drawn and painted; here, in a pipe, it is plain columns.
func ExampleSession_Table() {
	newRoot := func(run *ui.Session) *cobra.Command {
		root := &cobra.Command{Use: "app", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
			run.Info("two services")
			fmt.Println(run.Table(
				[]ui.TableColumn{{Title: "name", Priority: 2}, {Title: "state", Priority: 1}},
				[][]string{{"web", "running"}, {"worker", "stopped"}},
			))
			return run.End(ends().Done("listed 2 services"))
		}}
		root.SetArgs([]string{})
		return root
	}
	code := ui.Main(context.Background(), newRoot, ui.Brand{Name: "app"}, ui.WithSession(ends().Endings(), ui.WithMessages(messages()), ui.FlagNames(flags())))
	fmt.Println("exit", code)
	// Output:
	// two services
	// name    state
	// web     running
	// worker  stopped
	//
	//   done       listed 2 services
	// exit 0
}
