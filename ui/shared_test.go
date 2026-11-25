package ui

// The flags Execute gives every program, the answer it prints under --json,
// and a question with nobody to ask it of.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
	"github.com/spf13/cobra"
)

// runner is a session as a program under Execute makes one: with the house endings and flag
// names (House), and opts.
func runner(opts ...SessionOption) *Session {
	return New(append(House(), opts...)...)
}

// sharedRun runs root under Execute with args and the session run, off a
// terminal, and returns what reached stdout and what Execute returned.
func sharedRun(t *testing.T, root *cobra.Command, run *Session, args ...string) (string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	err := Execute(context.Background(), root, run, testBrand())
	return out.String(), err
}

// A program that defines one of the five shared flags itself, on its root or
// on any command below, persistent or not, is refused before anything runs,
// and the refusal names the flag and the command.
//
// What it costs when this fails: two programs spell --json or --yes apart, or
// one program's own --json shadows the shared one, so a script that passes it
// gets that program's idea of it and no answer document.
func TestExecuteRefusesAProgramThatDefinesASharedFlagOfItsOwn(t *testing.T) {
	t.Parallel()
	// serial: it sets the process's streams.
	n := 0
	for _, f := range sharedFlagsOf(runner()) {
		name := f.name
		// A command below first: without the refusal its own flag shadows the
		// shared one, and nothing else says so; pflag panics only on the root's.
		for _, where := range []string{"a command below", "root, local", "root, persistent"} {
			ran := false
			root := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) { ran = true }}
			sub := &cobra.Command{Use: "go", Run: func(*cobra.Command, []string) { ran = true }}
			root.AddCommand(sub)
			switch where {
			case "root, persistent":
				root.PersistentFlags().Bool(name, false, "the program's own")
			case "root, local":
				root.Flags().Bool(name, false, "the program's own")
			default:
				sub.Flags().Bool(name, false, "the program's own")
			}
			_, err := sharedRun(t, root, runner(), "go")
			if err == nil || !strings.Contains(err.Error(), "--"+name) || ran {
				t.Errorf("--%s defined on %s: Execute returned %v and the command ran: %v, want a refusal naming --%s", name, where, err, ran, name)
			}
			n++
		}
	}
	if n != 15 {
		t.Fatalf("tried %d definitions, want 15", n)
	}
	// The negative half: a program with flags of other names runs.
	ran := false
	root := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) { ran = true }}
	root.Flags().Bool("loud", false, "make noise")
	root.PersistentFlags().Bool("json-out", false, "not the shared one")
	if _, err := sharedRun(t, root, runner()); err != nil || !ran {
		t.Errorf("a program whose flags share no name was refused (%v) or did not run (%v)", err, ran)
	}
	if _, err := sharedRun(t, &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) {}}, nil); err == nil {
		t.Error("Execute ran with no session for the flags to set")
	}
}

// Each shared flag sets what it names in the run's session, on every
// command. A run without them sets none.
//
// What it costs when this fails: a person passes --plain or --yes and gets
// the live view or a question anyway, or a script passes --json and gets
// prose.
func TestTheSharedFlagsSetTheSession(t *testing.T) { //nolint:paralleltest // serial: it sets the process's environment or directory
	// serial: it sets the process's streams and environment.
	type seen struct{ plain, accessible, yes, json, noColor bool }
	n := 0
	for _, c := range []struct {
		args []string
		want seen
	}{
		{[]string{"go"}, seen{}},
		{[]string{"go", "--plain"}, seen{plain: true}},
		{[]string{"go", "--accessible"}, seen{accessible: true}},
		{[]string{"go", "--yes"}, seen{yes: true}},
		{[]string{"go", "--json"}, seen{json: true}},
		{[]string{"go", "--no-color"}, seen{noColor: true}},
		{[]string{"--plain=false", "go", "--yes=true"}, seen{yes: true}},
	} {
		run := runner()
		var got seen
		root := &cobra.Command{Use: "app"}
		root.AddCommand(&cobra.Command{Use: "go", Run: func(*cobra.Command, []string) {
			got = seen{run.plain.Load(), run.accessible.Load(), run.yes.Load(), run.JSON(), run.noColor.Load()}
			run.MarkPrinted()
		}})
		t.Setenv("NO_COLOR", "")
		t.Setenv("TERM", "xterm-256color")
		if _, err := sharedRun(t, root, run, c.args...); err != nil {
			t.Fatalf("%v: %v", c.args, err)
		}
		if got != c.want {
			t.Errorf("%v set %+v, want %+v", c.args, got, c.want)
		}
		n++
	}
	if n != 7 {
		t.Fatalf("ran %d command lines, want 7", n)
	}
}

