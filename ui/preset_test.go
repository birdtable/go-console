package ui

// ui's look is a preset a program may change: the palette, the
// shared flags, the banner and whether NO_COLOR beats CLICOLOR_FORCE.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"
)

// teal is Contrast with its action colour #112233 on both grounds, so a test
// reads it whatever ground the terminal has: 17;34;51 in an escape.
func teal() Theme {
	t := Contrast()
	t.Action = Colour{Light: "#112233", Dark: "#112233"}
	return t
}

// tealStream is a rich stream painted in teal.
func tealStream() stream { return New(screenOpt(true, 0), WithTheme(teal())).outStream() }

// tealEscape is teal's action colour as a true-colour escape writes it.
const tealEscape = "38;2;17;34;51"

// A session made with WithTheme paints its roles in it, and another session
// in its own.
//
// What it costs when this fails: a program that gives its own colours still
// paints in ui's, or paints every other program in the process in its own.
func TestASessionPaintsInTheThemeItWasGiven(t *testing.T) {
	t.Parallel()
	own := New(screenOpt(true, 80), WithTheme(teal()))
	plain := New(screenOpt(true, 80))
	if got := own.Action("x"); !strings.Contains(got, tealEscape) {
		t.Errorf("the session's Action is %q, want it in its theme's %s", got, tealEscape)
	}
	if strings.Contains(plain.Action("x"), tealEscape) {
		t.Errorf("a session in another theme painted in this one's: %q", plain.Action("x"))
	}
	if own.Bad("x") != plain.Bad("x") || own.Good("x") != plain.Good("x") || own.Attn("x") != plain.Attn("x") {
		t.Errorf("a role the two themes share is drawn differently: %q against %q", own.Bad("x"), plain.Bad("x"))
	}
	var ending bytes.Buffer
	own.Capture(&ending, func() { own.Log("type `go`") })
	if !strings.Contains(ending.String(), tealEscape) {
		t.Errorf("a command in a line is not in the session's action colour: %q", ending.String())
	}
}

// A Mono session paints no colour, in its lines, its tables, its code blocks
// and its help: Bold is bold and Muted faint, and nothing else.
//
// What it costs when this fails: a program that asked for no colour, for a
// terminal or a reader that takes none, gets some.
func TestAMonoSessionPaintsNoColour(t *testing.T) {
	// serial: fang reads the process's CLICOLOR_FORCE to paint the help.
	t.Setenv("CLICOLOR_FORCE", "1")
	s := New(On(Terminal{Unicode: true, Colour: true, ErrColour: true, Width: 80}), WithTheme(Mono()))
	if got := s.Bold("x"); !strings.Contains(got, "\x1b[1m") {
		t.Fatalf("Bold is not bold, so the session paints nothing and this proves little: %q", got)
	}
	if got := s.Note("x"); !strings.Contains(got, "\x1b[2m") {
		t.Errorf("Note is not faint: %q", got)
	}
	var help bytes.Buffer
	root := &cobra.Command{Use: "app", Short: "a made-up program", Run: func(*cobra.Command, []string) {}}
	root.SetOut(&help)
	root.SetErr(&help)
	root.SetArgs([]string{"--help"})
	if err := Execute(context.Background(), root, runner(On(Terminal{Unicode: true, Colour: true, ErrColour: true, Width: 80}), WithTheme(Mono())), testBrand()); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for name, got := range map[string]string{
		"a line":       s.Bad("x") + s.Attn("x") + s.Good("x") + s.Action("x"),
		"a table":      s.Table([]TableColumn{{Title: "name"}, {Title: "state"}}, [][]string{{"a", "b"}}),
		"a code block": s.Code("x := 1"),
		"the help":     help.String(),
	} {
		if c := colourEscape(got); c != "" {
			t.Errorf("%s holds the colour %q: %q", name, c, got)
		}
		checked++
	}
	if checked != 4 {
		t.Fatalf("checked %d, want 4", checked)
	}
}

