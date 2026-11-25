package ui_test

// A second consumer: a program that is not the house one. It names other flags, has other codes and
// other closing words, and Execute draws its look with none of the house's spelling anywhere in it.
//
// What it costs when this fails: ui has a default of the first program's in it again, and the second
// program's people are told to type a flag it does not have.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-run/exit"
	"github.com/spf13/cobra"

	"github.com/birdtable/go-console/internal/english"
	"github.com/birdtable/go-console/ui"
	"github.com/birdtable/go-console/uitest"
)

func otherEnds(t *testing.T) endKit {
	t.Helper()
	// Three kinds: a program with one closing word for every refusal and one for every failure
	// names the same kind in several fields of EndKinds.
	tab, err := exit.New(
		exit.Kind{Name: "done", Word: "finished", Code: 0},
		exit.Kind{Name: "failed", Word: "broke", Code: 70},
		exit.Kind{Name: "refused", Word: "turned away", Code: 64},
	)
	if err != nil {
		t.Fatal(err)
	}
	return endKit{tab, ui.EndKinds{Failed: "failed", Usage: "refused", Declined: "refused", Busy: "refused", NotReady: "refused", Refused: "refused", Roles: map[string]ui.EndRole{"done": ui.EndGood, "failed": ui.EndBad, "refused": ui.EndBad}}}
}

