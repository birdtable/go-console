package ui

// The live view's frame is the caller's: ui draws the body and keeps the mechanism, and says no word of the frame.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"
	"time"
)

// HouseFrame is the frame the first program drew, given by the caller as any program's frame is. It is exported
// from this test file so the tests of package ui_test draw the same one.
func HouseFrame(v LiveView) string {
	s := New()
	var b strings.Builder
	if v.Rows {
		b.WriteString(v.Body + "\n")
		b.WriteString("  " + s.Note(houseReadLine(v)) + "\n\n")
	} else {
		b.WriteString(v.Body + "\n\n")
	}
	if v.Selected != nil {
		b.WriteString("  " + s.Note("selected") + " " + s.Bold(v.Selected.ID) + "\n")
	}
	if t := v.Typing; t != nil {
		b.WriteString("  " + fmt.Sprintf("type %s to %s it: %s", s.Bold(t.Item.Name), t.Key.Help, t.Typed) + "\n")
	} else if v.Note != "" {
		b.WriteString("  " + v.Note + "\n")
	}
	hints := []string{"↑/↓ choose"}
	for _, k := range v.Keys {
		hints = append(hints, k.Key+" "+k.Help)
	}
	b.WriteString("  " + s.Note(strings.Join(append(hints, "q leave"), " · ")))
	return b.String()
}

func houseReadLine(v LiveView) string {
	if v.ReadAt.IsZero() {
		if v.ReadErr != nil {
			return fmt.Sprintf("could not read (%v): nothing has been read yet", v.ReadErr)
		}
		return "nothing has been read yet"
	}
	at := v.ReadAt.Format(time.TimeOnly)
	if v.ReadErr != nil {
		return fmt.Sprintf("could not read (%v): these rows were read at %s", v.ReadErr, at)
	}
	return fmt.Sprintf("read at %s", at)
}

// A live view with no Frame is its body alone: no heading, no selected row, no keys named, no footer.
//
// What it costs when this fails: a word of ui's own is on a screen whose program drew a frame of its own, or
// chose none.
func TestALiveViewWithNoFrameIsItsBodyAlone(t *testing.T) {
	t.Parallel()
	m := newLiveModelAt(New(WithMessages(houseMessages())), Live{
		Draw: func() []LiveItem { return []LiveItem{{ID: "ac11"}} },
		Keys: []LiveKey{{Key: "s", Help: "stop"}},
	}, noon)
	if got := m.Frame(); got != "" {
		t.Errorf("a screen that printed nothing and drew no frame is %q", got)
	}
	r := newLiveModelAt(New(WithMessages(houseMessages())), Live{
		Columns: liveCols(),
		Rows:    func() ([][]string, error) { return [][]string{{"ac11", "up"}}, nil },
	}, noon)
	got := r.Frame()
	if !strings.Contains(got, "ac11") || strings.Contains(got, "12:00") || strings.Contains(got, "q ") {
		t.Errorf("a table with no frame is its rows alone, got:\n%s", got)
	}
}

// ui/live.go holds no word of English: every word of a live screen is the caller's. A key's name is not a word
// (enter, backspace) and is the one run of letters allowed.
//
// What it costs when this fails: a fixed word is back in the live view, and a program in another language
// shows it on its screen.
func TestLiveGoHoldsNoWord(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "live.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range wordsIn(t, fset, f) {
		t.Error(bad)
	}
}

var letterRun = regexp.MustCompile(`[A-Za-z]{2,}`)

// wordsIn names every string literal of f that holds a run of two letters or more, bar the names of keys.
func wordsIn(t *testing.T, fset *token.FileSet, f *ast.File) []string {
	t.Helper()
	keyNames := map[string]bool{`"enter"`: true, `"backspace"`: true, `"up"`: true, `"down"`: true}
	var out []string
	seen := 0
	ast.Inspect(f, func(n ast.Node) bool {
		if imp, ok := n.(*ast.ImportSpec); ok {
			_ = imp
			return false
		}
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			seen++
			if !keyNames[lit.Value] && letterRun.MatchString(lit.Value) {
				out = append(out, fmt.Sprintf("%s: a word in live.go: %s", fset.Position(lit.Pos()), lit.Value))
			}
		}
		return true
	})
	if seen < 5 {
		t.Fatalf("read %d strings of live.go, which is too few to be it", seen)
	}
	return out
}
