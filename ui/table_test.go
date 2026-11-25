package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/birdtable/go-console/uitest"
)

func testCols() []TableColumn {
	return []TableColumn{
		{Title: "name", Priority: 9},
		{Title: "state", Priority: 5},
		{Title: "address", Priority: 2},
		{Title: "note", Priority: 1},
	}
}

func testRows() [][]string {
	return [][]string{
		{"alpha", "running", "192.0.2.10", "a note that runs on a while longer than the rest"},
		{"beta", "stopped", "192.0.2.11", "short"},
		{"gamma-with-a-long-name", "running", "192.0.2.12", "none"},
	}
}

func lines(s string) []string { return strings.Split(ansi.Strip(s), "\n") }

// At each width no line is wider than the screen, and what goes first is the
// lowest priority, the last one standing the highest.
func TestTableAtThreeWidths(t *testing.T) {
	t.Parallel()
	for _, rich := range []bool{false, true} {
		var dropped []int
		for _, w := range []int{80, 60, 40} {
			dropped = append(dropped, checkTableAt(t, rich, w))
		}
		if len(dropped) != 3 || dropped[0] > dropped[1] || dropped[1] > dropped[2] || dropped[2] <= dropped[0] {
			t.Errorf("rich %v: dropped %v columns at 80, 60 and 40; a narrower screen should lose more", rich, dropped)
		}
	}
}

// checkTableAt draws the test table at one width, checks it and returns how
// many columns it dropped.
func checkTableAt(t *testing.T, rich bool, w int) int {
	t.Helper()
	at := New(screenOpt(rich, w))
	out := at.Table(testCols(), testRows())
	ls := lines(out)
	if len(ls) < 4 {
		t.Fatalf("rich %v width %d: %d lines:\n%s", rich, w, len(ls), out)
	}
	for _, l := range ls {
		if cells(l) > w {
			t.Errorf("rich %v width %d: a line is %d cells: %q", rich, w, cells(l), l)
		}
	}
	head := ls[0]
	if !strings.Contains(head, "name") {
		t.Errorf("rich %v width %d: the highest priority column is gone: %q", rich, w, head)
	}
	gone := 0
	for _, c := range testCols() {
		if strings.Contains(head, c.Title) {
			continue
		}
		gone++
		for _, other := range testCols() {
			if other.Priority < c.Priority && strings.Contains(head, other.Title) {
				t.Errorf("rich %v width %d: %q went before %q", rich, w, c.Title, other.Title)
			}
		}
	}
	return gone
}

// A single column wider than the screen wraps onto lines under it, not dropped.
func TestTheLastColumnStandingIsCut(t *testing.T) {
	t.Parallel()
	at := New(screenOpt(false, 10))
	out := at.Table([]TableColumn{{Title: "name"}}, [][]string{{"a name far too long for ten cells"}})
	for _, l := range lines(out) {
		if cells(l) > 10 {
			t.Errorf("%q is %d cells", l, cells(l))
		}
	}
	if len(lines(out)) < 3 {
		t.Errorf("a long cell did not wrap:\n%s", out)
	}
	if strings.Contains(out, "…") || strings.Contains(out, "not shown") {
		t.Errorf("a wrapped cell is neither cut nor a dropped column:\n%s", out)
	}
	if joined := strings.Join(strings.Fields(out), " "); joined != "name a name far too long for ten cells" {
		t.Errorf("wrapping lost words: %q", joined)
	}
}

// Off a terminal, or with no colour, there is no box character and no escape
// sequence: a script reads the table as columns.
func TestTablePlainFace(t *testing.T) {
	t.Parallel()
	at := New(screenOpt(false, 0))
	out := at.Table(testCols(), testRows())
	if strings.ContainsAny(out, "\x1b│─┌┐└┘├┤┬┴┼╭╮╰╯") {
		t.Errorf("the plain table has decoration:\n%q", out)
	}
	ls := strings.Split(out, "\n")
	if len(ls) != 4 || !strings.HasPrefix(ls[1], "alpha ") || strings.HasSuffix(ls[1], " ") {
		t.Errorf("plain table:\n%s", out)
	}
	// Every row has its cells in the same columns.
	col := strings.Index(ls[0], "state")
	for _, l := range ls[1:] {
		if col < 0 || len(l) <= col || l[col-1] != ' ' {
			t.Errorf("the state column is not aligned in %q (header %q)", l, ls[0])
		}
	}
}

