package ui

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// envProbe is a bubbletea program that keeps the environment bubbletea hands it
// and quits.
type envProbe struct{ got *[]string }

func (p envProbe) Init() tea.Cmd { return nil }

func (p envProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if env, ok := msg.(tea.EnvMsg); ok {
		*p.got = slices.Clone(env)
		return p, tea.Quit
	}
	return p, nil
}

func (p envProbe) View() tea.View { return tea.NewView("") }

// envField is a question that keeps the environment huh's program hands it and
// quits.
type envField struct {
	huh.Field
	got *[]string
}

func (f *envField) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	if env, ok := msg.(tea.EnvMsg); ok {
		*f.got = slices.Clone(env)
		return f, tea.Quit
	}
	m, cmd := f.Field.Update(msg)
	if inner, ok := m.(huh.Field); ok {
		f.Field = inner
	}
	return f, cmd
}

// A screen and a question run under the environment the session's Terminal
// carries, which Detect sets to the env it read, and under an empty one where
// the Terminal carries none: never the process's.
//
// What it costs when this fails: a program that hands its session an
// environment, as a server does a request's, gets bubbletea and huh reading
// the process's TERM, and a test's screen depends on the shell it runs in.
func TestScreensAndQuestionsRunUnderTheTerminalsEnvironment(t *testing.T) {
	t.Parallel()
	process := os.Environ()
	if len(process) == 0 {
		t.Fatal("the process has no environment, so this proves nothing")
	}
	given := []string{"TERM=xterm-256color", "UI_ENV_PROBE=given"}
	if !slices.Equal(Detect(nil, nil, nil, given, StandardEnv()).Env, given) {
		t.Fatalf("Detect keeps no env: %v", Detect(nil, nil, nil, given, StandardEnv()).Env)
	}
	cases := 0
	for _, want := range [][]string{given, nil} {
		var out bytes.Buffer
		s := New(Out(&out), Err(&out), In(strings.NewReader("")), On(Terminal{Env: want}))

		var screen []string
		opts := append(s.teaStreams(), tea.WithContext(t.Context()))
		if _, err := tea.NewProgram(envProbe{got: &screen}, opts...).Run(); err != nil {
			t.Fatal(err)
		}
		var asked []string
		form := s.onStreams(huh.NewForm(huh.NewGroup(&envField{Field: huh.NewNote().Title("probe"), got: &asked})))
		if err := form.Run(); err != nil {
			t.Fatal(err)
		}

		for what, got := range map[string][]string{"a screen": screen, "a question": asked} {
			if !slices.Equal(got, want) {
				t.Errorf("%s ran under %d variables, want the Terminal's %q", what, len(got), want)
			}
		}
		cases++
	}
	if cases != 2 {
		t.Fatalf("ran %d cases, want 2", cases)
	}
}

// A question on a session given an error stream and no output is drawn on that
// error stream, where huh draws by default, and not on the process's.
//
// What it costs when this fails: a session that hands its questions an error
// stream of its own has them drawn on the process's stdout or stderr instead.
func TestAQuestionWithNoOutputIsDrawnOnTheSessionsErrorStream(t *testing.T) {
	t.Parallel()
	var errOut bytes.Buffer
	s := New(Err(&errOut), In(strings.NewReader("")))
	var asked []string
	form := s.onStreams(huh.NewForm(huh.NewGroup(&envField{Field: huh.NewNote().Title("probe"), got: &asked})))
	if err := form.Run(); err != nil {
		t.Fatal(err)
	}
	if errOut.Len() == 0 {
		t.Error("the question drew nothing on the session's error stream")
	}
}
