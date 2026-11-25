// Command probe runs one small command tree two ways, through ui.Execute and
// through fang alone, so a person on a real console can see how each paints
// help, an ending, output and a redirected stream. It is a probe,
// written to be deleted, and is in no release archive.
//
// Usage: probe [--real-file] [--no-color] <ui|fang> [args...]
//
// --no-color sets NO_COLOR for the run. --real-file hands fang a writer that
// embeds os.Stdout's *os.File, so colorprofile can read the console through it,
// the shape of the fix ui would need (its own writers hide the file today).
package main

import (
	"context"
	"fmt"
	"os"

	"charm.land/fang/v2"
	"github.com/birdtable/go-console/internal/english"
	"github.com/birdtable/go-console/ui"
	"github.com/birdtable/go-run/exit"
	"github.com/spf13/cobra"
)

// realWriter embeds the real file, so a reader that looks for Fd (colorprofile)
// sees the console through it.
type realWriter struct{ *os.File }

// ending is the closing word, sentence, line to type and exit code a run ends
// on: ui.Execute refuses a session that cannot word a refusal or a failure, so
// the probe gives it both (ui's own closing type is not exported).
func main() {
	args := os.Args[1:]
	realFile, noColor := false, false
	for len(args) > 0 {
		switch args[0] {
		case "--real-file":
			realFile = true
		case "--no-color":
			noColor = true
		default:
			goto run
		}
		args = args[1:]
	}
run:
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: probe [--real-file] [--no-color] <ui|fang> [args...]")
		os.Exit(2)
	}
	mode, rest := args[0], args[1:]
	if noColor {
		if err := os.Setenv("NO_COLOR", "1"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	root := tree()
	root.SetArgs(rest)
	if realFile {
		root.SetOut(realWriter{os.Stdout})
		root.SetErr(realWriter{os.Stderr})
	}
	ctx := context.Background()
	switch mode {
	case "fang":
		if err := fang.Execute(ctx, root); err != nil {
			os.Exit(1)
		}
	case "ui":
		table, err := exit.New(
			exit.Kind{Name: "done", Word: "done", Code: 0},
			exit.Kind{Name: "failed", Word: "failed", Code: 1},
			exit.Kind{Name: "usage", Word: "refused", Code: 2},
			exit.Kind{Name: "busy", Word: "busy", Code: 3},
			exit.Kind{Name: "declined", Word: "declined", Code: 6},
		)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		run := ui.New(
			ui.Endings(table, ui.EndKinds{Failed: "failed", Usage: "usage", Declined: "declined", Busy: "busy", NotReady: "busy", Refused: "usage", Roles: map[string]ui.EndRole{"done": ui.EndGood, "busy": ui.EndAttn, "declined": ui.EndAttn, "failed": ui.EndBad, "usage": ui.EndBad}}),
			ui.WithMessages(english.Messages()),
			ui.FlagNames(ui.Flags{Plain: "plain", Accessible: "accessible", Yes: "yes", JSON: "json", NoColor: "no-color"}),
		)
		err = ui.Execute(ctx, root, run, ui.Brand{Name: "probe"})
		// Exit with the run's own ending code (2 for a refusal, 1 for a
		// failure), so the probe shows what ui decided, not a flat 1.
		if e := run.Ended(); e != nil {
			os.Exit(e.Code())
		}
		if err != nil {
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "probe: first word is ui or fang, not %q\n", mode)
		os.Exit(2)
	}
}

func tree() *cobra.Command {
	root := &cobra.Command{Use: "probe", Short: "a console probe", SilenceUsage: true}
	root.AddCommand(&cobra.Command{
		Use:   "go",
		Short: "print one line",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("go: the probe ran")
			return nil
		},
	})
	return root
}
