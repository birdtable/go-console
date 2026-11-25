package ui

import (
	"context"

	"github.com/spf13/cobra"
)

// WithSession is how Main makes its session: the options a program gives New, beside the
// terminal Main reads from the process. They come after that terminal, so an On among them wins and the session is
// on the program's own Terminal.
func WithSession(opts ...SessionOption) Option {
	return func(s *settings) { s.session = append(s.session, opts...) }
}

// Main is the whole of a program's main: `os.Exit(ui.Main(ctx, newRoot, brand))`. It
// makes the session on the process's own terminal (Detect, with StandardEnv), hands
// it to newRoot to build the command tree, runs the tree under Execute, and
// returns the exit code the run ended on: the ending's code, or 1 for an error that
// ended on none. Ctrl+C and SIGTERM cancel ctx, so a command that stops with it ends
// the run on the failed ending. Either signal does it on every system, so a test may deliver either (Execute).
// Stopped tells a command that a signal ended its context, and which.
// A program with a Terminal of its own, read from its settings, hands it in with WithSession(On(t)), which wins over
// the one Main reads. WithFinish is called last with the code Main returns.
func Main(ctx context.Context, newRoot func(*Session) *cobra.Command, b Brand, opts ...Option) int {
	var s settings
	for _, o := range opts {
		o(&s)
	}
	code := mainCode(ctx, newRoot, b, s, opts)
	if s.finish != nil {
		s.finish(code)
	}
	return code
}

func mainCode(ctx context.Context, newRoot func(*Session) *cobra.Command, b Brand, s settings, opts []Option) int {
	run := New(append([]SessionOption{On(processTerminal())}, s.session...)...)
	root := newRoot(run)
	if s.menu != nil && len(processArgs()) == 0 {
		return menuMain(ctx, bareRun{run: run, root: root, newRoot: newRoot, brand: b, set: s, opts: opts})
	}
	return codeOf(run, Execute(ctx, root, run, b, opts...))
}
