package ui

// The screens a person meets first, drawn at 80, 60 and 40 columns (cli.cycle.1,
// cli.cycle.2, cli.commands.15): no row is wider than the screen, nothing is cut off, and the
// words are the same and in the same order as at 80, which each frame has a golden of. The plain
// and the accessible modes have goldens too. A screen is drawn through the same seams every
// other test here uses; no pty is run.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"

	"github.com/birdtable/go-console/uitest"
)

// narrowWidths are the widths each frame is checked at, the first being the golden's.
func narrowWidths() []int { return []int{80, 60, 40} }

// frameDraw draws one frame as a screen of width columns, painted as a terminal would be, with
// its styling stripped: a golden is the text a person reads.
type frameDraw struct {
	name string
	// draw is the frame as the person reads it.
	draw func(t *testing.T, width int) string
}

func menuChoices() []Choice {
	return []Choice{
		{Label: "alpha", Blurb: "the first thing to do, with a blurb that runs long", Value: "a"},
		{Label: "beta", Blurb: "the second", Value: "b"},
	}
}

// printed runs f with a session option for a screen of width columns, rich or
// plain, and returns what it wrote to stdout and stderr.
func printed(t *testing.T, width int, rich bool, f func(scr SessionOption)) string {
	t.Helper()
	return ansi.Strip(captureBoth(t, func() { f(screenOpt(rich, width)) }))
}

// menuFrame is the menu as a form draws it, shown as Pick shows it: no help line.
func menuFrame(width int) string {
	var got string
	f := huh.NewForm(huh.NewGroup(pickField(stream{}, houseMessages(), "What do you want to do now?", menuChoices(), "--what", &got))).
		WithShowHelp(false).WithTheme(theme(todaysStream()))
	return uitest.Run(uitest.FormScreen(f), width, 20).Frame
}

// helpFrame is a command tree's --help, as Execute writes it at the width.
func helpFrame(t *testing.T, width int, rich bool) string {
	t.Helper()
	// fang reads the process's CLICOLOR_FORCE to lay out and paint the help, until
	// the command tree is a block of its own; the session's Terminal says the rest.
	force := ""
	if rich {
		force = "1"
	}
	t.Setenv("CLICOLOR_FORCE", force)
	return printed(t, width, rich, func(scr SessionOption) {
		root := &cobra.Command{Use: "app", Short: "does a thing for the folder it is run in, and says what it changed", Run: func(*cobra.Command, []string) {}}
		root.Flags().Bool("loud", false, "make noise about every file the run reads or writes, one line each")
		root.Flags().String("folder", "", "the folder to work in, when it is not the one the run starts in")
		root.SetArgs([]string{"--help"})
		root.SetOut(os.Stdout)
		root.SetErr(os.Stdout)
		if err := Execute(context.Background(), root, runner(scr), Brand{}); err != nil {
			t.Error(err)
		}
	})
}

// refusalFrame is the refusal the frames draw, with its line to type, and the one whose text and JSON forms are goldens.
func refusalFrame() closing {
	return closing{word: "refused", what: "the folder is not one this tool made, so it will not touch it", next: "app init", code: 5}
}

// frames is every frame the narrow tests draw.
func frames() []frameDraw {
	return []frameDraw{
		{"first-run", func(t *testing.T, w int) string {
			t.Helper()
			return printed(t, w, true, func(scr SessionOption) {
				run := New(scr)
				run.Arrive(testBrand(), "v1.0.0")
				run.End(closing{word: "needs you", what: "no config yet, so nothing is set up for this folder", next: "app init --folder /some/long/path"})
			})
		}},
		{"help", func(t *testing.T, w int) string { t.Helper(); return helpFrame(t, w, true) }},
		{"menu", func(_ *testing.T, w int) string { return menuFrame(w) }},
		{"happy-path", func(t *testing.T, w int) string {
			t.Helper()
			return printed(t, w, true, func(scr SessionOption) {
				New(scr).End(closing{word: "done", what: "the plan was applied to three files in the project folder"})
			})
		}},
		{"refusal", func(t *testing.T, w int) string {
			t.Helper()
			return printed(t, w, true, func(scr SessionOption) {
				New(scr).End(refusalFrame())
			})
		}},
	}
}

// frameProblems is what is wrong with a frame drawn at width, against the golden drawn at 80:
// a row wider than the screen, a row cut off with an ellipsis, a word missing or out of order.
// The banner is decoration and drops its mark where there is none room, so a frame that opens
// with one is compared from its first row of words that is not part of it.
func frameProblems(frame string, width int, golden string) []string {
	var out []string
	for i, row := range strings.Split(frame, "\n") {
		if n := ansi.StringWidth(row); n > width {
			out = append(out, fmt.Sprintf("row %d is %d columns wide on a %d-column screen: %q", i+1, n, width, row))
		}
		if cut := strings.TrimRight(row, " "); strings.HasSuffix(cut, "…") || strings.HasSuffix(cut, "...") {
			out = append(out, fmt.Sprintf("row %d is cut off: %q", i+1, row))
		}
	}
	if got, want := words(frame), words(golden); !slices.Equal(got, want) {
		out = append(out, fmt.Sprintf("the words are not the 80-column golden's, in the same order:\n got %q\nwant %q", got, want))
	}
	return out
}

// words is the words of a frame in order, without the mark and the wordmark, which are
// decoration: the characters they are drawn with are block elements and box lines.
func words(frame string) []string {
	var out []string
	for w := range strings.FieldsSeq(frame) {
		if strings.Trim(w, "▀▄█▌▐◖◗┃ ") == "" {
			continue
		}
		out = append(out, w)
	}
	return out
}

