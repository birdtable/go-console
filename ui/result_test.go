package ui

// The Result of version 3, and a panic that ends a run as a failed one.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/birdtable/go-console/uitest"
)

// runDoc runs a command that does fn under --json and returns what reached stdout.
func runDoc(t *testing.T, run *Session, fn func(run *Session) error) string {
	t.Helper()
	root := &cobra.Command{Use: "app", SilenceErrors: true, RunE: func(*cobra.Command, []string) error { return fn(run) }}
	out, _ := sharedRun(t, root, run, "--json")
	return out
}

// Each of exit's seven codes, 0 to 6, is a golden in version 3, and its
// document has one error object only where the command returned an error.
//
// What it costs when this fails: a script reads one ending's document as
// another's, a failure arrives with no kind to branch on, or a look changes
// and no reviewer reads the diff.
func TestEachCodeOfExitIsAGoldenInVersionThree(t *testing.T) {
	t.Parallel()
	n := 0
	for _, c := range []struct {
		name string
		end  closing
		err  error
	}{
		{"0-done", done(), nil},
		{"1-failed", failed(), errors.New("the step did not work")},
		{"2-usage", usage(), nil},
		{"3-busy", closing{word: "busy", what: "another run holds the lock", next: "app status", code: 3}, nil},
		{"4-not-ready", closing{word: "not ready", what: "no config yet", next: "app init", code: 4}, nil},
		{"5-refused", refusalFrame(), nil},
		{"6-declined", declined(), nil},
	} {
		run := runner()
		out := runDoc(t, run, func(run *Session) error { run.End(c.end); return c.err })
		uitest.Golden(t, "result-code-"+c.name, out)
		r, err := ReadResult([]byte(out))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if r.Code != c.end.code || (r.Error != nil) != (c.err != nil) {
			t.Errorf("%s: code %d and error %+v, want code %d and an error %v", c.name, r.Code, r.Error, c.end.code, c.err != nil)
		}
		n++
	}
	if n != 7 {
		t.Fatalf("held %d endings, want 7", n)
	}
}

// The error's kind is the word of the ending the command returned, whatever
// the code: a failure that ended on "findings" says findings.
//
// What it costs when this fails: ui keeps a table from code to word, and
// two words that share a code (findings and failed) read as one.
func TestAnErrorsKindIsTheWordOfItsEnding(t *testing.T) {
	t.Parallel()
	for _, e := range []closing{findings(), failed(), {word: "a made-up word", what: "x", next: "app y", code: 1}} {
		out := runDoc(t, runner(), func(run *Session) error { run.End(e); return errors.New("it broke") })
		r, err := ReadResult([]byte(out))
		if err != nil || r.Error == nil || r.Error.Kind != e.word {
			t.Errorf("an error on %q answered %+v (%v), want that word as its kind", e.word, r.Error, err)
		}
	}
}

// The next line is what the command set, never what a scan of the message
// finds: a message with two backticked words leaves the line as it was.
//
// What it costs when this fails: a script types a word out of an error's
// sentence as a command.
func TestTheNextLineIsNeverFoundInTheMessage(t *testing.T) {
	t.Parallel()
	e := closing{word: "failed", what: "run `app fix` or `app undo` first", next: "app logs", code: 1}
	r, err := ReadResult([]byte(runDoc(t, runner(), func(run *Session) error { run.End(e); return errors.New(e.what) })))
	if err != nil || r.Next != "app logs" || r.Error == nil || r.Error.Hint != "app logs" {
		t.Errorf("the next line is %q and the hint %+v (%v), want app logs", r.Next, r.Error, err)
	}
	bare := closing{word: "findings", what: "run `app fix` or `app undo` first", code: 1}
	if r := New(JSON()).Result(nil); r.Next != "" {
		t.Errorf("a session that ended on nothing answered next %q", r.Next)
	}
	s := New(JSON())
	s.End(bare)
	if got := s.Result(nil).Next; got != "" {
		t.Errorf("findings with no line answered next %q, found in the message", got)
	}
}