// Under --json, a command that ended its run and printed no document of its
// own answers with one: version 3, its ending, its changes, that it was a
// dry run (none of these applied), and the error when it failed. A command that printed its own, and --version, add none.
//
// What it costs when this fails: a script that reads one program's answer
// cannot read another's, a run under --json prints nothing a script can
// read, or a program's own document is followed by a second one and neither
// parses.
func TestJSONAnswersWithOneVersionedDocument(t *testing.T) {
	t.Parallel()
	// serial: it sets the process's streams.
	n := 0
	for _, c := range []struct {
		name string
		cmd  func(run *Session) error
		want Result
	}{
		{"an ending and a change", func(run *Session) error {
			run.Change("write /etc/app.conf")
			run.End(needsYou())
			return nil
		}, Result{Version: 3, Command: "app", DryRun: true, Word: "needs you", What: "updates are waiting", Next: "app updates", Code: 0, Changes: []string{"write /etc/app.conf"}}},
		{"a refusal", func(run *Session) error {
			run.End(usage())
			return nil
		}, Result{Version: 3, Command: "app", DryRun: true, Word: "refused", What: "say which one", Next: "app run --name x", Code: 2, Changes: []string{}}},
		{"a failure the command returned", func(*Session) error {
			return errors.New("the disk is full")
		}, Result{Version: 3, Command: "app", DryRun: true, Word: "failed", What: "the disk is full", Next: "app --help", Code: 1, Changes: []string{}, Error: &Fault{Kind: "failed", Msg: "the disk is full", Hint: "app --help"}}},
		{"a failure the command ended on and returned", func(run *Session) error {
			run.End(failed())
			return errors.New("no network")
		}, Result{Version: 3, Command: "app", DryRun: true, Word: "failed", What: "the step did not work", Next: "app logs", Code: 1, Changes: []string{}, Error: &Fault{Kind: "failed", Msg: "no network", Hint: "app logs"}}},
	} {
		run := runner()
		root := &cobra.Command{Use: "app", SilenceErrors: true, RunE: func(*cobra.Command, []string) error { return c.cmd(run) }}
		out, _ := sharedRun(t, root, run, "--json")
		var got Result
		dec := json.NewDecoder(strings.NewReader(out))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&got); err != nil {
			t.Errorf("%s: stdout is not one answer (%v): %q", c.name, err, out)
			continue
		}
		if dec.More() {
			t.Errorf("%s: stdout holds more than one document: %q", c.name, out)
		}
		if !resultsEqual(got, c.want) {
			t.Errorf("%s: answered %+v, want %+v", c.name, got, c.want)
		}
		n++
	}
	if n != 4 {
		t.Fatalf("decoded %d answers, want 4", n)
	}
	// The negative halves: a command that printed its own document, a run
	// without --json, --version and help print no answer.
	mine := runner()
	own := &cobra.Command{Use: "app", Run: func(c *cobra.Command, _ []string) {
		_, _ = c.OutOrStdout().Write([]byte(`{"mine":true}` + "\n"))
		mine.MarkPrinted()
	}}
	if out, err := sharedRun(t, own, mine, "--json"); err != nil || out != `{"mine":true}`+"\n" {
		t.Errorf("a command that printed its own document got %q (%v)", out, err)
	}
	quiet := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) {}}
	if out, err := sharedRun(t, quiet, runner()); err != nil || out != "" {
		t.Errorf("a run without --json printed %q (%v)", out, err)
	}
	// Help, whichever command it is for and however it is asked for, and
	// --version, print what they print and no answer: no command ran.
	shown := 0
	for _, args := range [][]string{
		{"--version", "--json"},
		{"--help", "--json"},
		{"-h", "--json"},
		{"go", "--help", "--json"},
		{"go", "-h", "--json"},
		{"help", "go", "--json"},
		{"help", "--json"},
	} {
		if out, err := sharedRun(t, rootAndGo(&cobra.Command{Use: "app", Version: "v1", Run: func(*cobra.Command, []string) {}}), runner(), args...); err != nil || out == "" || strings.Contains(out, `"version":`) {
			t.Errorf("%v printed an answer after what it shows, or nothing: %q (%v)", args, out, err)
		}
		shown++
	}
	if shown != 7 {
		t.Fatalf("ran %d command lines, want 7", shown)
	}
	// A command with no run of its own shows its help, and ran nothing.
	if out, err := sharedRun(t, rootAndGo(&cobra.Command{Use: "app"}), runner(), "--json"); err != nil || out == "" || strings.Contains(out, `"version":`) {
		t.Errorf("a root with no run of its own printed an answer after its help, or nothing: %q (%v)", out, err)
	}
	// The positive halves: the command below ran, or the command line was
	// refused before any could, and each answers.
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"go", "--json"}, `"code": 0`},
		{[]string{"go", "--json", "--nope"}, `"msg": "unknown flag: --nope"`},
	} {
		if out, _ := sharedRun(t, rootAndGo(&cobra.Command{Use: "app"}), runner(), c.args...); !strings.Contains(out, `"version": 3`) || !strings.Contains(out, c.want) {
			t.Errorf("%v answered %q, want an answer with %s", c.args, out, c.want)
		}
		shown++
	}
	if shown != 9 {
		t.Fatalf("ran %d command lines, want 9", shown)
	}
}