// colourEscape is the first escape in s that sets a colour, foreground or
// ground, or "" for none.
func colourEscape(s string) string {
	for _, m := range sgr.FindAllString(s, -1) {
		for p := range strings.SplitSeq(strings.TrimSuffix(strings.TrimPrefix(m, "\x1b["), "m"), ";") {
			if n, err := strconv.Atoi(p); err == nil && (n >= 30 && n <= 49 && n != 39 && n != 49 || n >= 90 && n <= 107) {
				return m
			}
		}
	}
	return ""
}

// The help and the errors fang draws, the questions and the logger are in the
// session's palette.
//
// What it costs when this fails: `--help`, a question and a log line are each in
// ui's colours while the rest of the program is in its own.
func TestTheHelpTheQuestionsAndTheLoggerAreInTheSessionsPalette(t *testing.T) {
	scr := screenOpt(true, 80)
	// fang reads the process's CLICOLOR_FORCE to paint the help; the session's
	// Terminal keeps what it paints.
	t.Setenv("CLICOLOR_FORCE", "1")
	// The help, through Execute: fang downsamples, so the help in the palette is
	// held against the same help in the default.
	help := func(run *Session) string {
		root := &cobra.Command{Use: "app", Short: "a made-up program", Run: func(*cobra.Command, []string) {}}
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"--help"})
		if err := Execute(context.Background(), root, run, testBrand()); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		return out.String()
	}
	inTeal, inDefault := help(runner(scr, WithTheme(teal()))), help(runner(scr))
	if !strings.Contains(inDefault, "\x1b[") {
		t.Fatalf("the help is not painted, so the check below proves nothing: %q", inDefault)
	}
	if inTeal == inDefault {
		t.Errorf("the help in the session's palette is the default's:\n%q", inTeal)
	}
	if c := helpColours(tealStream()).Command; c != tealStream().colour(nameAction) {
		t.Errorf("the help's command colour is %v, want the palette's action", c)
	}
	// The questions.
	styles := theme(tealStream()).Theme(true)
	if got, want := styles.Focused.Title.GetForeground(), tealStream().colour(nameAction); got != want {
		t.Errorf("a question's title is %v, want the palette's action %v", got, want)
	}
	if theme(todaysStream()).Theme(true).Focused.Title.GetForeground() == styles.Focused.Title.GetForeground() {
		t.Error("the default theme's title is in the given palette, so the check above proves nothing")
	}
	// The logger, which keeps the colours it paints with and quotes the rest.
	var buf bytes.Buffer
	newLogger(&buf, "app", true, fixedNow, colorprofile.TrueColor, tealStream()).Info("x")
	if !strings.Contains(buf.String(), tealEscape) || strings.Contains(buf.String(), `\x1b`) {
		t.Errorf("an info line is not in the palette's action colour, or its colour was quoted: %q", buf.String())
	}
}

// By default NO_COLOR beats CLICOLOR_FORCE; rules with ForceWins give the force
// the last word. NO_COLOR alone, and
// TERM=dumb, are plain either way.
//
// What it costs when this fails: a person who set NO_COLOR gets colour from a
// program that did not ask to overrule them, or a program that records its own
// screens cannot.
func TestNoColorBeatsTheForceUnlessTheRulesSayOtherwise(t *testing.T) {
	t.Parallel()
	cases := []struct {
		env  []string
		wins bool
		rich bool
	}{
		{[]string{"NO_COLOR=1", "CLICOLOR_FORCE=1", "TERM=xterm-256color"}, false, false},
		{[]string{"NO_COLOR=1", "CLICOLOR_FORCE=1", "TERM=xterm-256color"}, true, true},
		{[]string{"NO_COLOR=1", "TERM=xterm-256color"}, true, false},
		{[]string{"CLICOLOR_FORCE=1", "TERM=dumb", "NO_COLOR=1"}, true, false},
		{[]string{"CLICOLOR_FORCE=1", "TERM=xterm-256color"}, false, true},
	}
	checked := 0
	for _, c := range cases {
		rules := StandardEnv()
		rules.ForceWins = c.wins
		s := New(On(Detect(nil, &bytes.Buffer{}, &bytes.Buffer{}, c.env, rules)))
		if got := s.outStream().paints(); got != c.rich {
			t.Errorf("%v, ForceWins %v: paints %v, want %v", c.env, c.wins, got, c.rich)
		}
		if got := s.errStream().paints(); got != c.rich {
			t.Errorf("%v, ForceWins %v: stderr paints %v, want %v", c.env, c.wins, got, c.rich)
		}
		checked++
	}
	if checked != len(cases) {
		t.Fatalf("checked %d of %d", checked, len(cases))
	}
}