// A cell from outside shows its control characters as text, so a name cannot
// start a line of its own or paint the screen.
func TestATableCellCannotInjectALine(t *testing.T) {
	t.Parallel()
	at := New(screenOpt(false, 0))
	out := at.Table([]TableColumn{{Title: "name"}}, [][]string{{"a\nforged  done\x1b[31m"}})
	if len(strings.Split(out, "\n")) != 2 || strings.Contains(out, "\x1b") {
		t.Errorf("%q", out)
	}
}

func TestATableWithNoColumnsIsEmpty(t *testing.T) {
	t.Parallel()
	if got := New().Table(nil, [][]string{{"x"}}); got != "" {
		t.Errorf("%q", got)
	}
}

// A row short of cells is padded, and the table at a terminal has the same
// words as the plain one.
func TestARichTableSaysWhatThePlainOneSays(t *testing.T) {
	t.Parallel()
	rows := [][]string{{"a"}, {"b", "c"}}
	cols := []TableColumn{{Title: "x"}, {Title: "y"}}
	at := New(screenOpt(false, 80))
	plain := strings.Fields(at.Table(cols, rows))
	at = New(screenOpt(true, 80))
	rich := strings.Fields(ansi.Strip(at.Table(cols, rows)))
	// the rich one draws a rule under the header
	var words []string
	for _, w := range rich {
		if strings.Trim(w, "─") != "" {
			words = append(words, w)
		}
	}
	if len(plain) == 0 || strings.Join(plain, " ") != strings.Join(words, " ") {
		t.Errorf("plain %q, rich %q", plain, words)
	}
}

func TestListFaces(t *testing.T) {
	t.Parallel()
	at := New(screenOpt(false, 0))
	if got := at.List("one", "two\nforged"); got != "- one\n- two\\nforged" {
		t.Errorf("plain list %q", got)
	}
	if at.List() != "" {
		t.Error("an empty list is not empty")
	}
	at = New(screenOpt(true, 80))
	out := ansi.Strip(at.List("one", "two"))
	if !strings.Contains(out, "one") || !strings.Contains(out, "two") || strings.Contains(out, "- ") {
		t.Errorf("rich list:\n%s", out)
	}
}

// Columns of the same priority go from the right, so the leftmost, which a
// reader looks at first, stays. Flipping the tie-break fails this.
func TestEqualPrioritiesDropFromTheRight(t *testing.T) {
	t.Parallel()
	cols := []TableColumn{{Title: "aaaaaa"}, {Title: "bbbbbb"}, {Title: "cccccc"}}
	rows := [][]string{{"1", "2", "3"}}
	for _, rich := range []bool{false, true} {
		at := New(screenOpt(rich, 15))
		head := lines(at.Table(cols, rows))[0]
		if !strings.Contains(head, "aaaaaa") || !strings.Contains(head, "bbbbbb") || strings.Contains(head, "cccccc") {
			t.Errorf("rich %v: kept %q; want the first two", rich, head)
		}
	}
}

// A cell holding ESC is shown as text at a terminal too: what the rich face
// strips of styling must not be the cell's own sequence.
func TestARichCellHoldingEscapeShowsItAsText(t *testing.T) {
	t.Parallel()
	at := New(screenOpt(true, 80))
	cell := "a\x1b[31mred"
	out := at.Table([]TableColumn{{Title: "name"}}, [][]string{{cell}})
	if want := Name(cell); want == cell || !strings.Contains(ansi.Strip(out), want) {
		t.Errorf("the cell was %q, the table says\n%q", want, ansi.Strip(out))
	}
	if strings.Contains(out, cell) {
		t.Errorf("the cell's own sequence reached the screen: %q", out)
	}
}

// The widest cell of a column fills it, and the next column starts two cells
// after it all the same: Column's safety space, for a label that fills its
// width, once pushed that row's second column out by one.
func TestAPlainTableAlignsWhenTheWidestCellFillsItsColumn(t *testing.T) {
	t.Parallel()
	want := cells("gamma-with-a-long-name") + 2
	for _, w := range []int{80, 60, 40} {
		ls := lines(New(screenOpt(false, w)).Table(testCols(), testRows()))
		if len(ls) < 4 {
			t.Fatalf("width %d: %d lines:\n%s", w, len(ls), strings.Join(ls, "\n"))
		}
		at := strings.Index(ls[0], "state")
		if at != want {
			t.Errorf("width %d: the second column starts at %d, want %d:\n%s", w, at, want, strings.Join(ls, "\n"))
		}
		for _, l := range ls[1:] {
			if got := strings.Index(l, "running"); got >= 0 && got != at {
				t.Errorf("width %d: a row's second column is at %d, its header's at %d: %q", w, got, at, l)
			}
			if got := strings.Index(l, "stopped"); got >= 0 && got != at {
				t.Errorf("width %d: a row's second column is at %d, its header's at %d: %q", w, got, at, l)
			}
		}
		for _, l := range ls {
			if l != strings.TrimRight(l, " ") {
				t.Errorf("width %d: a trailing space: %q", w, l)
			}
		}
	}
}