// Command, apply, run and data are in the document when set and absent, not
// null, when not.
//
// What it costs when this fails: a script reads a null for a field the
// program never had, or loses one the program set.
func TestOptionalFieldsAreAbsentNotNull(t *testing.T) {
	t.Parallel()
	bare := New(JSON())
	bare.End(done())
	var b bytes.Buffer
	if err := bare.Result(nil).Write(&b); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"command", "apply", "run", "data", "error"} {
		if _, ok := doc[k]; ok {
			t.Errorf("%q is in a document that set none of them: %s", k, b.String())
		}
	}
	set := New(JSON())
	set.SetDryRun(false)
	set.SetRun("r-1")
	set.SetData(map[string]int{"n": 2})
	set.End(done())
	b.Reset()
	if err := set.Result(nil).Write(&b); err != nil {
		t.Fatal(err)
	}
	doc = map[string]any{}
	if err := json.Unmarshal(b.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	n := 0
	for k, want := range map[string]any{"apply": true, "run": "r-1", "data": map[string]any{"n": float64(2)}} {
		if got, ok := doc[k]; !ok || !jsonEqual(got, want) {
			t.Errorf("%q is %v (present %v), want %v", k, got, ok, want)
		}
		n++
	}
	if n != 3 {
		t.Fatalf("checked %d set fields, want 3", n)
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

// A reader of version 3 refuses a document of version 2 with a named error
// and reads one of its own.
//
// What it costs when this fails: a consumer takes version 2's error string
// for an object, or cannot tell that it was handed another shape.
func TestAVersionThreeReaderRefusesVersionTwo(t *testing.T) {
	t.Parallel()
	_, err := ReadResult([]byte(`{"version":2,"word":"failed","what":"x","next":"","code":1,"changes":[],"dry_run":true,"error":"it broke"}`))
	if !errors.Is(err, ErrResultVersion) {
		t.Errorf("a version 2 document read with %v, want ErrResultVersion", err)
	}
	if _, err := ReadResult([]byte(`{"version":3,"word":"done","what":"x","next":"","code":0,"changes":[],"dry_run":true}`)); err != nil {
		t.Errorf("a version 3 document was refused: %v", err)
	}
	if _, err := ReadResult([]byte(`not json`)); err == nil || errors.Is(err, ErrResultVersion) {
		t.Errorf("a document that is not JSON read with %v", err)
	}
}

// The three shapes that existed for a run's answer, by the fields they have,
// each moved into version 3 by a stated map (word, sentence, error kind and
// the rest), so the move loses no field. The shapes are the fields of the
// three copies the move replaces, not their programs.
type shapeWordy struct {
	Schema  int      `json:"schema"`
	Command string   `json:"command"`
	Apply   bool     `json:"apply"`
	Ended   string   `json:"ended"`
	Msg     string   `json:"msg"`
	Next    string   `json:"next"`
	Run     string   `json:"run"`
	Changes []string `json:"changes"`
	Data    any      `json:"data"`
	Error   *struct {
		Kind string `json:"kind"`
		Msg  string `json:"msg"`
		Hint string `json:"hint"`
	} `json:"error"`
}

type shapeShort struct {
	Command string   `json:"command"`
	Apply   bool     `json:"apply"`
	Ended   string   `json:"ended"`
	Msg     string   `json:"msg"`
	Next    string   `json:"next"`
	Changes []string `json:"changes"`
	Error   *struct {
		Kind string `json:"kind"`
		Msg  string `json:"msg"`
		Hint string `json:"hint"`
	} `json:"error"`
}

type shapeKinded struct {
	Schema int `json:"schema"`
	Data   any `json:"data"`
	Error  *struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
	} `json:"error"`
}

// The map of each shape: word <- ended, sentence <- msg, error kind <- error.kind.
func fromWordy(s shapeWordy) Result {
	r := Result{Version: 3, Command: s.Command, Apply: &s.Apply, Word: s.Ended, What: s.Msg, Next: s.Next, Run: s.Run, Changes: s.Changes, Data: s.Data}
	if s.Error != nil {
		r.Error = &Fault{Kind: s.Error.Kind, Msg: s.Error.Msg, Hint: s.Error.Hint}
	}
	return r
}

func fromShort(s shapeShort) Result {
	r := Result{Version: 3, Command: s.Command, Apply: &s.Apply, Word: s.Ended, What: s.Msg, Next: s.Next, Changes: s.Changes}
	if s.Error != nil {
		r.Error = &Fault{Kind: s.Error.Kind, Msg: s.Error.Msg, Hint: s.Error.Hint}
	}
	return r
}

func fromKinded(s shapeKinded) Result {
	r := Result{Version: 3, Data: s.Data}
	if s.Error != nil {
		r.Error = &Fault{Kind: s.Error.Kind, Msg: s.Error.Message}
	}
	return r
}

// What it costs when this fails: a program that moves onto version 3 loses a
// field its scripts read, as the error's kind.
func TestEachCurrentShapeMovesIntoVersionThreeWithNoFieldLost(t *testing.T) {
	t.Parallel()
	yes := true
	cases := []struct {
		name string
		doc  string
		got  func(doc string) Result
		want Result
	}{
		{"wordy", `{"schema":1,"command":"app sync","apply":true,"ended":"failed","msg":"no network","next":"app logs","run":"r-9","changes":["write a"],"data":{"n":1},"error":{"kind":"failed","msg":"no network","hint":"app logs"}}`,
			func(d string) Result { var s shapeWordy; mustJSON(t, d, &s); return fromWordy(s) },
			Result{Version: 3, Command: "app sync", Apply: &yes, Word: "failed", What: "no network", Next: "app logs", Run: "r-9", Changes: []string{"write a"}, Data: map[string]any{"n": float64(1)}, Error: &Fault{Kind: "failed", Msg: "no network", Hint: "app logs"}}},
		{"short", `{"command":"app sync","apply":true,"ended":"refused","msg":"not here","next":"app init","changes":[],"error":{"kind":"refused","msg":"not here","hint":"app init"}}`,
			func(d string) Result { var s shapeShort; mustJSON(t, d, &s); return fromShort(s) },
			Result{Version: 3, Command: "app sync", Apply: &yes, Word: "refused", What: "not here", Next: "app init", Changes: []string{}, Error: &Fault{Kind: "refused", Msg: "not here", Hint: "app init"}}},
		{"kinded", `{"schema":1,"data":{"n":1},"error":{"kind":"busy","message":"locked"}}`,
			func(d string) Result { var s shapeKinded; mustJSON(t, d, &s); return fromKinded(s) },
			Result{Version: 3, Data: map[string]any{"n": float64(1)}, Error: &Fault{Kind: "busy", Msg: "locked"}}},
	}
	n := 0
	for _, c := range cases {
		got := c.got(c.doc)
		if !jsonEqual(got, c.want) {
			t.Errorf("%s moved to %s, want %s", c.name, mustMarshal(got), mustMarshal(c.want))
		}
		// Planted red: a map that drops error.kind is told from the real one.
		lossy := got
		e := *lossy.Error
		e.Kind = ""
		lossy.Error = &e
		if jsonEqual(lossy, c.want) {
			t.Errorf("%s: a map that drops error.kind passed", c.name)
		}
		n++
	}
	if n != 3 {
		t.Fatalf("moved %d shapes, want 3", n)
	}
}

func mustJSON(t *testing.T, doc string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(doc), v); err != nil {
		t.Fatal(err)
	}
}

