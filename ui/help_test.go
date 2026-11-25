package ui

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

// helpTree is a root whose commands and flags have text longer than any line of 40 cells, with a flag
// that must not be cut at its hyphen, a group command with no run of its own and an example.
func helpTree() *cobra.Command {
	root := &cobra.Command{
		Use: "demo", Short: "A root whose subcommand has a long short line, and whose own text is long too",
		Example: "# change one source\n  demo sub --source-file=rules/long-name.yaml --dry-run-first\ndemo sub",
	}
	root.AddCommand(&cobra.Command{Use: "sub", Short: "Record or change a source a rule cites, with a history entry for each change", Run: func(*cobra.Command, []string) {}})
	group := &cobra.Command{Use: "group", Short: "A command whose children are all there is of it, which says so at some length"}
	group.AddCommand(&cobra.Command{Use: "child", Short: "Does the one thing", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(group)
	root.Flags().String("source-file-path", "a.yaml", "the file a source is read from, when it is not the one the run starts in")
	return root
}

func helpAtWidth(t *testing.T, terminal, limit int, args ...string) string {
	t.Helper()
	opts := []SessionOption{On(Terminal{Width: terminal})}
	if limit > 0 {
		opts = append(opts, MaxWidth(limit))
	}
	out, err := sharedRun(t, helpTree(), runner(opts...), args...)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out
}

func widest(s string) (n int) {
	for l := range strings.SplitSeq(s, "\n") {
		n = max(n, ansi.StringWidth(l))
	}
	return n
}

// Help is laid out at the session's width, at most MaxWidth, and no wider than fang's 120, for every way help
// is asked for: --help and -h on the root and below it, the help command, and a command that only has children.
//
// What it costs when this fails: a program whose output is held to 80 columns, or to 96 by its own rule, prints
// help that is 102 or 120 wide, and a person on a narrow screen reads it wrapped by the terminal.
func TestHelpIsLaidOutAtTheSessionsWidth(t *testing.T) {
	t.Parallel()
	asks := [][]string{{"--help"}, {"-h"}, {"sub", "--help"}, {"group", "--help"}, {"group"}, {"help"}, {"help", "sub"}, {"help", "group"}, {"completion", "--help"}, {}}
	for _, c := range []struct{ terminal, limit, want int }{
		{40, 0, 40}, {60, 0, 60}, {80, 0, 80}, {200, 96, 96}, {200, 0, 120}, {0, 0, 120}, {80, 96, 80},
	} {
		for _, args := range asks {
			t.Run(strconv.Itoa(c.terminal)+"-"+strconv.Itoa(c.limit)+"-"+strings.Join(args, "_"), func(t *testing.T) {
				t.Parallel()
				got := helpAtWidth(t, c.terminal, c.limit, args...)
				if !strings.Contains(strings.ToLower(got), "usage") {
					t.Fatalf("no help in %q", got)
				}
				if w := widest(got); w > c.want {
					t.Errorf("help is %d columns wide, over %d:\n%s", w, c.want, got)
				}
			})
		}
	}
}

// The text of a command or flag is broken at spaces and a flag is never split.
func TestHelpBreaksNoFlag(t *testing.T) {
	t.Parallel()
	for _, w := range []int{40, 60, 80} {
		got := helpAtWidth(t, w, 0, "--help")
		for _, want := range []string{"--source-file-path", "--dry-run-first"} {
			// the key of a flag in the flags list, and a flag in an example, stay whole
			if strings.Contains(helpTreeText(), want) && !strings.Contains(got, want) {
				t.Errorf("width %d: %s is not whole in\n%s", w, want, got)
			}
		}
	}
}

func helpTreeText() string { return helpTree().Example + " --source-file-path" }

// Help keeps its layout: the sections fang names, in its order, and the default of a flag.
func TestHelpKeepsItsSections(t *testing.T) {
	t.Parallel()
	got := helpAtWidth(t, 80, 0, "--help")
	last := -1
	for _, s := range []string{"USAGE", "EXAMPLES", "COMMANDS", "FLAGS"} {
		i := strings.Index(got, s)
		if i <= last {
			t.Errorf("%s is not after the section before it in\n%s", s, got)
		}
		last = i
	}
	if !strings.Contains(got, "(a.yaml)") {
		t.Errorf("the flag's default is missing in\n%s", got)
	}
}

// padRoot is the root of the issue's padrepro: a long description, an example with two columns and a
// subcommand.
func padRoot() *cobra.Command {
	root := &cobra.Command{
		Use:     "demo",
		Short:   "A demo",
		Long:    "A demo program whose long description is a sentence long enough to wrap at forty columns.",
		Example: "  demo run      run it\n  demo run --x  run it with x",
		Run:     func(*cobra.Command, []string) {},
	}
	root.AddCommand(&cobra.Command{Use: "run", Short: "Run it", Run: func(*cobra.Command, []string) {}})
	return root
}

// No line of help ends in a space or a tab, at any width: not the padded title (`  USAGE  `), not a
// row the wrap breaks, piped or at a terminal that takes colour.
//
// What it costs when this fails: help that people pipe, diff, grep and paste carries trailing
// whitespace, and a program's goldens grow with it.
func TestNoHelpLineEndsInWhitespace(t *testing.T) {
	t.Parallel()
	for _, cols := range []int{40, 80, 120} {
		for _, colour := range []bool{false, true} {
			on := Detect(os.Stdin, os.Stdout, os.Stderr, []string{"COLUMNS=" + strconv.Itoa(cols)}, EnvRules{})
			on.Width, on.Colour = cols, colour
			out, err := sharedRun(t, padRoot(), runner(On(on)), "--help")
			if err != nil {
				t.Fatalf("%d columns: %v", cols, err)
			}
			n := 0
			for l := range strings.SplitSeq(out, "\n") {
				if t := ansi.Strip(l); t != strings.TrimRight(t, " \t") {
					n++
				}
			}
			if n != 0 || !strings.Contains(out, "forty") {
				t.Errorf("%d columns, colour %v: %d lines end in whitespace, in %q", cols, colour, n, out)
			}
		}
	}
}
