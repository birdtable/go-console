package ui

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// codeError is the exit code of a plain function, as the error OwnCode reads it from.
type codeError int

func (c codeError) Error() string { return fmt.Sprintf("exit code %d", int(c)) }
func (c codeError) ExitCode() int { return int(c) }

// MainFunc is the whole of a program's main when the program is one function and has no command tree: `os.Exit(ui.MainFunc("tool", run, brand))`. It runs run under Main, on a root command named name
// with the built-ins off (WithoutBuiltins) and run's own code kept (OwnCode). run gets the context Main gives its
// commands (Ctrl+C and SIGTERM cancel it, and a second signal is Main's too) and the arguments after the program name,
// unparsed: the kit has no flags of its own here, so `--`, `-h` and `--help` reach run as they were typed. What run
// returns is the exit code, held to 0..255; 0 ends the run done. Any other option passes to Main as it passes there.
func MainFunc(name string, run func(ctx context.Context, args []string) int, b Brand, opts ...Option) int {
	return mainFunc(context.Background(), processArgs(), name, run, b, opts...) //nolint:forbidigo // a program's main has no caller to take a context from; Main cancels it on a signal
}

func mainFunc(ctx context.Context, args []string, name string, run func(context.Context, []string) int, b Brand, opts ...Option) int {
	newRoot := func(*Session) *cobra.Command { //nolint:contextcheck // the context reaches run through the command, c.Context()
		root := &cobra.Command{
			Use:                name,
			DisableFlagParsing: true,
			SilenceUsage:       true,
			SilenceErrors:      true,
			Args:               cobra.ArbitraryArgs,
			RunE: func(c *cobra.Command, _ []string) error {
				if code := run(c.Context(), append([]string{}, args...)); code != 0 { // our args: cobra keeps __complete for itself
					return codeError(code)
				}
				return nil
			},
		}
		root.SetArgs([]string{})
		return OwnCode(root)
	}
	return Main(ctx, newRoot, b, append([]Option{WithoutBuiltins()}, opts...)...)
}