// A plain table fits the width it is given by COLUMNS, off a terminal and
// without --plain (issue 38): the width was once unknown there, and a table
// drew at its natural width whatever the screen.
func TestAPlainTableFitsCOLUMNSOffATerminal(t *testing.T) {
	t.Parallel()
	for _, w := range []int{80, 60, 40} {
		at := detected("COLUMNS=" + strconv.Itoa(w))
		ls := lines(at.Table(testCols(), testRows()))
		if len(ls) < 4 {
			t.Fatalf("width %d: %d lines", w, len(ls))
		}
		for _, l := range ls {
			if cells(l) > w {
				t.Errorf("COLUMNS=%d: a line is %d cells: %q", w, cells(l), l)
			}
		}
		// A wrapped cell has lines of its own under it, so the first columns are
		// found line by line, in order.
		firsts := []string{"name", "alpha", "beta", "gamma-with-a-long-name"}
		next := 0
		for _, l := range ls {
			if next < len(firsts) && strings.HasPrefix(l, firsts[next]) {
				next++
			}
		}
		if next != len(firsts) {
			t.Errorf("COLUMNS=%d: a first column was lost:\n%s", w, strings.Join(ls, "\n"))
		}
	}
}

// tableSets are five data sets the plain table prints: ASCII, accents and emoji, rows short of
// cells, no rows, and cells longer than any screen.
func tableSets() []struct {
	name string
	cols []TableColumn
	rows [][]string
} {
	long := strings.Repeat("a long cell that goes on ", 8)
	return []struct {
		name string
		cols []TableColumn
		rows [][]string
	}{
		{"ascii", testCols(), testRows()},
		{"accents and emoji", testCols(), [][]string{
			{"café", "prêt", "192.0.2.10", "naïve résumé"},
			{"日本語", "🚀 go", "192.0.2.11", "é combined"},
			{"zoë 🎉🎉", "done ✅", "::1", "ok"},
		}},
		{"short rows", testCols(), [][]string{{"alpha"}, {"beta", "stopped"}, {"gamma", "running", "192.0.2.12"}}},
		{"empty", testCols(), nil},
		{"long cells", testCols(), [][]string{{long, long, long, long}, {"x", "y", "z", long}}},
	}
}

// The plain table prints the same text on five data sets at six widths. The golden was written by
// the table's own plain printer, before it was replaced by lipgloss's table with the header rule
// off, stripped and right-trimmed; it is the proof the replacement prints the same text.
//
// What it costs when this fails: a script that reads the plain table reads other columns, or other
// padding, than it did.
func TestThePlainTablePrintsTheSameTextAtSixWidths(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	for _, set := range tableSets() {
		for _, w := range []int{0, 20, 40, 60, 80, 120} {
			out := New(screenOpt(false, w)).Table(set.cols, set.rows)
			b.WriteString("== " + set.name + " at " + strconv.Itoa(w) + "\n" + out + "\n")
			for l := range strings.SplitSeq(out, "\n") {
				if w > 0 && cells(l) > w {
					t.Errorf("%s at %d: a line is %d cells: %q", set.name, w, cells(l), l)
				}
				if l != strings.TrimRight(l, " ") {
					t.Errorf("%s at %d: a line has trailing spaces: %q", set.name, w, l)
				}
			}
		}
	}
	uitest.Golden(t, "plain-table-matrix", b.String())
}

// The table at 80 columns, three ways: it fits, one cell wraps, two columns go
// and a note line says which.
func TestTableGoldensAt80(t *testing.T) {
	t.Parallel()
	cols := testCols()
	wide := []TableColumn{{Title: "name", Priority: 9}, {Title: "state", Priority: 5}, {Title: "address", Priority: 2}, {Title: "note", Priority: 1}}
	long := strings.Repeat("padding ", 8) + "end"
	cases := []struct {
		name string
		cols []TableColumn
		rows [][]string
	}{
		{"table-fits-80", cols[:2], testRows()},
		{"table-wrap-80", cols, testRows()},
		{"table-dropped-80", wide, [][]string{{"alpha", "running", "192.0.2.10", long}, {"beta", long, "192.0.2.11", "x"}}},
	}
	for _, c := range cases {
		out := New(screenOpt(false, 80)).Table(c.cols, c.rows)
		for l := range strings.SplitSeq(out, "\n") {
			if cells(l) > 80 {
				t.Errorf("%s: a line is %d cells: %q", c.name, cells(l), l)
			}
		}
		uitest.Golden(t, c.name, out)
	}
}

