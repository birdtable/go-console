package ui

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// withArgs runs f with the process's arguments as args, the program's name first.
func withArgs(t *testing.T, args []string, f func()) {
	t.Helper()
	was := os.Args
	os.Args = append([]string{"app"}, args...)
	defer func() { os.Args = was }()
	f()
}

// codedError is an error that carries an exit code, as an *exec.ExitError does.
type codedError struct{ code int }

func (e codedError) Error() string { return "it exited" }
func (e codedError) ExitCode() int { return e.code }

// A bare run in a pipe prints help, asks nothing and returns 2.
//
// What it costs when this fails: a script that runs the program bare hangs on a menu, or reads
// a page of help as a success.
func TestABareRunInAPipePrintsHelpAndAsksNothing(t *testing.T) { //nolint:paralleltest // serial: it sets the process's arguments and streams.
	asked := false
	menu := WithMenu(func(*Session) ([]string, bool) { asked = true; return nil, false })
	for _, ci := range []string{"", "1"} {
		t.Setenv("CI", ci)
		var code int
		var out string
		withArgs(t, nil, func() {
			code, out, _ = mainRun(t, func(*Session) *cobra.Command {
				root := &cobra.Command{Use: "app", Short: "does a thing"}
				root.AddCommand(&cobra.Command{Use: "go", Short: "goes", Run: func(*cobra.Command, []string) {}})
				return root
			}, menu)
		})
		if code != 2 || asked || !strings.Contains(out, "go") {
			t.Errorf("CI=%q: code %d, asked %v, out %q: want help, nothing asked, 2", ci, code, asked, out)
		}
	}
}

// An OwnCode subtree that returns 75 makes Main return 75 and paints no ending; a command outside
// it that returns the same error ends on the failed ending, code 1.
//
// What it costs when this fails: a hook's own exit code is lost, or a failure outside the subtree
// leaves with a hook's code and no ending.
func TestAnOwnCodeSubtreeReturnsItsCallersCode(t *testing.T) { //nolint:paralleltest // serial: it sets the process's arguments and streams.
	tree := func(*Session) *cobra.Command {
		root := &cobra.Command{Use: "app", SilenceUsage: true, SilenceErrors: true}
		hook := OwnCode(&cobra.Command{Use: "hook"})
		hook.AddCommand(&cobra.Command{Use: "run", RunE: func(*cobra.Command, []string) error { return codedError{75} }})
		hook.AddCommand(&cobra.Command{Use: "bad", RunE: func(*cobra.Command, []string) error { return errors.New("it broke") }})
		root.AddCommand(hook, &cobra.Command{Use: "other", RunE: func(*cobra.Command, []string) error { return codedError{75} }})
		return root
	}
	for _, c := range []struct {
		args       []string
		code       int
		ending     bool
		stderrWord string
	}{
		{[]string{"hook", "run"}, 75, false, ""},
		{[]string{"hook", "bad"}, 1, true, "failed"},
		{[]string{"other"}, 1, true, "failed"},
	} {
		var code int
		var errs string
		withArgs(t, c.args, func() { code, _, errs = mainRun(t, tree) })
		if code != c.code || strings.Contains(errs, "failed") != c.ending {
			t.Errorf("%v: code %d, stderr %q; want %d, an ending %v", c.args, code, errs, c.code, c.ending)
		}
	}
}