// Execute registers only the flags the program names (FlagNames), and a flag it left unnamed is the
// program's to define: with no JSON name a program may have a --json of its own, and keeps the
// flags it did name.
//
// What it costs when this fails: a program with a --json of its own, older than ui's, cannot take
// ui at all, or ui gives every program a flag spelling nobody chose.
func TestExecuteRegistersOnlyTheFlagsTheProgramNames(t *testing.T) {
	t.Parallel()
	var own bool
	root := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) {}}
	root.Flags().BoolVar(&own, "json", false, "the program's own")
	run := New(houseEndings(), WithMessages(houseMessages()), FlagNames(Flags{Plain: "plain"}))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--json", "--plain"})
	if err := Execute(context.Background(), root, run, testBrand()); err != nil {
		t.Fatalf("a program that named no json flag and defined its own was refused: %v", err)
	}
	if !own || run.JSON() {
		t.Errorf("--json set the program's flag %v and the session's JSON %v, want the program's only", own, run.JSON())
	}
	if !run.plain.Load() {
		t.Error("--plain, which the program named, did not set the session")
	}
	if out.Len() != 0 {
		t.Errorf("a run that named no json flag printed a Result: %q", out.String())
	}
	// No flags named: none are registered.
	bare := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) {}}
	if _, err := sharedRun(t, bare, New(houseEndings(), WithMessages(houseMessages()))); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"plain", "accessible", "yes", "json", "no-color", "apply"} {
		if bare.PersistentFlags().Lookup(name) != nil || bare.Flags().Lookup(name) != nil {
			t.Errorf("--%s is registered though the program named no flags", name)
		}
	}
	// A flag it named and also defines is refused.
	again := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) {}}
	again.Flags().Bool("json", false, "the program's own")
	if _, err := sharedRun(t, again, runner()); err == nil {
		t.Error("a program that names --json and defines it too ran")
	}
}

// ui's code and its doc name no one program's look: no person, no product, no
// one who ruled. A preset is ui's own, for any program.
//
// What it costs when this fails: a program that takes ui reads in its doc that
// its look is somebody else's, and the foundation reads as one owner's tool.
func TestUINamesNoOneProgramsLook(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no Go files: %v", err)
	}
	owner := regexp.MustCompile(`(?i)\b(operator|owner)('s)?\b`)
	read := 0
	for _, f := range files {
		if strings.HasSuffix(f, "preset_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(b), "\n")
		looked := 0
		for i, line := range lines {
			if m := owner.FindString(line); m != "" {
				t.Errorf("%s:%d says %q: %s", f, i+1, m, strings.TrimSpace(line))
			}
			looked++
		}
		if looked != len(lines) {
			t.Fatalf("%s: looked at %d of %d lines", f, looked, len(lines))
		}
		read++
	}
	if read < 10 {
		t.Fatalf("read %d files, want ui's", read)
	}
}