// A word is never broken at a hyphen, at a terminal or off it; one too long for
// its column is cut where it stands.
func TestWrapCellNeverBreaksAtAHyphen(t *testing.T) {
	t.Parallel()
	got := wrapCell("one two-part-word three", 12)
	for _, l := range got {
		if strings.HasSuffix(l, "-") {
			t.Errorf("a line ends at a hyphen: %q", got)
		}
	}
	if want := "one|two-part-word|three"; strings.Join(got, "|") != want && cells(got[1]) <= 12 {
		t.Logf("wrapped as %q", got)
	}
	long := wrapCell("abcdefghij", 4)
	if strings.Join(long, "|") != "abcd|efgh|ij" {
		t.Errorf("a long word was cut as %q", long)
	}
	if got := wrapCell("", 5); len(got) != 1 || got[0] != "" {
		t.Errorf("empty text is %q", got)
	}
}

// The last column left wraps and nothing more is dropped, down to minLast.
func TestTheLastColumnLeftWrapsBeforeAnotherGoes(t *testing.T) {
	t.Parallel()
	cols := []TableColumn{{Title: "id", Priority: 9}, {Title: "text", Priority: 5}}
	rows := [][]string{{"1", strings.Repeat("word ", 12)}}
	out := New(screenOpt(false, 24)).Table(cols, rows)
	if strings.Contains(out, "not shown") {
		t.Errorf("a column went with %d cells left for the last:\n%s", 24-3, out)
	}
	if len(lines(out)) < 4 {
		t.Errorf("the last column did not wrap:\n%s", out)
	}
}

// A cell's escape byte is shown as text in the plain face, and a pipe has no rule or box.
func TestAPlainTableShowsEscapeAndHasNoRule(t *testing.T) {
	t.Parallel()
	out := New(screenOpt(false, 0)).Table([]TableColumn{{Title: "name"}, {Title: "b"}}, [][]string{{"a\x1b[31mred", "y"}})
	if strings.ContainsAny(out, "\x1b─│┌┐└┘") {
		t.Errorf("the plain table has an escape or a rule: %q", out)
	}
	if !strings.Contains(out, Name("a\x1b[31mred")) {
		t.Errorf("the escape is not shown as text: %q", out)
	}
	if len(lines(out)) != 2 {
		t.Errorf("a plain table is a header and its rows, got:\n%s", out)
	}
}

// With NoHeaderRule a painted table has its header row and its first body row adjacent, and is otherwise
// the table it was: the painted table without the option less its one rule line, byte for byte, so the
// columns, the header's weight and the colour are kept. Without the option the painted face keeps its
// rule, and the plain face is the same with it and without.
//
// What it costs when this fails: a program whose contract has no rule under a header draws one, or a
// program that never asked loses its rule, its columns or its header's colour.
func TestNoHeaderRulePaintsATableWithNothingUnderItsHeader(t *testing.T) {
	t.Parallel()
	cols, rows := testCols()[:2], testRows()
	with := New(screenOpt(true, 80), NoHeaderRule()).Table(cols, rows)
	without := New(screenOpt(true, 80)).Table(cols, rows)
	var kept []string
	rules := 0
	for l := range strings.SplitSeq(without, "\n") {
		if strings.Contains(l, "─") {
			rules++
			continue
		}
		kept = append(kept, l)
	}
	if rules != 1 {
		t.Fatalf("the painted table without the option has %d rule lines, want 1:\n%s", rules, without)
	}
	if with != strings.Join(kept, "\n") {
		t.Errorf("with the option the table is\n%q\nwant the table without it less its rule line\n%q", with, strings.Join(kept, "\n"))
	}
	ls := strings.Split(with, "\n")
	if len(ls) != 1+len(rows) || !strings.Contains(ls[0], "name") || !strings.Contains(ls[1], "alpha") {
		t.Fatalf("with the option the header and the first row are not adjacent:\n%s", with)
	}
	if !strings.Contains(ls[0], "\x1b[1") {
		t.Errorf("with the option the header lost its weight: %q", ls[0])
	}
	if a, b := New(screenOpt(false, 80), NoHeaderRule()).Table(cols, rows), New(screenOpt(false, 80)).Table(cols, rows); a != b {
		t.Errorf("the plain face changed with the option:\n%q\nwas\n%q", a, b)
	}
	uitest.Golden(t, "table-painted-80", without)
	uitest.Golden(t, "table-painted-no-rule-80", with)
}
