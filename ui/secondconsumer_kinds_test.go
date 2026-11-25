package ui_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/birdtable/go-run/exit"

	"github.com/birdtable/go-console/internal/english"
	"github.com/birdtable/go-console/ui"
)

// A second program with eight kinds of its own: names, words and codes unlike the house's, among
// them a "needs you" that leaves on a code and a "stopped at" that leaves on 0. ui ends on only three of them, and
// EndKinds.Roles gives a role to kinds ui never ends on. A run ended on each kind gives the kind's code and word
// back, prints its closing line on stderr for a code and on stdout for 0, and Paint gives the role Roles named,
// and none for a kind Roles leaves out.
//
// What it costs when this fails: a program with more kinds than ui's six fields cannot colour or end on its own
// kinds, and has to take the house's table to get a painted closing word.
func TestSecondConsumerEndsOnEightKindsOfItsOwn(t *testing.T) {
	t.Parallel()
	kinds := []struct {
		kind exit.Kind
		role ui.EndRole
		set  bool
	}{
		{exit.Kind{Name: "landed", Word: "landed", Code: 0}, ui.EndGood, true},
		{exit.Kind{Name: "crashed", Word: "fell over", Code: 70}, ui.EndBad, true},
		{exit.Kind{Name: "misread", Word: "not understood", Code: 64}, ui.EndBad, true},
		{exit.Kind{Name: "needs-you", Word: "needs you", Code: 77}, ui.EndAttn, true},
		{exit.Kind{Name: "stopped-at", Word: "stopped at", Code: 0}, ui.EndAttn, true},
		{exit.Kind{Name: "elsewhere", Word: "in use elsewhere", Code: 75}, ui.EndAttn, true},
		{exit.Kind{Name: "absent", Word: "not there", Code: 69}, ui.EndPlain, true},
		{exit.Kind{Name: "partial", Word: "partly done", Code: 9}, ui.EndPlain, false},
	}
	var table []exit.Kind
	roles := map[string]ui.EndRole{}
	for _, k := range kinds {
		table = append(table, k.kind)
		if k.set {
			roles[k.kind.Name] = k.role
		}
	}
	tab, err := exit.New(table...)
	if err != nil {
		t.Fatal(err)
	}
	names := ui.EndKinds{Failed: "crashed", Usage: "misread", Declined: "misread", Busy: "elsewhere", NotReady: "elsewhere", Refused: "misread", Roles: roles}

	// The escapes each role paints in, read from a session whose three kinds share no name with the eight.
	ref, err := exit.New(exit.Kind{Name: "g", Word: "g", Code: 0}, exit.Kind{Name: "a", Word: "a", Code: 1}, exit.Kind{Name: "b", Word: "b", Code: 2})
	if err != nil {
		t.Fatal(err)
	}
	term := ui.Terminal{Colour: true, ErrColour: true, Width: 80}
	refRun := ui.New(ui.Endings(ref, ui.EndKinds{Failed: "b", Usage: "b", Declined: "a", Busy: "a", NotReady: "a", Refused: "b",
		Roles: map[string]ui.EndRole{"g": ui.EndGood, "a": ui.EndAttn, "b": ui.EndBad}}), ui.On(term))
	painted := map[ui.EndRole]string{
		ui.EndPlain: "x",
		ui.EndGood:  refRun.Paint(ref.End("g", "", ""), "x"),
		ui.EndAttn:  refRun.Paint(ref.End("a", "", ""), "x"),
		ui.EndBad:   refRun.Paint(ref.End("b", "", ""), "x"),
	}
	if painted[ui.EndGood] == painted[ui.EndAttn] || painted[ui.EndAttn] == painted[ui.EndBad] || painted[ui.EndGood] == painted[ui.EndBad] || !strings.Contains(painted[ui.EndAttn], "\x1b[") {
		t.Fatalf("the three roles do not paint apart: %v", painted)
	}

	for _, k := range kinds {
		var stdout, stderr bytes.Buffer
		run := ui.New(ui.Endings(tab, names), ui.WithMessages(english.Messages()), ui.On(term), ui.Out(&stdout), ui.Err(&stderr))
		e := tab.End(k.kind.Name, "the thing", "")
		_ = run.End(e)
		ended := run.Ended()
		if ended == nil || run.ExitCode() != k.kind.Code || ended.Word() != k.kind.Word {
			t.Errorf("%s: the run ended on %v with code %d, want %q and %d", k.kind.Name, ended, run.ExitCode(), k.kind.Word, k.kind.Code)
			continue
		}
		if got := run.Paint(ended, "x"); got != painted[k.role] {
			t.Errorf("%s: Paint gave %q, want the role %d, %q", k.kind.Name, got, k.role, painted[k.role])
		}
		on, off := &stdout, &stderr
		if k.kind.Code != 0 {
			on, off = off, on
		}
		if !strings.Contains(on.String(), k.kind.Word) || off.Len() != 0 {
			t.Errorf("%s (code %d): stdout %q, stderr %q; want the closing line on the one stream its code names", k.kind.Name, k.kind.Code, stdout.String(), stderr.String())
		}
		if k.role != ui.EndPlain && !strings.Contains(on.String(), painted[k.role][:strings.Index(painted[k.role], "x")]) {
			t.Errorf("%s: the closing line %q is not in its role's colour", k.kind.Name, on.String())
		}
	}
}
