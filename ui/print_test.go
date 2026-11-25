package ui

// What a session prints is its own: --json and `--no-color` are the session's,
// as are its changes and its dry run, so two runs in one process each print,
// keep and answer their own.

import (
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
)

// Two sessions that print at once, in goroutines of their own, one under
// --json as a dry run and one that applies under `--no-color`, each print,
// keep and answer only their own: the JSON one prints nothing to stdout, the
// other prints its lines with no escape, each session's changes and Result
// hold its own lines alone, and only the one that applies tells its OnChange.
//
// What it costs when this fails: two command tests run side by side see each
// other's lines, changes or answer, so every test that runs a command must
// run alone; or one run's --json, `--no-color` or dry run reaches another run
// in the same process, and a change really made is logged as planned, or the
// reverse.
func TestTwoSessionsAtOnceEachPrintKeepAndAnswerTheirOwn(t *testing.T) {
	// serial: it reads the process's stdout at a terminal that paints.
	scr := screenOpt(true, 0)
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	var heardA, heardB atomic.Int32
	a := New(WithMessages(houseMessages()), scr, JSON(), OnChange(func(string) { heardA.Add(1) }))
	b := New(WithMessages(houseMessages()), scr, OnChange(func(string) { heardB.Add(1) }))
	b.SetDryRun(false)
	b.noColor.Store(true)
	const rounds = 100
	started := 0
	out := capture(t, &os.Stdout, func() {
		var wg sync.WaitGroup
		for _, run := range []struct {
			s    *Session
			name string
		}{{a, "a"}, {b, "b"}} {
			wg.Go(func() {
				for i := range rounds {
					run.s.Log("%s says %d", run.name, i)
					run.s.Change("write /%s/%d", run.name, i)
				}
			})
			wg.Go(func() {
				for range rounds {
					_, _, _ = run.s.Changes(), run.s.Changed(), run.s.Result(nil)
				}
			})
			started++
		}
		wg.Wait()
	})
	if started != 2 {
		t.Fatalf("started %d sessions, want 2", started)
	}
	if strings.Contains(out, "a says") || strings.Contains(out, "/a/") {
		t.Error("the session under --json printed to stdout")
	}
	if says, changes := strings.Count(out, "b says"), strings.Count(out, "/b/"); says != rounds || changes != rounds {
		t.Errorf("the session without --json printed %d of its %d lines and %d of its changes", says, rounds, changes)
	}
	if strings.Contains(out, "\x1b") || strings.Contains(out, "would") {
		t.Errorf("the session under `--no-color` that applies printed an escape or a plan: %q", out)
	}
	for _, c := range []struct {
		s     *Session
		name  string
		dry   bool
		heard int32
	}{{a, "a", true, heardA.Load()}, {b, "b", false, heardB.Load()}} {
		changes := c.s.Changes()
		own := 0
		for _, line := range changes {
			if strings.HasPrefix(line, "write /"+c.name+"/") {
				own++
			}
		}
		if own != rounds || len(changes) != rounds || !c.s.Changed() {
			t.Errorf("%s kept %d changes, %d of them its own, want %d", c.name, len(changes), own, rounds)
		}
		if got := c.s.Result(nil).Changes; strings.Join(got, "\n") != strings.Join(changes, "\n") {
			t.Errorf("%s answered %d changes, not its own %d", c.name, len(got), len(changes))
		}
		if c.s.DryRun() != c.dry {
			t.Errorf("%s is a dry run: %v, want %v", c.name, c.s.DryRun(), c.dry)
		}
		if want := map[bool]int32{true: 0, false: rounds}[c.dry]; c.heard != want {
			t.Errorf("%s told its OnChange of %d changes, want %d", c.name, c.heard, want)
		}
	}
	// The negative half: at the same terminal a session with colour paints.
	if out := capture(t, &os.Stdout, func() { New(scr, WithMessages(houseMessages())).Change("write /c") }); !strings.Contains(out, "\x1b") {
		t.Fatalf("a session with colour printed no escape, so the check above proves nothing: %q", out)
	}
}

