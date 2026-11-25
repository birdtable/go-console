package ui_test

// A second consumer in another language: Messages given in Swedish, every screen it reaches drawn with
// no English of ui's left on it.
//
// What it costs when this fails: a sentence of ui's own is fixed in English again, and the second
// program's people read a language they did not choose.

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/birdtable/go-console/internal/english"
	"github.com/birdtable/go-console/ui"
	"github.com/birdtable/go-console/uitest"
)

func swedish() ui.Messages {
	return ui.Messages{
		PlainHelp: "ingen levande vy eller bläddrare, och frågor som vanliga rader", AccessibleHelp: "frågor som vanliga rader för en skärmläsare",
		YesHelp: "svara ja på varje fråga före en ändring", JSONHelp: "ett JSON-dokument på stdout, resten på stderr", NoColorHelp: "ingen färg, som NO_COLOR ber om",
		Yes: "Ja", No: "Nej", AnsweredYes: "ja", AnsweredNo: "nej",
		AnsweredYesBy: "ja, genom %s", AnsweredYesAdvance: "ja, på förhand", AnsweredYesTyped: "ja, skrev %s", YesFlagName: "programmets ja",
		TypeNameTitle: "Skriv %s för att %s", TypeNameAdvance: "Eller svara på förhand med %s", TypeNameExactly: "skriv %s precis, eller tryck escape för att lämna det",
		NobodyToAsk: "detta skulle %s, och här finns ingen terminal att fråga i", CouldNotAsk: "detta skulle %s, men frågan kunde inte ställas (%s)",
		CouldNotAskNothingDone: "%s, så ingenting gjordes",
		FormNoTerminal:         "detta behöver svar, och här finns ingen terminal att fråga i", FormNotAnswered: "ingenting besvarades", FormCouldNot: "frågorna kunde inte ställas (%s)",
		DryWould:           "skulle %s",
		LiveNothingChanged: "ingenting ändrades", LiveNotTheName: "det är inte namnet, så ingenting ändrades", LiveNothingSelected: "ingenting är valt",
		LiveNothingToStepInto: "inget att gå in i", LiveNoName: "raden har inget namn att skriva, så ingenting ändrades", LiveBack: "tillbaka från %s",
		HelpArgs: "[ämne eller kommando]", HelpShort: "Hjälp om ett kommando", HelpUnknownTopic: "Okänt hjälpämne %#q",
		NameOnCommandLine: "Eller ange det på kommandoraden: %s", AnswerWith: "Eller svara med %s",
		MenuCouldNotShow: "kunde inte visa menyn (%s), så här är hjälpen i stället", CouldNotAskFor: "kunde inte fråga efter %s (%s)", MenuNothingPicked: "inget valdes",
		InternalError:   "kommandot stoppade på ett internt fel",
		FollowFollowing: "följer", FollowPaused: "pausad, slutet följer", FollowKeys: " · ↑/↓ rulla · ctrl-c lämna", PageKeys: "%3.0f%% · ↑/↓ rulla · q lämna",
		HelpFlagHelp: "hjälp för %s", HelpCommandArgs: "[kommando]", HelpAnyShort: "Hjälp om vilket kommando som helst", VersionLine: "version %s",
	}
}

// swedishScreens is every screen of ui's own that the second program reaches, drawn in its language.
func swedishScreens(t *testing.T) string {
	t.Helper()
	var all strings.Builder
	for _, args := range [][]string{{"--help"}, {"put"}, {"drop"}, {"drop", "--ja"}, {"wipe"}, {"put", "--help"}, {"help"}, {"help", "nope"}} {
		ends := otherEnds(t)
		var buf bytes.Buffer
		run := ui.New(ends.Endings(), ui.WithMessages(swedish()),
			ui.FlagNames(ui.Flags{Plain: "bare", Yes: "ja", JSON: "machine"}),
			ui.On(ui.Terminal{}), ui.Out(&buf), ui.Err(&buf))
		root := &cobra.Command{Use: "other", Short: "ett andra program", SilenceUsage: true, SilenceErrors: true}
		root.AddCommand(&cobra.Command{Use: "put", Short: "lägg något", RunE: func(*cobra.Command, []string) error {
			run.Change("sak till hylla")
			return run.End(ends.Done("la en sak"))
		}})
		root.AddCommand(&cobra.Command{Use: "drop", Short: "släpp något", RunE: func(*cobra.Command, []string) error {
			ok, e := run.Confirm("släppa allt", "", "--ja")
			if e != nil {
				return run.End(e)
			}
			_ = ok
			return run.End(ends.Done("klart"))
		}})
		root.AddCommand(&cobra.Command{Use: "wipe", Short: "radera", RunE: func(*cobra.Command, []string) error {
			_, e := run.ConfirmName("radera hyllan", "hylla", "--bekräfta hylla")
			return run.End(e)
		}})
		root.SetArgs(args)
		root.SetOut(&buf)
		root.SetErr(&buf)
		_ = ui.Execute(context.Background(), root, run, ui.Brand{Name: "other"})
		all.WriteString("## other " + strings.Join(args, " ") + "\n" + buf.String() + "\n")
	}

	run := ui.New(otherEnds(t).Endings(), ui.WithMessages(swedish()), ui.On(ui.Terminal{}))

	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	var fail error
	d := run.NewLiveDriver(ui.Live{
		Frame:   swedishFrame,
		Columns: []ui.TableColumn{{Title: "id", Priority: 9}, {Title: "läge", Priority: 5}},
		Rows: func() ([][]string, error) {
			if fail != nil {
				return nil, fail
			}
			return [][]string{{"ac11", "igång"}}, nil
		},
		Keys: []ui.LiveKey{
			{Key: "e", Help: "gå in", Exec: func(ui.LiveItem) *exec.Cmd { return exec.CommandContext(t.Context(), "other") }},
			{Key: "x", Help: "ta bort", Name: true, Act: func(ui.LiveItem) string { return "borttagen" }},
		},
	}, func() time.Time { return now }, func(*exec.Cmd) error { return errors.New("slutade med 2") })
	for _, keys := range [][]string{{"e"}, {"x", "escape"}, {"x", "q", "enter"}} {
		fail = nil
		res := uitest.Run(uitest.LiveScreen(d), 60, 12, keys...)
		all.WriteString("## live " + strings.Join(keys, " ") + "\n" + res.Frame + "\n")
	}
	fail = errors.New("nätverket")
	all.WriteString("## live failed read\n" + uitest.Run(uitest.LiveScreen(d), 60, 12).Frame + "\n")
	return all.String()
}