func mustMarshal(v any) string { b, _ := json.Marshal(v); return string(b) }

// documents is how many JSON documents s holds, or -1 when some of it is not one.
func documents(s string) int {
	dec := json.NewDecoder(strings.NewReader(s))
	n := 0
	for dec.More() {
		var v any
		if err := dec.Decode(&v); err != nil {
			return -1
		}
		n++
	}
	return n
}

// A command that panics ends the run as a failed one, with exit 1's word and
// one document on stdout, whose error says the failed word and not the
// panic's value; the stack goes to stderr. The ending is a golden.
//
// What it costs when this fails: a script under --json reads nothing, or two
// documents, from a run that crashed, or a secret in a panic's value reaches
// its logs.
func TestACommandThatPanicsEndsAsAFailedRunWithOneDocument(t *testing.T) {
	t.Parallel()
	run := runner()
	root := &cobra.Command{Use: "app", SilenceErrors: true, RunE: func(*cobra.Command, []string) error {
		run.Log("half the output")
		panic("token s3cret-value")
	}}
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"--json"})
	err := Execute(context.Background(), root, run, testBrand())
	if err == nil {
		t.Fatal("a panic ended the run with no error")
	}
	if got := documents(out.String()); got != 1 {
		t.Fatalf("stdout holds %d documents: %q", got, out.String())
	}
	if strings.Contains(out.String(), "s3cret-value") {
		t.Errorf("the panic's value is in the document: %s", out.String())
	}
	r, rerr := ReadResult(out.Bytes())
	if rerr != nil || r.Word != "failed" || r.Code != 1 || r.Error == nil || r.Error.Kind != "failed" {
		t.Errorf("the panic answered %+v (%v), want the failed word and exit 1", r, rerr)
	}
	if !strings.Contains(errb.String(), "s3cret-value") || !strings.Contains(errb.String(), "goroutine") {
		t.Errorf("stderr holds no panic and no stack: %q", errb.String())
	}
	uitest.Golden(t, "panic-json", out.String())
}

