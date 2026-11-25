// Command ctxwait runs under ui.Main and a command that blocks on its context. It prints "READY" when the command is
// running, "CANCELLED" when the context is cancelled, and returns the context's error, so ui.Main ends the run on the
// failed ending. A test starts it in a process group of its own, sends it a stop (Ctrl+Break on Windows, SIGINT or
// SIGTERM elsewhere) once READY is seen, and reads CANCELLED and the failed ending's non-zero code back.
//
// After CANCELLED it prints "STOPPED <signal>" when ui.Stopped says a signal ended the context, "STOPPED none" when
// not. Given the word again, it then waits for a line on its standard input, so a test can send a second stop while
// the command still runs, and prints "AGAIN <signal>" with what ui.Stopped says then.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/birdtable/go-run/exit"
	"github.com/spf13/cobra"

	"github.com/birdtable/go-console/internal/english"
	"github.com/birdtable/go-console/ui"
)

func main() {
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
	kinds := ui.EndKinds{
		Failed: "failed", Usage: "usage", Declined: "declined", Busy: "busy", NotReady: "busy", Refused: "usage",
		Roles: map[string]ui.EndRole{"done": ui.EndGood, "busy": ui.EndAttn, "declined": ui.EndAttn, "failed": ui.EndBad, "usage": ui.EndBad},
	}
	opts := []ui.Option{ui.WithSession(ui.Endings(table, kinds), ui.WithMessages(english.Messages()))}
	os.Exit(ui.Main(context.Background(), func(*ui.Session) *cobra.Command {
		root := &cobra.Command{Use: "ctxwait", Short: "waits on its context so a test can cancel it", Args: cobra.ArbitraryArgs, SilenceUsage: true, SilenceErrors: true}
		root.RunE = func(c *cobra.Command, args []string) error {
			fmt.Println("READY")
			<-c.Context().Done()
			fmt.Println("CANCELLED")
			fmt.Println("STOPPED", stopped(c.Context()))
			if len(args) == 1 && args[0] == "again" {
				_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
				fmt.Println("AGAIN", stopped(c.Context()))
			}
			return c.Context().Err()
		}
		return root
	}, ui.Brand{Name: "ctxwait"}, opts...))
}

// stopped is the name of the signal ui.Stopped answers for ctx, or none.
func stopped(ctx context.Context) string {
	if sig, ok := ui.Stopped(ctx); ok {
		return sig.String()
	}
	return "none"
}
