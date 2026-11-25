package ui

// The check for a shared flag the program defines itself, and what it leaves of the tree for help to read.

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// twoRowTree is a program with a persistent flag of its own, a command that has no flag of its own and one that has one.
func twoRowTree() *cobra.Command {
	root := &cobra.Command{Use: "app", Short: "a program"}
	// A persistent flag of the program's own, of no shared name: it is what a merge would hand every command.
	root.PersistentFlags().String("config", "", "the settings file")
	root.AddCommand(&cobra.Command{Use: "bare", Short: "a command with no flag of its own", Run: func(*cobra.Command, []string) {}})
	flagged := &cobra.Command{Use: "flagged", Short: "a command with one", Run: func(*cobra.Command, []string) {}}
	flagged.Flags().Bool("loud", false, "make noise")
	root.AddCommand(flagged)
	return root
}

// kitFour is the four flags a program names when it leaves yes to itself.
func kitFour() Flags {
	return Flags{Plain: "plain", Accessible: "accessible", JSON: "json", NoColor: "no-color"}
}

// The help of a program that names the shared flags (FlagNames) is, byte for byte, the help of the same program
// that registers those flags itself: the command column has `[--flags]` on the command with a flag of its own and
// on no other row.
//
// What it costs when this fails: a program that moves its own --json and --plain to the kit's finds every row of
// its command list a mark wider, and each of its help snapshots moved, for a check that was to change nothing.
func TestHelpWithTheKitsFlagsIsTheHelpWithTheProgramsOwn(t *testing.T) {
	t.Parallel()
	msgs := houseMessages()
	kit, err := sharedRun(t, twoRowTree(), New(houseEndings(), WithMessages(msgs), FlagNames(kitFour())), "--help")
	if err != nil {
		t.Fatal(err)
	}
	own := twoRowTree()
	for name, usage := range map[string]string{
		"plain": msgs.PlainHelp, "accessible": msgs.AccessibleHelp, "json": msgs.JSONHelp, "no-color": msgs.NoColorHelp,
	} {
		own.PersistentFlags().Bool(name, false, usage)
	}
	theirs, err := sharedRun(t, own, New(houseEndings(), WithMessages(msgs)), "--help")
	if err != nil {
		t.Fatal(err)
	}
	if kit != theirs {
		t.Errorf("help with the kit's flags differs from help with the program's own:\n%s\nwant:\n%s", kit, theirs)
	}
	for _, flag := range []string{"--plain", "--accessible", "--json", "--no-color"} {
		if !strings.Contains(kit, flag) {
			t.Errorf("help does not list %s:\n%s", flag, kit)
		}
	}
	rows := 0
	for line := range strings.SplitSeq(kit, "\n") {
		switch fields := strings.Fields(line); {
		case len(fields) > 0 && fields[0] == "bare":
			rows++
			if strings.Contains(line, "[--flags]") {
				t.Errorf("the row of a command with no flag of its own is marked: %q", line)
			}
		case len(fields) > 0 && fields[0] == "flagged":
			rows++
			if !strings.Contains(line, "[--flags]") {
				t.Errorf("the row of a command with a flag of its own is not marked: %q", line)
			}
		}
	}
	if rows != 2 {
		t.Fatalf("read %d of the 2 command rows:\n%s", rows, kit)
	}
}

// After Execute with FlagNames a command with no flag of its own still has none: the check reads each command's
// sets and merges nothing into them.
//
// What it costs when this fails: a read-only check changes what the tree says of itself, so help, a man page or
// a program's own walk of its commands sees flags on every command.
func TestTheSharedFlagCheckMergesNothingIntoACommand(t *testing.T) {
	t.Parallel()
	root := twoRowTree()
	if _, err := sharedRun(t, root, New(houseEndings(), WithMessages(houseMessages()), FlagNames(kitFour())), "flagged"); err != nil {
		t.Fatal(err)
	}
	// flagged ran, and cobra merges the flags of a command it runs. bare did not run, so what it holds is the check's doing.
	bare, _, err := root.Find([]string{"bare"})
	if err != nil || bare.Name() != "bare" {
		t.Fatalf("found %q: %v", bare.Name(), err)
	}
	if bare.HasAvailableFlags() {
		t.Error("a command with no flag of its own has flags after Execute with FlagNames")
	}
	for _, name := range []string{"plain", "accessible", "json", "no-color", "config"} {
		if bare.Flags().Lookup(name) != nil {
			t.Errorf("the check merged root's --%s into a command that did not run", name)
		}
		if root.PersistentFlags().Lookup(name) == nil {
			t.Errorf("--%s is not on the root, so this test reads nothing", name)
		}
	}
}