// Everything a session under `--no-color` prints, on stdout and on stderr,
// holds no escape at a terminal that paints, and its bar and wait do not
// redraw in place; the same calls in a session with colour paint.
//
// What it costs when this fails: a person who passed `--no-color` gets
// colour anyway from some of the lines, or a log of the run holds a bar's
// redraws as escapes.
func TestNoColorStripsEverythingTheSessionPrints(t *testing.T) {
	// serial: it reads the process's streams at a terminal that paints.
	scr := screenOpt(true, 0)
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	n := 0
	for _, p := range []struct {
		name   string
		stream **os.File
		print  func(s *Session)
	}{
		{"Log", &os.Stdout, func(s *Session) { s.Log("%s", New(scr).Bold("x")) }},
		{"Info", &os.Stdout, func(s *Session) { s.Info("%s", New(scr).Note("x")) }},
		{"Change", &os.Stdout, func(s *Session) { s.Change("write x") }},
		{"Assumed", &os.Stdout, func(s *Session) { s.Assumed("format the disk") }},
		{"End", &os.Stdout, func(s *Session) { s.End(done()) }},
		{"Warn", &os.Stderr, func(s *Session) { s.Warn(failed()) }},
	} {
		plain := New(scr, WithMessages(houseMessages()), houseEndings())
		plain.noColor.Store(true)
		if out := capture(t, p.stream, func() { p.print(plain) }); out == "" || strings.Contains(out, "\x1b") {
			t.Errorf("%s under `--no-color` printed %q", p.name, out)
		}
		if out := capture(t, p.stream, func() { p.print(New(scr, WithMessages(houseMessages()), houseEndings())) }); !strings.Contains(out, "\x1b") {
			t.Errorf("%s with colour printed no escape, so its check proves nothing: %q", p.name, out)
		}
		n++
	}
	if n != 6 {
		t.Fatalf("tried %d printers, want 6", n)
	}
	// The bar redraws in place only at a terminal a person is at.
	plain := New(scr, richTerminal())
	plain.noColor.Store(true)
	if plain.moves() || !New(scr, richTerminal()).moves() {
		t.Errorf("under `--no-color` the bar redraws in place: %v, or with colour it does not: %v", plain.moves(), New(scr, richTerminal()).moves())
	}
}

// `--no-color` strips the escapes from what the command writes through
// cobra's writers too, and a run without it keeps them.
//
// What it costs when this fails: a command's own table or help keeps its
// colour when a person asked for none.
func TestNoColorStripsWhatTheCommandWrites(t *testing.T) {
	// serial: it sets the process's streams and environment.
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	const painted = "\x1b[1mx\x1b[0m\n"
	n := 0
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--no-color"}, "x\n"},
		{nil, painted},
	} {
		root := &cobra.Command{Use: "app", Run: func(c *cobra.Command, _ []string) {
			_, _ = c.OutOrStdout().Write([]byte(painted))
		}}
		if out, err := sharedRun(t, root, runner(screenOpt(true, 0)), c.args...); err != nil || out != c.want {
			t.Errorf("%v: the command's line came out %q (%v), want %q", c.args, out, err, c.want)
		}
		n++
	}
	if n != 2 {
		t.Fatalf("ran %d command lines, want 2", n)
	}
}

// Passthrough is the door a child process's output goes through: the
// process's own streams, untouched under `--no-color`, nothing on stdout
// under --json, and the writer a Capture catches lines in while its session
// draws.
//
// What it costs when this fails: apt's progress or a login link vanishes from
// a run somebody is watching, or lands in the middle of a JSON answer a
// script reads, or prints over a live view.
func TestPassthroughGivesAChildTheScreenAndNothingUnderJSON(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it reads os.Stdout, which the tests that read the process's
	// stdout swap.
	if out, errOut := New().Passthrough(); out != os.Stdout || errOut != os.Stderr {
		t.Errorf("a session gave a child %v and %v, want the process's streams", out, errOut)
	}
	plain := New()
	plain.noColor.Store(true)
	if out, _ := plain.Passthrough(); out != os.Stdout {
		t.Errorf("under `--no-color` a child got %v, want its output left as it wrote it", out)
	}
	if out, errOut := New(JSON()).Passthrough(); out != io.Discard || errOut != os.Stderr {
		t.Errorf("under --json a child got %v and %v, want nothing on stdout and stderr as it is", out, errOut)
	}
	s := New()
	var b strings.Builder
	s.Capture(&b, func() {
		out, _ := s.Passthrough()
		_, _ = io.WriteString(out, "child line")
	})
	if b.String() != "child line" {
		t.Errorf("while drawing, a child's line went elsewhere: caught %q", b.String())
	}
}