// After output has begun, a panic still ends in one closing line, in text.
//
// What it costs when this fails: a person watching a run that crashed sees
// half an output and no word on how it ended.
func TestAPanicAfterOutputStillEndsInOneClosingLine(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it captures the process's stderr.
	run := runner()
	root := &cobra.Command{Use: "app", SilenceErrors: true, RunE: func(_ *cobra.Command, _ []string) error {
		run.Log("half the output")
		panic("boom")
	}}
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{})
	stderr := capture(t, &os.Stderr, func() {
		if err := Execute(context.Background(), root, run, testBrand()); err == nil {
			t.Error("a panic ended the run with no error")
		}
	})
	if got := strings.Count(stderr, "  failed "); got != 1 || !strings.Contains(stderr, "app --help") {
		t.Errorf("the run printed %d closing lines, want 1: %q", got, stderr)
	}
}

// A program words its panic with PanicWith, and the value still never
// reaches the document.
//
// What it costs when this fails: a program cannot say what its crash means
// to the person, or its words are dropped for ui's.
func TestPanicWithWordsThePanicAndKeepsItsValueOut(t *testing.T) {
	t.Parallel()
	run := runner(PanicWith(func(any) (string, string) { return "the sync broke", "app logs" }))
	out := runDoc(t, run, func(*Session) error { panic("s3cret-value") })
	r, err := ReadResult([]byte(out))
	if err != nil || r.What != "the sync broke" || r.Next != "app logs" || strings.Contains(out, "s3cret-value") {
		t.Errorf("the panic answered %q (%v), want its own words and no value", out, err)
	}
}

// A runtime.Goexit is left alone: Execute does not return, and prints no
// document, so a goroutine that ended itself is not made a failure.
//
// What it costs when this fails: a command that ends its own goroutine is
// reported as a crash.
func TestAGoexitIsLeftAlone(t *testing.T) {
	t.Parallel()
	run := runner()
	root := &cobra.Command{Use: "app", SilenceErrors: true, RunE: func(*cobra.Command, []string) error { runtime.Goexit(); return nil }}
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"--json"})
	returned := make(chan bool, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = Execute(context.Background(), root, run, testBrand())
		returned <- true
	}()
	<-done
	if len(returned) != 0 {
		t.Error("Execute returned after a Goexit")
	}
	if out.Len() != 0 || run.Ended() != nil {
		t.Errorf("a Goexit printed %q and ended the run on %v", out.String(), run.Ended())
	}
}

// An os.Exit is left alone: the process ends on the command's code, and
// prints no document.
//
// What it costs when this fails: a command that exits itself is turned into
// a crash, or its code is lost.
func TestAnOsExitIsLeftAlone(t *testing.T) {
	t.Parallel()
	// serial: it starts a process.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), exe, "-test.run=^TestOsExitChild$", "ui-exit-child")
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 7 {
		t.Fatalf("the child ended with %v, want exit 7", err)
	}
	if strings.Contains(out.String(), `"version"`) {
		t.Errorf("an os.Exit was followed by a document: %q", out.String())
	}
}

// TestOsExitChild is the child of the test above: it does nothing unless it
// was started with that word, and then runs a command that exits.
//
// What it costs when this fails: the test above cannot tell an os.Exit that
// was left alone from one that was turned into a crash.
func TestOsExitChild(_ *testing.T) { //nolint:paralleltest // serial: it is the child process of another test, not a test of its own
	// serial: it exits the process.
	if !slices.Contains(flag.Args(), "ui-exit-child") {
		return // only the child of TestAnOsExitIsLeftAlone runs the rest
	}
	run := runner()
	root := &cobra.Command{Use: "app", SilenceErrors: true, RunE: func(*cobra.Command, []string) error {
		os.Exit(7) //nolint:revive // the child process of the exit test must end with its code
		return nil
	}}
	root.SetArgs([]string{"--json"})
	_ = Execute(context.Background(), root, run, testBrand())
}
