package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// ui names no kind of ending: a kind is a name the program gave exit.New,
// and ui reaches one only through EndKinds. A non-test file that asks the table for a kind by its method,
// or names exit's old codes and words, builds a run's ending on a kind the program may not have.
//
// What it costs when this fails: a program whose table has no such kind gets the zero ending, which
// prints nothing, from a refusal or a failure ui meant to say.
func TestUINamesNoKindOfEnding(t *testing.T) {
	t.Parallel()
	names := regexp.MustCompile(`(?:table\(\)|\.ends|\bexit)\.(?:Done|Findings|NeedsYou|StoppedAt|Refused|Failed|Usage|Busy|NotReady|Declined|Codes|Words)\b`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if filepath.Ext(f) != ".go" || len(f) > 8 && f[len(f)-8:] == "_test.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range names.FindAll(src, -1) {
			t.Errorf("%s names a kind of ending: %s", f, m)
		}
	}
	// An exit code is the program's too: a number compared with an ending's code, or a case of one, names a kind.
	codes := regexp.MustCompile(`(?:\.Code\(\)|\bcase c|\bc) *(?:==|!=|<=|>=|<|>) *[1-9]|case c == [1-9]|case [1-9][0-9]*, c == `)
	for _, f := range files {
		if len(f) > 8 && f[len(f)-8:] == "_test.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range codes.FindAll(src, -1) {
			t.Errorf("%s names an exit code other than 0, the one every shell reads as done: %s", f, m)
		}
	}
	if len(files) == 0 {
		t.Fatal("no files read")
	}
}

// A table that does not hold a kind EndKinds names is refused before any command runs, naming the field.
func TestEndKindsNamingNoKindOfTheTableIsRefused(t *testing.T) {
	t.Parallel()
	k := houseEndKinds()
	k.Busy = "occupied"
	run := New(Endings(houseEnds().Table, k), WithMessages(houseMessages()))
	err := run.endsErr()
	if err == nil || !regexp.MustCompile(`EndKinds\.Busy is "occupied"`).MatchString(err.Error()) {
		t.Errorf("endsErr = %v, want a refusal naming EndKinds.Busy", err)
	}
	if err := New(Endings(houseEnds().Table, houseEndKinds())).endsErr(); err != nil {
		t.Errorf("the house kinds are refused: %v", err)
	}
}