// A session's Follow under `--json` prints nothing on stdout and shows no
// viewport, so a guest's line that is a JSON document never goes ahead of
// the run's Result; the same Follow without `--json` prints the line, and at
// a terminal shows the viewport.
//
// What it costs when this fails: a guest or a server writes a line such as
// {"version":1,"code":0}, and a script that decodes the first document on
// stdout takes it as the run's answer.
func TestAFollowUnderJSONPrintsNothingOnStdout(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	// serial: it sets the process's streams and reads its stdout.
	guest := `{"version":1,"code":0,"changes":[]}`
	n := 0
	for _, c := range []struct {
		name string
		s    *Session
		want string
	}{
		{"under --json", New(JSON()), ""},
		{"without --json", New(), guest + "\n"},
	} {
		f := c.s.NewFollow()
		out := capture(t, &os.Stdout, func() {
			f.Line(guest)
			f.Stop()
			if err := f.Run(); err != nil {
				t.Errorf("%s: Run = %v", c.name, err)
			}
		})
		if out != c.want {
			t.Errorf("%s: a Follow printed %q on stdout, want %q", c.name, out, c.want)
		}
		n++
	}
	if New(richTerminal(), JSON()).NewFollow().follows() || !New(richTerminal()).NewFollow().follows() {
		t.Error("at a terminal a JSON session's Follow shows a viewport, or another session's does not")
	}
	if n != 2 {
		t.Fatalf("ran %d Follows, want 2", n)
	}
}

// Changes hands out a copy: a caller that changes or appends to what it got
// leaves the session's record as it was.
//
// What it costs when this fails: a program that sorts or extends the list it
// read rewrites the run's record, and its Result and run log say what the
// program wrote there, not what the run changed.
func TestChangesHandsOutACopy(t *testing.T) {
	t.Parallel()
	s := New(WithMessages(houseMessages()), JSON())
	s.Change("write /a")
	s.Change("write /b")
	got := s.Changes()
	got[0] = "write /elsewhere"
	if got = append(got[:1], "write /planted"); got[1] != "write /planted" {
		t.Fatalf("the append went nowhere: %q", got)
	}
	if again := s.Changes(); len(again) != 2 || again[0] != "write /a" || again[1] != "write /b" {
		t.Errorf("a caller's edit reached the record: %q", again)
	}
}

// Result reads the changes under the session's lock while Change records
// more, in goroutines of their own, which -race holds.
//
// What it costs when this fails: a Result built while a command still
// reports changes, as a live view's run, is torn or races, and the answer
// misses or repeats a change.
func TestResultReadsTheChangesUnderTheLock(t *testing.T) {
	t.Parallel()
	s := New(WithMessages(houseMessages()), JSON())
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := range 200 {
			s.Change("write /%d", i)
		}
	})
	read := 0
	wg.Go(func() {
		for range 200 {
			_ = s.Result(nil)
			read++
		}
	})
	wg.Wait()
	if got := s.Result(nil).Changes; read != 200 || len(got) != 200 {
		t.Fatalf("read %d results and the last holds %d changes, want 200 and 200", read, len(got))
	}
}

// A dry run's Result and a real run's with the same changes and ending
// differ only in dry_run, which the dry run's says is true and the real
// run's false, in the version that added it. Apply, which says
// the same when the program set it (version 3), is the other field that differs.
//
// What it costs when this fails: a script that runs a command under --json
// without --apply reads the changes as made, and acts on work that was only
// planned; or the field moves with no new version, and a script that knows
// version 1 reads a shape it does not know.
func TestADryRunsResultSaysItWasADryRun(t *testing.T) {
	t.Parallel()
	decoded := map[bool]map[string]any{}
	for _, dry := range []bool{true, false} {
		s := New(WithMessages(houseMessages()), JSON())
		s.SetDryRun(dry)
		s.Change("write /a")
		s.Change("remove /b")
		s.End(done())
		var b strings.Builder
		if err := s.Result(nil).Write(&b); err != nil {
			t.Fatalf("the result of a run (dry %v) was not written: %v", dry, err)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(b.String()), &doc); err != nil {
			t.Fatalf("the result of a run (dry %v) is not JSON: %v\n%s", dry, err, b.String())
		}
		decoded[dry] = doc
	}
	read := 0
	for dry, doc := range decoded {
		read++
		if got, ok := doc["dry_run"]; !ok || got != dry {
			t.Errorf("the result of a run (dry %v) says dry_run %v (present %v), want %v", dry, got, ok, dry)
		}
		if doc["version"] != float64(3) {
			t.Errorf("the result of a run (dry %v) is version %v, want 3", dry, doc["version"])
		}
		if got, ok := doc["apply"]; !ok || got != !dry {
			t.Errorf("the result of a run (dry %v) says apply %v (present %v), want %v", dry, got, ok, !dry)
		}
		delete(doc, "dry_run")
		delete(doc, "apply")
	}
	if read != 2 {
		t.Fatalf("read %d results, want a dry run's and a real run's", read)
	}
	if !reflect.DeepEqual(decoded[true], decoded[false]) {
		t.Errorf("a dry run's result and a real run's differ beyond dry_run:\n%v\n%v", decoded[true], decoded[false])
	}
	if len(decoded[true]) != 6 {
		t.Errorf("the result holds %d fields beside dry_run and apply, want 6 (version, word, what, next, code, changes): %v", len(decoded[true]), decoded[true])
	}
}
