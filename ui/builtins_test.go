package ui

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// builtinsTree is a program that brings its own help func and its own completion command, and one command
// with a completion function of its own.
func builtinsTree() *cobra.Command {
	root := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) {}}
	root.SetHelpFunc(func(c *cobra.Command, _ []string) { fmt.Fprintf(c.OutOrStdout(), "OWN HELP %s\n", c.Name()) })
	root.AddCommand(&cobra.Command{Use: "completion", Run: func(c *cobra.Command, _ []string) { fmt.Fprintln(c.OutOrStdout(), "OWN COMPLETION") }})
	root.AddCommand(&cobra.Command{
		Use: "go", Run: func(*cobra.Command, []string) {},
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return []string{"fromtree"}, cobra.ShellCompDirectiveNoFileComp
		},
	})
	return root
}

func builtinsRun(t *testing.T, opts []Option, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := builtinsTree()
	root.SetOut(&out)
	root.SetArgs(args)
	err := Execute(context.Background(), root, runner(), testBrand(), opts...)
	return out.String(), err
}

// Under WithoutBuiltins a tree with its own help func and completion command keeps them: --help, -h, help and
// a command's --help print the tree's own help, completion and __complete answer from the tree, and man is an
// unknown command. Without the option the same tree is drawn by go-console.
//
// What it costs when this fails: a program that brings its own help and completion gets go-console's drawn over
// it, or a man command it never wrote.
func TestWithoutBuiltinsLeavesHelpCompletionAndManToTheTree(t *testing.T) {
	t.Parallel()
	own := []Option{WithoutBuiltins()}
	for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}, {"go", "--help"}, {"help", "go"}} {
		got, err := builtinsRun(t, own, args...)
		if err != nil || !strings.HasPrefix(got, "OWN HELP ") {
			t.Errorf("%v printed %q (err %v), want the tree's own help", args, got, err)
		}
	}
	if got, err := builtinsRun(t, own, "completion"); err != nil || got != "OWN COMPLETION\n" {
		t.Errorf("completion printed %q (err %v), want the tree's own", got, err)
	}
	if got, err := builtinsRun(t, own, "__complete", "go", ""); err != nil || !strings.Contains(got, "fromtree") {
		t.Errorf("__complete printed %q (err %v), want the tree's own answer", got, err)
	}
	if got, err := builtinsRun(t, own, "man"); err == nil {
		t.Errorf("man printed %q, want an unknown command", got)
	}
	if got, _ := builtinsRun(t, nil, "--help"); strings.Contains(got, "OWN HELP") {
		t.Errorf("without the option --help printed %q, want go-console's help", got)
	}
}

// A tree with no completion command of its own gets none of cobra's under WithoutBuiltins.
//
// What it costs when this fails: a program that brings no completion is given one it never wrote.
func TestWithoutBuiltinsAddsNoCompletionCommand(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	root := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(&cobra.Command{Use: "go", Run: func(*cobra.Command, []string) {}})
	root.SetOut(&out)
	root.SetArgs([]string{"completion", "bash"})
	if err := Execute(context.Background(), root, runner(), testBrand(), WithoutBuiltins()); err == nil {
		t.Errorf("completion printed %q, want an unknown command", out.String())
	}
}

// leaflessRaw is a root with no subcommands that takes every argument raw (DisableFlagParsing), and records
// them in *got.
func leaflessRaw(got *[]string) *cobra.Command {
	return &cobra.Command{
		Use: "app", DisableFlagParsing: true, SilenceUsage: true, SilenceErrors: true,
		Run: func(_ *cobra.Command, args []string) { *got = append([]string(nil), args...) },
	}
}

// A leafless root with DisableFlagParsing, under WithoutBuiltins, receives help, man and --help as its own
// arguments, through Execute and through Main: no help command, no man command and no help flag is added in
// front of it.
//
// What it costs when this fails: a program that forwards its arguments (a wrapper around another tool) has
// help, man or --help taken from it and answered by go-console.
func TestWithoutBuiltinsLeavesALeaflessRawRootItsArguments(t *testing.T) { //nolint:paralleltest // serial: Main sets the process's streams.
	// serial: Main sets the process's streams.
	for _, args := range [][]string{{"help"}, {"man"}, {"--help"}, {"-h"}, {"help", "man", "--help"}} {
		var viaExecute []string
		root := leaflessRaw(&viaExecute)
		root.SetArgs(args)
		var out bytes.Buffer
		root.SetOut(&out)
		if err := Execute(context.Background(), root, runner(), testBrand(), WithoutBuiltins()); err != nil {
			t.Errorf("Execute %v: %v", args, err)
		}
		if !slices.Equal(viaExecute, args) || out.Len() != 0 {
			t.Errorf("Execute %v: the root received %v and %q was printed, want the arguments and no output", args, viaExecute, out.String())
		}

		var viaMain []string
		code, stdout, _ := mainRun(t, func(*Session) *cobra.Command {
			r := leaflessRaw(&viaMain)
			r.SetArgs(args)
			return r
		}, WithoutBuiltins())
		if code != 0 || !slices.Equal(viaMain, args) || stdout != "" {
			t.Errorf("Main %v: code %d, the root received %v, stdout %q, want 0, the arguments and no output", args, code, viaMain, stdout)
		}
	}
}
