// Command ptyprog is the program the pty tests of ui run: a small tree under
// ui.Main, so help and a menu are drawn on the streams the test gives it. With
// PTYPROG=menu a bare run asks a menu first.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/birdtable/go-run/exit"
	"github.com/spf13/cobra"

	"github.com/birdtable/go-console/internal/english"
	"github.com/birdtable/go-console/ui"
)

func main() {
	brand := ui.Brand{Name: "tool", Mark: "◗  ▄▀▀▄\n ▐▄▄▄▄▌", MarkGap: 3, Row: "a made-up tagline"}
	table, err := exit.New(
		exit.Kind{Name: "done", Word: "done", Code: 0},
		exit.Kind{Name: "failed", Word: "failed", Code: 1},
		exit.Kind{Name: "usage", Word: "refused", Code: 2},
		exit.Kind{Name: "busy", Word: "busy", Code: 3},
		exit.Kind{Name: "declined", Word: "declined", Code: 6},
	)
	if err != nil {
		panic(err)
	}
	opts := []ui.Option{ui.WithSession(ui.Endings(table, ui.EndKinds{Failed: "failed", Usage: "usage", Declined: "declined", Busy: "busy", NotReady: "busy", Refused: "usage", Roles: map[string]ui.EndRole{"done": ui.EndGood, "busy": ui.EndAttn, "declined": ui.EndAttn, "failed": ui.EndBad, "usage": ui.EndBad}}), ui.WithMessages(english.Messages()), ui.FlagNames(ui.Flags{Plain: "plain", Accessible: "accessible", Yes: "yes", JSON: "json", NoColor: "no-color"}))}
	if os.Getenv("PTYPROG") == "menu" {
		opts = append(opts, ui.WithMenu(func(run *ui.Session) ([]string, bool) {
			v, ok := run.Pick("What now?", []ui.Choice{
				{Label: "Make noise", Blurb: "run it loudly", Value: "--loud"},
				{Label: "Stay quiet", Blurb: "run it", Value: "--folder=here"},
			}, "--pick")
			return strings.Fields(v), ok
		}))
	}
	if os.Getenv("PTYPROG") == "ask" {
		opts = append(opts, ui.WithMenu(askMenu))
	}
	os.Exit(ui.Main(context.Background(), func(*ui.Session) *cobra.Command {
		root := &cobra.Command{Use: "tool", Short: "does a thing for the folder it is run in, and says what it changed"}
		root.Flags().Bool("loud", false, "make noise about every file the run reads or writes, one line each")
		root.Flags().String("folder", "", "the folder to work in, when it is not the one the run starts in")
		root.Run = func(c *cobra.Command, _ []string) {
			loud, _ := c.Flags().GetBool("loud")
			fmt.Printf("ran loud=%v\n", loud)
		}
		return root
	}, brand, opts...))
}
