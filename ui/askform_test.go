package ui

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/huh/v2"

	"github.com/birdtable/go-console/uitest"
)

// regionForm is a form of the kind a program hands Ask.
func regionForm(got *string) *huh.Form {
	return huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Which region?").
			Options(huh.NewOption("north", "north"), huh.NewOption("south", "south")).
			Value(got)))
}

// Where nobody can answer, Ask runs nothing and the ending is the usage refusal
// naming every flag that answers the form.
//
// What it costs when this fails: a program in a pipe waits on a form nobody can
// fill in, or fails without saying what to pass.
func TestAskOffATerminalAsksNothingAndNamesTheFlags(t *testing.T) {
	t.Parallel()
	for name, opts := range map[string][]SessionOption{
		"a pipe": {On(Terminal{})},
		"json":   {On(Terminal{Interactive: true}), JSON()},
	} {
		var out bytes.Buffer
		var got string
		s := New(append(append(House(), opts...), Out(&out))...)
		e := s.Ask(regionForm(&got), "--region", "size")
		if e == nil || e.Code() != houseEnds().Code("usage") {
			t.Fatalf("%s: the ending was %v, want the usage refusal", name, e)
		}
		if !strings.Contains(e.What(), "no terminal here") {
			t.Errorf("%s: the refusal was %q, and does not say nobody can answer", name, e.What())
		}
		if next := e.Next(); !strings.Contains(next, "--region") || !strings.Contains(next, "--size") {
			t.Errorf("%s: the line to type is %q, and does not name both flags", name, next)
		}
		if out.Len() != 0 {
			t.Errorf("%s: it drew %q", name, out.String())
		}
	}
}

// In accessible mode Ask asks plain lines: nothing drawn, no escape byte in the
// label, and the answer typed is the answer kept.
func TestAskAccessibleAsksPlainLines(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	var got string
	s := New(On(Terminal{Interactive: true, Accessible: true}), In(strings.NewReader("2\n")), Out(&out))
	if e := s.Ask(regionForm(&got), "--region"); e != nil {
		t.Fatalf("the ending was %v", e)
	}
	if got != "south" {
		t.Errorf("answered %q, want south", got)
	}
	if !strings.Contains(out.String(), "Which region?") || strings.ContainsRune(out.String(), 0x1b) {
		t.Errorf("accessible output was %q", out.String())
	}
}

// At a screen the form is the one the program built: Ask runs the same form
// uitest walks, so what a test sees is what a person is shown, and a pick keeps.
func TestAFormAskRunsIsTheOneAScreenWalks(t *testing.T) {
	t.Parallel()
	var got string
	f := regionForm(&got)
	frame := uitest.Run(uitest.FormScreen(regionForm(new(string))), 60, 12).Frame
	uitest.Golden(t, "ask-form", frame)
	if !strings.Contains(frame, "Which region?") {
		t.Errorf("the form shown was:\n%s", frame)
	}
	uitest.Run(uitest.FormScreen(f), 60, 12, "down", "enter")
	if got != "south" {
		t.Errorf("down, enter kept %q, want south", got)
	}
}