// kitSession is a session that names the four flags, as a program that calls AddFlags makes one.
func kitSession() *Session {
	return New(houseEndings(), WithMessages(houseMessages()), FlagNames(kitFour()))
}

// persistentNames is the names of root's persistent flags, each as often as it is registered.
func persistentNames(root *cobra.Command) []string {
	var names []string
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) { names = append(names, f.Name) })
	slices.Sort(names)
	return names
}

// After AddFlags the tree has the shared flags before Execute runs: a parse of the root's flags sets the session,
// a command below reads them too, and Execute on that tree runs with no refusal and one flag of each name, after
// one call of AddFlags or two.
//
// What it costs when this fails: a program's own read of its tree before Execute, as help printed as a document,
// a tree run after a menu, an early refusal or a test in the process, ends on `unknown flag: --json`; or the
// program that registers them early is refused by Execute for flags the kit gave it.
func TestAddFlagsGivesATreeTheSharedFlagsBeforeExecute(t *testing.T) {
	t.Parallel()
	want := []string{"accessible", "config", "json", "no-color", "plain"}
	for _, calls := range []int{1, 2} {
		root, run := twoRowTree(), kitSession()
		if err := root.ParseFlags([]string{"--json"}); err == nil {
			t.Fatal("--json parsed before AddFlags, so this test reads nothing")
		}
		for range calls {
			run.AddFlags(root)
		}
		if got := persistentNames(root); !slices.Equal(got, want) {
			t.Fatalf("after %d calls of AddFlags the root has %v, want %v", calls, got, want)
		}
		if err := root.ParseFlags([]string{"--json"}); err != nil || !run.JSON() {
			t.Fatalf("after AddFlags, --json parsed with %v and the session's JSON is %v", err, run.JSON())
		}
		// A command below reads them before Execute too, which merges them into its own set.
		bare, _, err := root.Find([]string{"bare"})
		if err != nil {
			t.Fatal(err)
		}
		if err := bare.ParseFlags([]string{"--plain"}); err != nil || !run.plain.Load() {
			t.Fatalf("after AddFlags, --plain parsed on a command below with %v and the session's plain is %v", err, run.plain.Load())
		}
		if bare.Flags().Lookup("plain") == nil {
			t.Fatal("cobra did not merge the flag into the command below, so Execute is not tried on a merged tree")
		}
		out, err := sharedRun(t, root, run, "flagged", "--no-color")
		if err != nil {
			t.Fatalf("Execute refused a tree that AddFlags gave the flags (%d calls): %v", calls, err)
		}
		if !run.noColor.Load() {
			t.Error("--no-color under Execute did not set the session")
		}
		if !strings.Contains(out, `"`) {
			t.Errorf("the session's JSON, set before Execute, printed no document: %q", out)
		}
		// Execute adds cobra's own help flag, and no second flag of a shared name.
		if got, after := persistentNames(root), []string{"accessible", "config", "help", "json", "no-color", "plain"}; !slices.Equal(got, after) {
			t.Errorf("after Execute the root has %v, want %v", got, after)
		}
	}
	// A session that names no flags adds none, and a nil session or tree is left alone.
	bare := twoRowTree()
	New(houseEndings(), WithMessages(houseMessages())).AddFlags(bare)
	(*Session)(nil).AddFlags(bare)
	kitSession().AddFlags(nil)
	if got := persistentNames(bare); !slices.Equal(got, []string{"config"}) {
		t.Errorf("a session with no flag named gave the root %v", got)
	}
}