// Execute draws the banner only when the program asks with WithBanner.
//
// What it costs when this fails: every program that takes ui opens each command
// with a banner it did not ask for, or one that asked draws none.
func TestExecuteDrawsTheBannerOnlyWhenAsked(t *testing.T) {
	t.Parallel()
	scr := screenOpt(true, 0)
	drawn := func(opts ...Option) string {
		root := &cobra.Command{Use: "app", Run: func(*cobra.Command, []string) {}}
		root.SetArgs(nil)
		run := runner(scr)
		var out bytes.Buffer
		run.Capture(&out, func() {
			if err := Execute(context.Background(), root, run, testBrand(), opts...); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		return out.String()
	}
	mark := wordmarkOf(testBrand().Name)[0]
	if got := drawn(); strings.Contains(got, mark) {
		t.Errorf("Execute drew the banner unasked:\n%s", got)
	}
	if got := drawn(WithBanner()); !strings.Contains(got, mark) {
		t.Errorf("Execute with WithBanner drew no banner:\n%q", got)
	}
}

// A session paints only in the theme it was given: every colour escape in its
// lines, its notes, a table's rules, a tree's marks and a code block's ground is
// one of the theme's.
//
// What it costs when this fails: a program's own colours sit beside ui's, so its
// look is two looks.
func TestOnlyTheThemesColoursArePainted(t *testing.T) {
	t.Parallel()
	own := Theme{
		Bad: Colour{"#110000", "#110000"}, Attn: Colour{"#001100", "#001100"}, Good: Colour{"#000011", "#000011"},
		Action: Colour{"#111100", "#111100"}, Muted: Colour{"#001111", "#001111"}, CodeGround: Colour{"#110011", "#110011"},
	}
	allowed := map[string]bool{}
	for _, c := range []string{"17;0;0", "0;17;0", "0;0;17", "17;17;0", "0;17;17"} {
		allowed["38;2;"+c] = true
	}
	allowed["48;2;17;0;17"] = true
	s := New(On(Terminal{Unicode: true, Colour: true, ErrColour: true, Width: 80}), WithTheme(own))
	drawn := s.Bad("x") + s.Attn("x") + s.Good("x") + s.Action("x") + s.Note("x") +
		s.Table([]TableColumn{{Title: "name"}, {Title: "state"}}, [][]string{{"a", "b"}}) + s.Code("x := 1")
	seen := map[string]bool{}
	escapes := sgr.FindAllString(drawn, -1)
	read := 0
	for _, m := range escapes {
		body := strings.TrimSuffix(strings.TrimPrefix(m, "\x1b["), "m")
		asked := 0
		for c := range allowed {
			if strings.Contains(body, c) {
				seen[c] = true
				body = strings.ReplaceAll(body, c, "")
			}
			asked++
		}
		if asked != len(allowed) {
			t.Fatalf("asked %d of %d colours", asked, len(allowed))
		}
		read++
		if c := colourEscape("\x1b[" + body + "m"); c != "" {
			t.Errorf("an escape the theme does not hold: %q in %q", m, drawn)
		}
	}
	if read == 0 || read != len(escapes) {
		t.Fatalf("read %d of %d escapes", read, len(escapes))
	}
	if len(seen) != len(allowed) {
		t.Errorf("drew %d of the theme's %d colours, so a role went unpainted: %v", len(seen), len(allowed), seen)
	}
}

// A session given no markup prints a line as it was written, backticks and
// all, even where it paints; BacktickMarkup paints the command and drops them.
//
// What it costs when this fails: a program that writes backticks for its own
// reasons finds them gone, and a word painted it never asked to paint.
func TestALineIsPrintedAsWrittenWithoutMarkup(t *testing.T) {
	t.Parallel()
	var plain, marked bytes.Buffer
	at := On(Terminal{Unicode: true, Colour: true, ErrColour: true})
	New(Out(&plain), at).Log("run `app sync` now")
	New(Out(&marked), at, WithMarkup(BacktickMarkup)).Log("run `app sync` now")
	if plain.String() != "run `app sync` now\n" {
		t.Errorf("with no markup the line came out %q", plain.String())
	}
	if strings.Contains(marked.String(), "`") || !strings.Contains(marked.String(), "\x1b[") {
		t.Errorf("BacktickMarkup left the backticks or painted nothing: %q", marked.String())
	}
}
