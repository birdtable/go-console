package ui

// The bare run's menu, and the subtree that keeps its caller's own exit code.

import (
	"context"
	"errors"

	"github.com/spf13/cobra"
)

// WithMenu is what a bare run does at a terminal: Main shows the banner, asks pick
// for the words of a command line, and runs them on a fresh tree, built again by
// newRoot, so the flags among them are read as if they had been typed. pick is
// the program's own, which draws its rows in the order it likes, with
// Session.Pick; it answers false when nothing was picked, and the run ends
// declined. Off a terminal, and with CI set, a bare run prints help, asks
// nothing, and returns the usage code, 2. The banner is shown in the menu and
// nowhere else, unless the program also asks for WithBanner.
func WithMenu(pick func(*Session) (argv []string, ok bool)) Option {
	return func(s *settings) { s.menu = pick }
}

type bareRun struct {
	run     *Session
	root    *cobra.Command
	newRoot func(*Session) *cobra.Command
	brand   Brand
	set     settings
	opts    []Option
}

func menuMain(ctx context.Context, m bareRun) int {
	if !m.run.inTerminal() {
		m.root.SetArgs([]string{"--help"})
		if err := Execute(ctx, m.root, m.run, m.brand, m.opts...); err != nil {
			return codeOf(m.run, err)
		}
		return m.run.usageCode()
	}
	m.run.Arrive(m.brand, m.set.version)
	argv, ok := m.set.menu(m.run)
	if !ok {
		if err := m.run.End(m.run.declined(m.run.messages().MenuNothingPicked)); err != nil {
			return m.run.failedCode()
		}
		return m.run.ExitCode()
	}
	fresh := m.newRoot(m.run)
	fresh.SetArgs(argv)
	return codeOf(m.run, Execute(ctx, fresh, m.run, m.brand, m.opts...))
}

// codeOf is the code a run ended on: the ending's, the error handler's, 0 too, or 1 for an error that ended on none.
func codeOf(run *Session, err error) int {
	if code := run.ExitCode(); code != 0 || err == nil || run.handled.Load() {
		return code
	}
	return run.failedCode()
}

const annotationOwnCode = "ui.owncode"

// OwnCode marks c, and every command below it, as a subtree that returns its
// caller's own exit code: when a command in it returns an error with an
// ExitCode() int method (as an *exec.ExitError has), Main returns that code and
// paints no ending for it. Any other error in the subtree, a command outside it,
// and a panic end on the failed ending, code 1, as everywhere. The program's own
// map from what it ran to a code stays in the program.
func OwnCode(c *cobra.Command) *cobra.Command {
	if c.Annotations == nil {
		c.Annotations = map[string]string{}
	}
	c.Annotations[annotationOwnCode] = "true"
	return c
}

func ownCodeOf(at *cobra.Command, err error) (int, bool) {
	inside := false
	for c := at; c != nil; c = c.Parent() {
		inside = inside || c.Annotations[annotationOwnCode] == "true"
	}
	var coded interface{ ExitCode() int }
	if !inside || err == nil || !errors.As(err, &coded) {
		return 0, false
	}
	return min(max(coded.ExitCode(), 0), 255), true
}