func TestSecondConsumerInAnotherLanguageHasNoEnglishLeft(t *testing.T) { //nolint:paralleltest // serial: Execute sets the process's streams.
	got := swedishScreens(t)
	// Every fixed run of words of the English set, apart from a word the program also says itself.
	v := reflect.ValueOf(english.Messages())
	for i := range v.NumField() {
		for _, seg := range strings.FieldsFunc(v.Field(i).String(), func(r rune) bool { return r == '%' }) {
			seg = strings.TrimLeft(seg, "svd")
			if len(strings.Fields(seg)) < 2 || len(seg) < 12 {
				continue
			}
			if strings.Contains(got, seg) {
				t.Errorf("English of ui's own is left on the second program's screens: %q (%s)\n%s", seg, v.Type().Field(i).Name, got)
			}
		}
	}
	for _, word := range []string{"would ", " choose", " leave", "Nothing was", "selected "} {
		if strings.Contains(got, word) {
			t.Errorf("English of ui's own is left on the second program's screens: %q\n%s", word, got)
		}
	}
	uitest.Golden(t, "secondconsumer-sv", got)
}

// Execute refuses a session without Messages, or with a message left empty, and names which.
func TestExecuteRefusesASessionWithNoMessages(t *testing.T) { //nolint:paralleltest // serial: Execute sets the process's streams.
	for name, c := range map[string]struct {
		msgs ui.Messages
		give bool
		want string
	}{
		"none":  {give: false, want: "Messages"},
		"empty": {msgs: english.Messages(), give: true, want: "PlainHelp"},
	} {
		if name == "empty" {
			c.msgs.PlainHelp = ""
		}
		opts := []ui.SessionOption{otherEnds(t).Endings(), ui.FlagNames(ui.Flags{Plain: "bare"})}
		if c.give {
			opts = append(opts, ui.WithMessages(c.msgs))
		}
		ran := false
		root := &cobra.Command{Use: "other", RunE: func(*cobra.Command, []string) error { ran = true; return nil }}
		err := ui.Execute(context.Background(), root, ui.New(opts...), ui.Brand{Name: "other"})
		if err == nil || !strings.Contains(err.Error(), c.want) || ran {
			t.Errorf("%s: Execute returned %v and ran %v, want a refusal naming %s", name, err, ran, c.want)
		}
	}
}

// swedishFrame is the second program's own live frame, drawn in its words: ui supplies none.
func swedishFrame(v ui.LiveView) string {
	s := ui.New()
	var b strings.Builder
	if v.Rows {
		read := "inget har lästs än"
		at := v.ReadAt.Format(time.TimeOnly)
		switch {
		case v.ReadAt.IsZero() && v.ReadErr != nil:
			read = "kunde inte läsa (" + v.ReadErr.Error() + "): inget har lästs än"
		case v.ReadErr != nil:
			read = "kunde inte läsa (" + v.ReadErr.Error() + "): raderna lästes " + at
		case !v.ReadAt.IsZero():
			read = "läst " + at
		}
		b.WriteString(v.Body + "\n  " + s.Note(read) + "\n\n")
	} else {
		b.WriteString(v.Body + "\n\n")
	}
	if v.Selected != nil {
		b.WriteString("  " + s.Note("vald") + " " + s.Bold(v.Selected.ID) + "\n")
	}
	if t := v.Typing; t != nil {
		b.WriteString("  skriv " + s.Bold(t.Item.Name) + " för att " + t.Key.Help + " det: " + t.Typed + "\n")
	} else if v.Note != "" {
		b.WriteString("  " + v.Note + "\n")
	}
	hints := []string{"↑/↓ välj"}
	for _, k := range v.Keys {
		hints = append(hints, k.Key+" "+k.Help)
	}
	b.WriteString("  " + s.Note(strings.Join(append(hints, "q lämna"), " · ")))
	return b.String()
}