// otherRunIn runs the second program's tree on args and returns what it drew and the code it ended on.
// otherRunIn runs the second program's tree on args and returns what it drew and the code it ended on,.
func otherRunIn(t *testing.T, args ...string) (out string, code int) {
	t.Helper()
	ends := otherEnds(t)
	var buf bytes.Buffer
	run := ui.New(
		ends.Endings(),
		ui.WithMessages(english.Messages()),
		ui.FlagNames(ui.Flags{Plain: "bare", Yes: "agree", JSON: "machine"}),
		ui.On(ui.Terminal{}), ui.Out(&buf), ui.Err(&buf),
	)
	root := &cobra.Command{Use: "other", Short: "a second program", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(&cobra.Command{Use: "put", Short: "put a thing", RunE: func(*cobra.Command, []string) error {
		run.Change("thing to shelf")
		return run.End(ends.Done("put one thing"))
	}})
	root.AddCommand(&cobra.Command{Use: "look", Short: "look at things", RunE: func(*cobra.Command, []string) error {
		run.Info("two things")
		run.Info(run.Table([]ui.TableColumn{{Title: "name", Priority: 2}, {Title: "state", Priority: 1}}, [][]string{{"a", "up"}, {"b", "down"}}))
		return run.End(ends.Done("looked at 2 things"))
	}})
	root.SetArgs(args)
	_ = ui.Execute(context.Background(), root, run, ui.Brand{Name: "other"})
	return buf.String(), run.ExitCode()
}

func TestSecondConsumerDrawsItsOwnLook(t *testing.T) { //nolint:paralleltest // serial: Execute sets the process's streams.
	var all strings.Builder
	for _, c := range []struct {
		name string
		args []string
		code int
	}{
		{"a run", []string{"look"}, 0},
		{"a change", []string{"put", "a b"}, 0},
		{"a flag it does not name", []string{"look", "--json"}, 64},
	} {
		out, code := otherRunIn(t, c.args...)
		if code != c.code {
			t.Errorf("%s: code %d, want %d:\n%s", c.name, code, c.code, out)
		}
		fmt.Fprintf(&all, "## %s: other %s\n%s\n", c.name, strings.Join(c.args, " "), out)
	}
	got := all.String()
	// What the program typed is echoed by the header and by cobra's own "unknown flag"; neither is ui's word.
	bare := strings.Join(slices.DeleteFunc(strings.Split(got, "\n"), func(l string) bool {
		return strings.HasPrefix(l, "## ") || strings.Contains(l, "unknown flag")
	}), "\n")
	for _, house := range []string{"--yes", "--apply", "--json", "--plain", "--accessible", "--no-color", "done ", "refused ", "findings", "undecided"} {
		if strings.Contains(bare, house) {
			t.Errorf("the second program's output holds the house's %q:\n%s", house, got)
		}
	}
	if !strings.Contains(got, "finished") {
		t.Errorf("the second program's output does not name its closing words:\n%s", got)
	}
	uitest.Golden(t, "secondconsumer", got)
}

// The pager is the caller's: with PAGER unset there is none, and the caller names its fallback.
func TestSecondConsumerPagerHasNoDefault(t *testing.T) {
	t.Parallel()
	if got := ui.PagerFromEnv(nil); got != nil {
		t.Errorf("an unset PAGER gave %q, want none", got)
	}
	if got := ui.PagerFromEnv([]string{"LESS=R"}); got != nil {
		t.Errorf("LESS alone gave %q, want none", got)
	}
	if got := ui.PagerFromEnv([]string{"PAGER=moreish -x"}); len(got) != 2 || got[0] != "moreish" {
		t.Errorf("PAGER=moreish -x gave %q", got)
	}
}

// errShelfFull is an error of the second program's own, which it words and codes itself.
var errShelfFull = errors.New("the shelf is full")

// The second program runs under Main with both hooks: it words its own error and leaves on its
// own code, 73, which is in neither its table nor the house's, lets ui end every other error on its table's
// kinds, and is told the code of each run as Main returns it.
//
// What it costs when this fails: the hooks work only for a program with the house's codes, or a program that
// closes its own run record is given another code than the process left with.
func TestSecondConsumerHandlesItsOwnErrors(t *testing.T) { //nolint:paralleltest // serial: Main sets the process's streams.
	ends := otherEnds(t)
	for _, c := range []struct {
		name string
		args []string
		code int
		want string
		not  string
	}{
		{"its own error", []string{"put"}, 73, "other: the shelf is full\n", "broke"},
		{"another error", []string{"look"}, 70, "broke", "other:"},
		{"a flag it does not have", []string{"put", "--json"}, 64, "turned away", "other:"},
		{"a run that ends itself", []string{"done"}, 0, "finished", "other:"},
	} {
		var buf bytes.Buffer
		var finished []int
		newRoot := func(run *ui.Session) *cobra.Command {
			root := &cobra.Command{Use: "other", SilenceUsage: true, SilenceErrors: true}
			root.AddCommand(
				&cobra.Command{Use: "put", RunE: func(*cobra.Command, []string) error { return errShelfFull }},
				&cobra.Command{Use: "look", RunE: func(*cobra.Command, []string) error { return errors.New("the light is out") }},
				&cobra.Command{Use: "done", RunE: func(*cobra.Command, []string) error { return run.End(ends.Done("put one thing")) }},
			)
			root.SetOut(&buf)
			root.SetErr(&buf)
			root.SetArgs(c.args)
			return root
		}
		code := ui.Main(context.Background(), newRoot, ui.Brand{Name: "other"},
			ui.WithSession(ends.Endings(), ui.WithMessages(english.Messages()), ui.FlagNames(ui.Flags{Plain: "bare"}), ui.On(ui.Terminal{}), ui.Out(&buf), ui.Err(&buf)),
			ui.WithErrorHandler(func(w io.Writer, err error) (int, bool) {
				if !errors.Is(err, errShelfFull) {
					return 0, false
				}
				_, _ = fmt.Fprintln(w, "other:", err)
				return 73, true
			}),
			ui.WithFinish(func(code int) { finished = append(finished, code) }),
		)
		out := buf.String()
		if code != c.code || !slices.Equal(finished, []int{c.code}) || !strings.Contains(out, c.want) || strings.Contains(out, c.not) {
			t.Errorf("%s: Main returned %d, finish got %v, output %q; want %d, %q and no %q", c.name, code, finished, out, c.code, c.want, c.not)
		}
	}
}