// rootAndGo is root with one command below it, go, that runs and prints
// nothing.
func rootAndGo(root *cobra.Command) *cobra.Command {
	root.AddCommand(&cobra.Command{Use: "go", Run: func(*cobra.Command, []string) {}})
	return root
}

// resultsEqual compares two answers field by field.
func resultsEqual(a, b Result) bool {
	return a.Version == b.Version && a.Word == b.Word && a.What == b.What && a.Next == b.Next &&
		a.Code == b.Code && a.Command == b.Command && faultsEqual(a.Error, b.Error) && strings.Join(a.Changes, "\n") == strings.Join(b.Changes, "\n") &&
		(a.Changes == nil) == (b.Changes == nil)
}

// faultsEqual compares two errors of an answer: both none, or the same three fields.
func faultsEqual(a, b *Fault) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// A question with no terminal to ask it on, in a session given UsageWith,
// asks nothing and ends on the usage refusal: code 2, saying there is no
// terminal, with the command and the flag that answers it as the line to
// type. --yes answers Confirm yes, and never ConfirmName, which only its own
// flag answers.
//
// What it costs when this fails: a run from a scheduler waits on a prompt
// nobody sees, ends as if it had been told no and exits 0, or tells the
// reader to do something they cannot; or --yes deletes what only a typed
// name should.
func TestAQuestionWithNobodyToAskEndsOnTheUsageRefusalNamingItsFlag(t *testing.T) { //nolint:paralleltest // serial: it sets the process's streams or arguments
	// serial: it sets the process's streams and reads its stderr.
	if New().CanAsk() {
		t.Fatal("a stream with no terminal reported that it could ask a question")
	}
	wasArgs := os.Args
	t.Cleanup(func() { os.Args = wasArgs })
	os.Args = []string{"/usr/local/bin/app", "disk", "format", "--token", "s3cret", "sdb"}
	run := runner()
	path := "app disk format" // as Execute records it on reaching the command
	run.path.Store(&path)
	n := 0
	for _, c := range []struct {
		name string
		ask  func() (bool, Ending)
		flag string
	}{
		{"Confirm", func() (bool, Ending) { return run.Confirm("format /dev/sdb", "", "--yes") }, "--yes"},
		{"ConfirmName", func() (bool, Ending) { return run.ConfirmName("remove box:home", "box:home", "confirm") }, "--confirm"},
	} {
		var yes bool
		var e Ending
		warned := capture(t, &os.Stderr, func() { yes, e = c.ask() })
		if yes || e == nil {
			t.Fatalf("%s: answered %v and ended on %v, want no and the usage refusal", c.name, yes, e)
		}
		if e.Code() != 2 || !strings.Contains(e.What(), "no terminal") || e.Next() != "app disk format "+c.flag {
			t.Errorf("%s: ended on %q, %q, code %d", c.name, e.What(), e.Next(), e.Code())
		}
		if strings.Contains(e.Next(), "s3cret") || strings.Contains(e.Next(), "sdb") || warned != "" {
			t.Errorf("%s: the line to type holds a word typed after the command (%q), or it warned as well: %q", c.name, e.Next(), warned)
		}
		n++
	}
	if n != 2 {
		t.Fatalf("asked %d questions, want 2", n)
	}
	run.yes.Store(true)
	if yes, e := run.Confirm("format /dev/sdb", "", "--yes"); !yes || e != nil {
		t.Errorf("under --yes Confirm answered %v and ended on %v, want yes and nothing", yes, e)
	}
	if yes, e := run.ConfirmName("remove box:home", "box:home", "--confirm"); yes || e == nil || e.Code() != 2 {
		t.Errorf("under --yes ConfirmName answered %v and ended on %v, want no and the usage refusal", yes, e)
	}
}

