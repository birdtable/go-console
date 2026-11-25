package ui

import (
	"strings"
	"testing"

	"charm.land/huh/v2"

	"github.com/birdtable/go-console/uitest"
)

func TestPickFilters(t *testing.T) {
	t.Parallel()
	choices := []Choice{
		{Label: "alpha", Blurb: "the first", Value: "a"},
		{Label: "beta", Blurb: "the second", Value: "b"},
		{Label: "gamma", Blurb: "the third", Value: "c"},
	}
	var got string
	f := huh.NewForm(huh.NewGroup(pickField(stream{}, houseMessages(), "What now?", choices, "--what", &got)))
	s := uitest.FormScreen(f)
	all := uitest.Run(s, 60, 20)
	for _, want := range []string{"alpha", "beta", "gamma", "Or answer with --what"} {
		if !strings.Contains(all.Frame, want) {
			t.Errorf("%q is not offered:\n%s", want, all.Frame)
		}
	}
	narrow := uitest.Run(s, 60, 20, "gam")
	uitest.Golden(t, "pick-filtered", narrow.Frame)
	if !strings.Contains(narrow.Frame, "gamma") || strings.Contains(narrow.Frame, "alpha") || strings.Contains(narrow.Frame, "beta") {
		t.Errorf("typing gam left:\n%s", narrow.Frame)
	}
	uitest.Run(s, 60, 20, "enter")
	if got != "c" {
		t.Errorf("the filtered row picked %q", got)
	}
}

func TestAFlagIsShownAsItIsTyped(t *testing.T) {
	t.Parallel()
	for flag, want := range map[string]string{"region": "--region", "--region": "--region", "<id>": "<id>", "": ""} {
		if got := flagText(flag); got != want {
			t.Errorf("%q was shown as %q, want %q", flag, got, want)
		}
	}
}

// The hint under a pick names the flag in the words that fit it: a positional
// is "named on the command line", a flag is "answered with".
func TestThePickHintFitsTheFlag(t *testing.T) {
	t.Parallel()
	for flag, want := range map[string]string{
		"<id>":     "Or name it on the command line: <id>",
		"--region": "Or answer with --region",
		"region":   "Or answer with --region",
	} {
		var got string
		f := huh.NewForm(huh.NewGroup(pickField(stream{}, houseMessages(), "What?", []Choice{{Label: "a", Value: "a"}}, flag, &got)))
		if frame := uitest.Run(uitest.FormScreen(f), 60, 12).Frame; !strings.Contains(frame, want) {
			t.Errorf("flag %q: the hint %q is not in\n%s", flag, want, frame)
		}
	}
	var got string
	f := huh.NewForm(huh.NewGroup(pickField(stream{}, houseMessages(), "What?", []Choice{{Label: "a", Value: "a"}}, "", &got)))
	if frame := uitest.Run(uitest.FormScreen(f), 60, 12).Frame; strings.Contains(frame, "Or ") {
		t.Errorf("a pick with no flag has a hint:\n%s", frame)
	}
}