// Execute keeps a shared flag only when it is this session's own. The program's own flag of a shared name is
// refused whether it was defined before AddFlags or after, on the root or below, and so is a flag another
// session's AddFlags gave.
//
// What it costs when this fails: a program's own --json shadows the kit's and a script gets no document, or a
// flag that sets another session passes for this run's and the run never hears it.
func TestExecuteStillRefusesAFlagThatIsNotTheSessionsOwn(t *testing.T) {
	t.Parallel()
	tried := 0
	for _, c := range []struct {
		name  string
		build func(root *cobra.Command, run *Session)
	}{
		{"the program's own on the root, before AddFlags", func(root *cobra.Command, run *Session) {
			root.PersistentFlags().Bool("json", false, "the program's own")
			run.AddFlags(root)
		}},
		{"the program's own on the root's local set, after AddFlags", func(root *cobra.Command, run *Session) {
			run.AddFlags(root)
			root.Flags().Bool("json", false, "the program's own")
		}},
		{"the program's own on a command below, after AddFlags", func(root *cobra.Command, run *Session) {
			run.AddFlags(root)
			root.Commands()[0].Flags().Bool("json", false, "the program's own")
		}},
		{"the program's own on a command below, with no AddFlags", func(root *cobra.Command, _ *Session) {
			root.Commands()[0].PersistentFlags().Bool("json", false, "the program's own")
		}},
		{"another session's AddFlags", func(root *cobra.Command, _ *Session) {
			kitSession().AddFlags(root)
		}},
	} {
		ran := false
		root, run := twoRowTree(), kitSession()
		for _, sub := range root.Commands() {
			sub.Run = func(*cobra.Command, []string) { ran = true }
		}
		c.build(root, run)
		_, err := sharedRun(t, root, run, "flagged")
		// Another session gave all four, and the refusal names the first of them.
		flag := "--json"
		if c.name == "another session's AddFlags" {
			flag = "--plain"
		}
		if err == nil || !strings.Contains(err.Error(), "defines "+flag+",") || ran {
			t.Errorf("%s: Execute returned %v and the command ran: %v, want a refusal naming %s", c.name, err, ran, flag)
		}
		tried++
	}
	if tried != 5 {
		t.Fatalf("tried %d of 5", tried)
	}
}

// A flag of a shared name that the program defined on pflag's global set is refused as one on its tree is, with
// AddFlags and without: cobra adds that set to the root's persistent flags at its first merge, which is after
// Execute's check and after the session's flag of that name is on the root.
//
// What it costs when this fails: the program's own --json, defined with pflag.Bool, is dropped by cobra in favour
// of the session's with nothing said, so the program's variable stays false and the run answers as the kit's.
func TestExecuteRefusesASharedNameOnPflagsGlobalSet(t *testing.T) { //nolint:paralleltest // serial: it swaps pflag's global set
	// serial: it swaps pflag.CommandLine, which every cobra root in the process reads.
	saved := pflag.CommandLine
	t.Cleanup(func() { pflag.CommandLine = saved })
	tried := 0
	for _, addFirst := range []bool{false, true} {
		pflag.CommandLine = pflag.NewFlagSet("global", pflag.ContinueOnError)
		var own bool
		pflag.CommandLine.BoolVar(&own, "json", false, "the program's own, on the global set")
		ran := false
		root, run := twoRowTree(), kitSession()
		for _, sub := range root.Commands() {
			sub.Run = func(*cobra.Command, []string) { ran = true }
		}
		if addFirst {
			run.AddFlags(root)
		}
		_, err := sharedRun(t, root, run, "flagged", "--json")
		if err == nil || !strings.Contains(err.Error(), "defines --json,") || ran {
			t.Errorf("--json on pflag's global set, AddFlags first %v: Execute returned %v and the command ran: %v, want a refusal naming --json", addFirst, err, ran)
		}
		tried++
	}
	// The negative half: a global flag of another name is the program's to keep, and the run goes on.
	pflag.CommandLine = pflag.NewFlagSet("global", pflag.ContinueOnError)
	var loud bool
	pflag.CommandLine.BoolVar(&loud, "global-loud", false, "the program's own, of no shared name")
	root, run := twoRowTree(), kitSession()
	if _, err := sharedRun(t, root, run, "flagged", "--global-loud", "--json"); err != nil || !loud || !run.JSON() {
		t.Errorf("a global flag of no shared name: Execute returned %v, the program's flag is %v and the session's JSON %v", err, loud, run.JSON())
	}
	// Execute a second time on the same tree with the same session keeps the flags the first left, and runs.
	if _, err := sharedRun(t, root, run, "flagged"); err != nil {
		t.Errorf("a second Execute on the same tree with the same session was refused: %v", err)
	}
	if tried != 2 {
		t.Fatalf("tried %d of 2", tried)
	}
}