// End to end: a command whose question has nobody to ask ends on the usage
// refusal, which its answer under --json carries with code 2; with --yes the
// same command goes on and is done.
//
// What it costs when this fails: a scheduled run that could not ask reports
// success to the script that started it, or --yes on the command line does
// not reach the question.
func TestACommandWhoseQuestionHasNobodyToAskExitsWithTheUsageCode(t *testing.T) {
	t.Parallel()
	// serial: it sets JSON, the run's output and the process's streams.
	n := 0
	for _, c := range []struct {
		args []string
		code int
		word string
	}{
		{[]string{"--json"}, 2, "refused"},
		{[]string{"--json", "--yes"}, 0, "done"},
	} {
		run := runner()
		code := -1
		root := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) {
			yes, e := run.Confirm("format /dev/sdb", "", "--yes")
			switch {
			case e != nil:
				_ = run.End(e)
				code = run.ExitCode()
			case yes:
				_ = run.End(done())
				code = run.ExitCode()
			}
		}}
		out, err := sharedRun(t, root, run, c.args...)
		var got Result
		if jerr := json.Unmarshal([]byte(out), &got); err != nil || jerr != nil {
			t.Fatalf("%v: Execute returned %v and stdout %q (%v)", c.args, err, out, jerr)
		}
		if code != c.code || got.Code != c.code || got.Word != c.word {
			t.Errorf("%v: exited %d, answered %q with %d, want %q and %d", c.args, code, got.Word, got.Code, c.word, c.code)
		}
		n++
	}
	if n != 2 {
		t.Fatalf("ran %d command lines, want 2", n)
	}
}

// The line to type is the command's path with the flag after it, or, with
// no path, the program's name: no word a person typed after the program's
// name is in it, a positional word or a flag's value.
//
// What it costs when this fails: a refusal or a failure prints back a token
// passed on the command line, into a log a scheduler keeps and the --json
// document a script reads.
func TestTheLineToTypeIsTheCommandsPathAndNoWordTyped(t *testing.T) {
	t.Parallel()
	n := 0
	for _, c := range []struct {
		path    string
		program string
		want    string
	}{
		{"", "", "--yes"},
		{"", "app", "app --yes"},
		{"app disk format", "app", "app disk format --yes"},
		{"app push", "", "app push --yes"},
	} {
		if got := lineFor(c.path, c.program, "--yes"); got != c.want {
			t.Errorf("program %q at %q: the line is %q, want %q", c.program, c.path, got, c.want)
		}
		n++
	}
	if n != 4 {
		t.Fatalf("read %d command lines, want 4", n)
	}
}

// One refusal's text and JSON forms are each a golden, and the JSON one says
// the same words as the text: the word, the sentence and the line to type. The text form is the plain refusal frame (plain-refusal).
//
// What it costs when this fails: a script under --json reads other words
// than a person reads at the terminal, or the refusal's look moves and no
// reviewer sees it.
func TestARefusalSaysTheSameWordsInTextAndInJSON(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it sets the process's streams.
	root := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) {}}
	run := runner()
	root.Run = func(*cobra.Command, []string) { run.End(refusalFrame()) }
	out, err := sharedRun(t, root, run, "--json")
	if err != nil {
		t.Fatal(err)
	}
	uitest.Golden(t, "refusal-json", out)
	var got Result
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("the answer is not JSON (%v): %q", err, out)
	}
	text := printed(t, 80, false, func(scr SessionOption) { New(scr).End(refusalFrame()) })
	n := 0
	for _, word := range []string{got.Word, got.What, got.Next} {
		if word == "" || !strings.Contains(text, word) {
			t.Errorf("the JSON says %q, which the text %q does not", word, text)
		}
		n++
	}
	if n != 3 || got.Code != refusalFrame().code {
		t.Fatalf("compared %d words and code %d, want 3 and %d", n, got.Code, refusalFrame().code)
	}
}

// The line that answers a question without asking it, for a flag and for
// none (--plain), is the command's path, under Execute, or the program's
// name, with the flag: neither a positional word nor a flag's value typed
// after the command is in it.
//
// What it costs when this fails: a question that could not be asked in
// `app unlock <token>` prints the token back in its refusal or its warning,
// on stderr, in the run log and in the --json Result.
func TestTheLineThatAnswersAQuestionHoldsNoWordTyped(t *testing.T) {
	t.Parallel()
	path := "app unlock"
	under := runner(Program("app"))
	under.path.Store(&path)
	n := 0
	for _, c := range []struct {
		s    *Session
		flag string
		want string
	}{
		{under, "yes", "app unlock --yes"},
		{under, "", "app unlock --plain"},
		{runner(Program("app")), "yes", "app --yes"},
		{runner(Program("app")), "", "app --plain"},
	} {
		if got := c.s.answerLine(c.flag); got != c.want || strings.Contains(got, "PLANTED") {
			t.Errorf("flag %q: the line is %q, want %q", c.flag, got, c.want)
		}
		n++
	}
	if n != 4 {
		t.Fatalf("read %d lines, want 4", n)
	}
}