// Each frame at 60 and at 40 fits its screen, loses nothing and keeps its order, against
// its golden at 80.
//
// What it costs when this fails: on a narrow terminal a row runs off the edge or a hint is
// cut off, and the person is told less than at 80 without being told so.
func TestTheFirstScreensFitSixtyAndFortyColumns(t *testing.T) { //nolint:paralleltest // serial: it sets the process's environment or directory
	for _, f := range frames() { //nolint:paralleltest // serial: draw sets CLICOLOR_FORCE for the process
		t.Run(f.name, func(t *testing.T) {
			at80 := f.draw(t, 80)
			uitest.Golden(t, "narrow-"+f.name+"-80", at80)
			for _, w := range narrowWidths()[1:] {
				for _, p := range frameProblems(f.draw(t, w), w, at80) {
					t.Errorf("%s at %d columns: %s", f.name, w, p)
				}
			}
		})
	}
	if len(frames()) != 5 {
		t.Fatalf("%d frames, and the issue names five", len(frames()))
	}
}

// A frame 70 columns wide is refused at 60, a row cut off is, a word lost or moved is, and the
// frame that is right is not.
func TestAFrameThatDoesNotFitIsRefused(t *testing.T) {
	t.Parallel()
	golden := "needs you  no config yet\n  app init\n"
	for _, c := range []struct {
		name, frame string
		width       int
		want        string
	}{
		{"a clean frame", "needs you  no config\n  yet\n  app init\n", 60, ""},
		{"a frame 70 columns wide", "needs you  no config yet" + strings.Repeat(" x", 23) + "\n  app init\n", 60, "is 70 columns wide on a 60-column screen"},
		{"a row cut off", "needs you  no config yet …\n  app init\n", 60, "is cut off"},
		{"a row cut off with three dots", "needs you  no config yet...\n  app init\n", 60, "is cut off"},
		{"a word lost", "needs you  no config\n  app init\n", 60, "the words are not the 80-column golden's"},
		{"two rows swapped", "  app init\nneeds you  no config yet\n", 60, "the words are not the 80-column golden's"},
	} {
		got := strings.Join(frameProblems(c.frame, c.width, golden), "\n")
		switch {
		case c.want == "" && got != "":
			t.Errorf("%s: a right frame is refused: %s", c.name, got)
		case c.want != "" && !strings.Contains(got, c.want):
			t.Errorf("%s: want a problem saying %q, got %q", c.name, c.want, got)
		}
	}
}

// Plain mode (no colour, no terminal width but the one told) has a golden of each frame, and
// accessible mode has one of the menu as huh asks it as lines.
//
// What it costs when this fails: a script reading the plain output, or a person on a screen
// reader, is given a different text from the one that was reviewed.
func TestPlainAndAccessibleModesHaveGoldens(t *testing.T) { //nolint:paralleltest // serial: it sets the process's environment or directory
	for _, f := range []string{"first-run", "help", "happy-path", "refusal"} {
		var got string
		switch f {
		case "first-run":
			got = printed(t, 80, false, func(scr SessionOption) {
				run := New(scr)
				run.Arrive(testBrand(), "v1.0.0")
				run.End(closing{word: "needs you", what: "no config yet, so nothing is set up for this folder", next: "app init --folder /some/long/path"})
			})
		case "help":
			got = helpFrame(t, 80, false)
		case "happy-path":
			got = printed(t, 80, false, func(scr SessionOption) {
				New(scr).End(closing{word: "done", what: "the plan was applied to three files in the project folder"})
			})
		case "refusal":
			got = printed(t, 80, false, func(scr SessionOption) {
				New(scr).End(refusalFrame())
			})
		}
		if strings.Contains(got, "\x1b") {
			t.Errorf("plain %s has an escape in it: %q", f, got)
		}
		uitest.Golden(t, "plain-"+f, got)
	}
	var got string
	var out bytes.Buffer
	err := huh.NewForm(huh.NewGroup(pickField(stream{}, houseMessages(), "What do you want to do now?", menuChoices(), "--what", &got))).
		WithAccessible(true).WithInput(strings.NewReader("2\n")).WithOutput(&out).Run()
	if err != nil {
		t.Fatal(err)
	}
	if got != "b" {
		t.Errorf("answering 2 in accessible mode chose %q, and want b", got)
	}
	uitest.Golden(t, "accessible-menu", ansi.Strip(out.String()))
}

// huh draws the filter prompt, a "/", where a select's title goes, so a menu that filters from the
// first key never shows its title; the question is in the description instead, above the rows and
// above the line that names the flag. This holds that at the three widths: the question
// is on the screen whole and in the words, and the filter prompt is still the first row, since the
// menu still filters as a person types.
//
// What it costs when this fails: a person is shown a list with no question over it, and the words
// check at 60 and 40 cannot see the loss.
func TestTheMenuShowsItsQuestionBesideItsFilterPrompt(t *testing.T) {
	t.Parallel()
	for _, width := range []int{80, 60, 40} {
		frame := menuFrame(width)
		if !strings.Contains(strings.Join(strings.Fields(frame), " "), "What do you want to do now?") {
			t.Errorf("the menu at %d shows no question:\n%s", width, frame)
		}
		if first, _, _ := strings.Cut(frame, "\n"); !strings.HasSuffix(strings.TrimSpace(first), "/") {
			t.Errorf("the menu's first row at %d is %q, and was the filter prompt", width, first)
		}
	}
}
