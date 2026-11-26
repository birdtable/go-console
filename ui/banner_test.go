package ui

// The banner, which is how somebody knows which tool they are talking to.
//
// It has to appear when a person is watching and disappear completely when
// anything else is reading, and it has to do the first of those exactly once.

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/birdtable/go-console/uitest"
)

// testBrand is a made-up program: a name, a two-row mark, a tagline.
func testBrand() Brand {
	return Brand{Name: "app", Mark: "◖  ▄██▄\n ▀▀███▌", MarkGap: 3, Row: "a made-up tagline"}
}

// Arriving says where you have arrived. This is the whole point, and it did not
// happen: the banner was drawn by `version` and nothing else, which is the one
// command you run when you already know what you are running.
func TestArrivingDrawsTheBanner(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	scr := screenOpt(true, 0)

	out := capture(t, &os.Stdout, func() { New(scr).Arrive(testBrand(), "v1.2.3") })
	if !strings.Contains(out, "a made-up tagline") {
		t.Errorf("no banner was drawn:\n%q", out)
	}
	if !strings.Contains(out, "v1.2.3") {
		t.Errorf("the banner does not say which build it is:\n%q", out)
	}
}

// Once per run, its session, whoever asks. Two callers want it, and `version` drew
// it twice before this.
func TestTheBannerIsDrawnAtMostOnce(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	scr := screenOpt(true, 0)

	run := New(scr)
	first := capture(t, &os.Stdout, func() { run.Arrive(testBrand(), "v1") })
	second := capture(t, &os.Stdout, func() { run.Arrive(testBrand(), "v1") })

	if !strings.Contains(first, "a made-up tagline") {
		t.Fatalf("the first call drew nothing, so this proves nothing:\n%q", first)
	}
	if strings.TrimSpace(second) != "" {
		t.Errorf("the banner was drawn a second time:\n%q", second)
	}
}

// The second caller still needs a true answer, or `version` prints its plain
// heading underneath a banner that is already on screen.
func TestASecondCallerIsToldThereIsABannerOnScreen(t *testing.T) {
	t.Parallel()
	scr := screenOpt(true, 0)

	run := New(scr)
	if !run.Arrive(testBrand(), "v1") {
		t.Fatal("the first call says it drew nothing")
	}
	if !run.Arrive(testBrand(), "v1") {
		t.Error("the second call says there is no banner, so a caller would print a heading under one")
	}
}

// Off a terminal it draws nothing and says so, which is what keeps a scheduled
// run's log file and a piped command's output clean.
func TestNoBannerWhereNobodyIsWatching(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams

	out := capture(t, &os.Stdout, func() {
		if New().Arrive(testBrand(), "v1") {
			t.Error("a redirect was told a banner had been drawn")
		}
	})
	if strings.TrimSpace(out) != "" {
		t.Errorf("something was drawn into a redirect:\n%q", out)
	}
}

// A width of 0 is unknown, not narrow. It is what every stream that is not a
// terminal reports, and what a forced-colour capture reports, and treating it
// as narrow meant the banner could not be captured for a document at all.
func TestAnUnknownWidthIsNotTreatedAsANarrowOne(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	scr := screenOpt(true, 0)
	if New(scr).Width() != 0 {
		t.Fatalf("the width is %d, so this is testing something else", New(scr).Width())
	}

	out := capture(t, &os.Stdout, func() { New(scr).Arrive(testBrand(), "v1") })
	if !strings.Contains(out, "a made-up tagline") {
		t.Errorf("an unknown width was treated as a narrow one:\n%q", out)
	}
}

// A terminal too narrow to hold it gets nothing rather than a wordmark in
// pieces.
func TestANarrowTerminalGetsNoBanner(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	at30 := onScreen(stream{rich: true, interactive: true, width: 30}, stream{rich: true}, false)
	out := capture(t, &os.Stdout, func() { New(at30).Arrive(testBrand(), "v1") })
	if strings.TrimSpace(out) != "" {
		t.Errorf("a 30-column terminal got a 26-column wordmark plus its tagline:\n%q", out)
	}
}

// At every width a terminal can have, the banner is whole or absent: no line
// of it holds anything but a row of the wordmark, so a tagline that wrapped
// under the name fails here. Counting lines, not measuring them, because Log
// reflows an overlong line into short ones that each fit (learned the hard
// way: a test of line widths passed against the broken cut-off).
func TestTheBannerIsWholeAtEveryWidth(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	for w := 30; w <= 100; w++ {
		at := onScreen(stream{rich: true, interactive: true, width: w}, stream{rich: true}, false)
		out := capture(t, &os.Stdout, func() { New(at).Arrive(testBrand(), "6f315a1") })
		var rows []string
		for line := range strings.SplitSeq(out, "\n") {
			if strings.TrimSpace(ansi.Strip(line)) != "" {
				rows = append(rows, line)
			}
		}
		if len(rows) != 0 && len(rows) != len(wordmarkOf(testBrand().Name)) {
			t.Errorf("width %d: %d lines, not %d:\n%s", w, len(rows), len(wordmarkOf(testBrand().Name)), out)
		}
		for i, row := range rows {
			if i < len(wordmarkOf(testBrand().Name)) && !strings.Contains(row, wordmarkOf(testBrand().Name)[i]) {
				t.Errorf("width %d: line %d holds no row of the name: %q", w, i, row)
			}
		}
	}
}

// What the wordmark spells: the font is read back, so a letter drawn wrong
// fails here and not on somebody's screen.
func TestTheWordmarkSpellsTheNameItWasGiven(t *testing.T) {
	t.Parallel()
	if len(font()) == 0 {
		t.Fatal("the font is empty, so this proves nothing")
	}
	// Every letter as its two rows must differ from every other, or two letters
	// would read back as one.
	seen := map[[2]string]rune{}
	for r, g := range font() {
		if cells(g[0]) != cells(g[1]) {
			t.Errorf("%q: its rows are %d and %d cells wide", r, cells(g[0]), cells(g[1]))
		}
		if other, dup := seen[g]; dup {
			t.Errorf("%q and %q are drawn the same", r, other)
		}
		seen[g] = r
	}
	for _, name := range []string{"app", "box", "hu9"} {
		rows := wordmarkOf(name)
		if len(rows) != 2 || cells(rows[0]) != cells(rows[1]) {
			t.Fatalf("%q: rows %q do not line up", name, rows)
		}
		want := 0
		for _, r := range name {
			want += cells(font()[r][0])
		}
		want += len(name) - 1
		if cells(rows[0]) != want {
			t.Errorf("%q is %d cells wide, its letters and gaps come to %d", name, cells(rows[0]), want)
		}
		var read []rune
		for col, letter := 0, 0; letter < len(name); letter++ {
			g := font()[rune(name[letter])]
			w := cells(g[0])
			if string([]rune(rows[0])[col:col+w]) != g[0] || string([]rune(rows[1])[col:col+w]) != g[1] {
				t.Errorf("%q: letter %d is not drawn as %q", name, letter, name[letter])
			}
			read = append(read, rune(name[letter]))
			col += w + 1
		}
		if string(read) != name {
			t.Errorf("read back %q, want %q", string(read), name)
		}
	}
}

// A Brand reaches the banner: its name as the wordmark, its mark at the left,
// its row beside the top. Another brand draws another banner.
func TestABrandReachesTheBanner(t *testing.T) {
	t.Parallel()
	scr := screenOpt(true, 0)
	b := Brand{Name: "box", Mark: "<<\n>>", Row: "another tagline"}
	got := ansi.Strip(bannerOf(New(scr), b))
	for _, want := range append(wordmarkOf("box"), "another tagline", "<<", ">>") {
		if !strings.Contains(got, want) {
			t.Errorf("the banner lacks %q:\n%s", want, got)
		}
	}
	if other := ansi.Strip(bannerOf(New(scr), testBrand())); other == got {
		t.Error("two brands drew the same banner")
	}
}

// The negative halves: a name the font cannot spell, or a banner off a
// terminal, draws nothing rather than half a wordmark.
func TestABannerThatCannotBeDrawnIsEmpty(t *testing.T) {
	t.Parallel()
	scr := screenOpt(true, 0)
	for _, b := range []Brand{{}, {Name: "a b"}, {Name: "caf\u00e9"}} {
		if got := bannerOf(New(scr), b); got != "" {
			t.Errorf("%+v drew %q", b, got)
		}
	}
	if got := bannerOf(New(), testBrand()); got != "" {
		t.Errorf("off a terminal the banner was %q", got)
	}
}

// The mark goes first, at exactly the width where it no longer fits, and the
// name at exactly its own.
func TestTheMarkGoesBeforeTheName(t *testing.T) { //nolint:paralleltest // serial: it captures the process's streams
	wm := wordmarkOf(testBrand().Name)
	full := bannerWidth(wm, []string{testBrand().Row, "v1"})
	bw := testBrand().markWidth()
	for _, tc := range []struct {
		width      int
		name, mark bool
	}{
		{full - 1, false, false}, {full, true, false},
		{full + bw - 1, true, false}, {full + bw, true, true}, {0, true, true},
	} {
		at := onScreen(stream{rich: true, interactive: true, width: tc.width}, stream{rich: true}, false)
		out := capture(t, &os.Stdout, func() { New(at).Arrive(testBrand(), "v1") })
		if got := strings.Contains(out, "a made-up tagline"); got != tc.name {
			t.Errorf("width %d: name drawn %v, want %v:\n%s", tc.width, got, tc.name, out)
		}
		if got := strings.Contains(out, "▀▀███▌"); got != tc.mark {
			t.Errorf("width %d: mark drawn %v, want %v:\n%s", tc.width, got, tc.mark, out)
		}
	}
}

// A mark keeps the roles it is given: a program whose mark has a dim moon
// beside an action-coloured body draws exactly that, so adopting the module
// moves no golden.
func TestAMarkKeepsItsDimRunes(t *testing.T) {
	t.Parallel()
	scr := screenOpt(true, 0)
	moon := Brand{Name: "app", Mark: "◖  ▄██▄\n ▀▀███▌", MarkGap: 3, MarkNote: "◖", Row: "a made-up tagline"}
	rows := strings.Split(bannerOf(New(scr), moon), "\n")
	if len(rows) != 2 {
		t.Fatalf("the banner is %d rows", len(rows))
	}
	if want := New(scr).Note("◖") + New(scr).Action("  ▄██▄"); !strings.HasPrefix(rows[0], want) {
		t.Errorf("the top row does not start with a dim moon and an action body:\n%q\nwant prefix %q", rows[0], want)
	}
	// The negative half: with no MarkNote the same mark has no dim run.
	flat := moon
	flat.MarkNote = ""
	if want := New(scr).Note("◖"); strings.HasPrefix(bannerOf(New(scr), flat), want) {
		t.Errorf("a mark with no MarkNote still drew a dim moon: %q", bannerOf(New(scr), flat))
	}
}

// The column is the widest row and the gap: one when none is given.
func TestTheMarkColumnIsItsWidestRowAndAGap(t *testing.T) {
	t.Parallel()
	if got := testBrand().markWidth(); got != 10 {
		t.Errorf("a 7-cell mark with a gap of 3 has a column of %d", got)
	}
	if got := (Brand{}).markWidth(); got != 0 {
		t.Errorf("no mark has a column of %d", got)
	}
}

// A name with capitals draws the same wordmark as the name in lower case,
// not nothing and not a silent gap.
func TestACapitalisedNameDrawsTheSameWordmark(t *testing.T) {
	t.Parallel()
	scr := screenOpt(true, 0)
	lower := Brand{Name: "app", Row: "x"}
	for _, name := range []string{"App", "APP", "aPp"} {
		b := lower
		b.Name = name
		if got, want := bannerOf(New(scr), b), bannerOf(New(scr), lower); got == "" || got != want {
			t.Errorf("%q drew %q, want %q", name, got, want)
		}
	}
	// The negative half: a rune the font lacks is still refused.
	if got := bannerOf(New(scr), Brand{Name: "Ap p", Row: "x"}); got != "" {
		t.Errorf("a name with a space drew %q", got)
	}
}

func TestTheGapDefaultsToOne(t *testing.T) {
	t.Parallel()
	if got := (Brand{Mark: "ab\ncd"}).markWidth(); got != 3 {
		t.Errorf("a 2-cell mark with no gap has a column of %d, want 3", got)
	}
}

// Sample brands, made up for this test, each drawing its banner cell for
// cell and in the roles its mark is given. The expected pictures are written
// out, not read from the font, so a glyph that drifts fails here. The last
// three cases spell the whole alphabet and the digits, so every glyph is held
// to its picture.
func TestSampleBrandsDrawTheBannersWrittenOut(t *testing.T) {
	t.Parallel()
	scr := screenOpt(true, 0)
	for _, tc := range []struct {
		brand    Brand
		version  string
		wantRows [2]string
		dimMark  func() string // the mark's first row, painted; nil for no mark
	}{
		{
			brand: Brand{Name: "wombat", Mark: "◖  ▄▄▄▄\n ▀▀██▀▌", MarkNote: "◖", MarkGap: 3,
				Row: "a notes app that syncs"},
			version: "v1",
			wantRows: [2]string{
				"◖  ▄▄▄▄   █ █ █ █▀█ █▀▄▀█ █▄▄ ▄▀█ ▀█▀   a notes app that syncs",
				" ▀▀██▀▌   ▀▄▀▄▀ █▄█ █ ▀ █ █▄█ █▀█  █    v1",
			},
			dimMark: func() string { return New(scr).Note("◖") + New(scr).Action("  ▄▄▄▄") },
		},
		{
			brand: Brand{Name: "kettle", Mark: "◖ ▐▀▀▌\n  ▐  ▌", MarkNote: "◖",
				Beside: func(v string) [2]string { return [2]string{"tool  " + v, "the page for a tool"} }},
			version: "v1",
			wantRows: [2]string{
				"◖ ▐▀▀▌ █▄▀ █▀▀ ▀█▀ ▀█▀ █   █▀▀   tool  v1",
				"  ▐  ▌ █ █ ██▄  █   █  █▄▄ ██▄   the page for a tool",
			},
			dimMark: func() string { return New(scr).Note("◖") + New(scr).Action(" ▐▀▀▌") },
		},
		{
			brand: Brand{Name: "orchard", Mark: "  ▄█•█▄▖\n  ▀██▀▁▁", MarkNote: "▁", MarkInk: "•", MarkGap: 2,
				Row: "tasks that wait until run"},
			version: "v1",
			wantRows: [2]string{
				"  ▄█•█▄▖  █▀█ █▀█ █▀▀ █ █ ▄▀█ █▀█ █▀▄   tasks that wait until run",
				"  ▀██▀▁▁  █▄█ █▀▄ █▄▄ █▀█ █▀█ █▀▄ █▄▀   v1",
			},
			dimMark: func() string { return New(scr).Action("  ▄█") + "•" + New(scr).Action("█▄▖") },
		},
		{
			brand:   Brand{Name: "abcdefghijklm", Row: "the first half"},
			version: "v1",
			wantRows: [2]string{
				"▄▀█ █▄▄ █▀▀ █▀▄ █▀▀ █▀▀ █▀▀ █ █ ▀█▀   █ █▄▀ █   █▀▄▀█   the first half",
				"█▀█ █▄█ █▄▄ █▄▀ ██▄ █▀  █▄█ █▀█ ▄█▄ █▄█ █ █ █▄▄ █ ▀ █   v1",
			},
		},
		{
			brand:   Brand{Name: "nopqrstuvwxyz", Row: "the second half"},
			version: "v1",
			wantRows: [2]string{
				"█▄ █ █▀█ █▀█ █▀█ █▀█ ▄▀▀ ▀█▀ █ █ █ █ █ █ █ ▀▄▀ █ █ ▀▀█   the second half",
				"█ ▀█ █▄█ █▀▀ ▀▀▄ █▀▄ ▄▄▀  █  █▄█ ▀▄▀ ▀▄▀▄▀ █ █  █  █▄▄   v1",
			},
		},
		{
			brand:   Brand{Name: "0123456789", Row: "the digits"},
			version: "v1",
			wantRows: [2]string{
				"█▀█ ▄█  ▀▀▄ ▀▀█ █ █ █▀▀ █▀  ▀▀█ ▄▀▄ █▀█   the digits",
				"▀▄▀  █  █▄▄ ▄▄█ ▀▀█ ▄▄█ █▄█   █ ▀▄▀ ▀▀█   v1",
			},
		},
	} {
		rows := strings.Split(banner(New(scr).outStream(), tc.brand, tc.version), "\n")
		if len(rows) != 2 {
			t.Fatalf("%s: %d rows", tc.brand.Name, len(rows))
		}
		for i, row := range rows {
			if got := ansi.Strip(row); got != tc.wantRows[i] {
				t.Errorf("%s row %d:\n got %q\nwant %q", tc.brand.Name, i, got, tc.wantRows[i])
			}
		}
		if tc.dimMark == nil {
			continue
		}
		if want := tc.dimMark(); !strings.HasPrefix(rows[0], want) {
			t.Errorf("%s: the mark's roles: %q does not start with %q", tc.brand.Name, rows[0], want)
		}
	}
}

// tallMark is a made-up mark four rows high, in braille: a program's own mark that is
// taller than the wordmark.
func tallMark() Brand {
	return Brand{Name: "app", Mark: "⠀⢀⡀⠀⠀⠀\n⠰⣿⣷⣄⡀⠀\n⠀⠈⠻⣿⣿⡆\n⠀⠀⠀⠈⠉⠁", MarkGap: 2, Row: "a made-up tagline"}
}

// A mark of four rows draws beside the name with its rows aligned: each row of the mark
// in its column, the wordmark level with its two middle rows, and what is beside the
// wordmark still beside it. The golden is the four rows as they are drawn, colour aside.
//
// What it costs when this fails: a program whose mark is four lines keeps its own copy
// of the banner to draw it, or its mark is drawn with its rows out of line.
func TestAFourRowMarkDrawsBesideTheNameWithItsRowsAligned(t *testing.T) {
	t.Parallel()
	b := tallMark()
	got := ansi.Strip(banner(stream{rich: true}, b, "v1.2.3"))
	uitest.Golden(t, "banner-mark-four-rows", got)
	rows := strings.Split(got, "\n")
	if len(rows) != 4 {
		t.Fatalf("the banner is %d rows, want the mark's 4:\n%s", len(rows), got)
	}
	wm, mark, bw := wordmarkOf(b.Name), b.markRows(), b.markWidth()
	checked := 0
	for i, row := range rows {
		checked++
		if !strings.HasPrefix(row, mark[i]) {
			t.Errorf("row %d does not start with the mark's row %d, %q: %q", i, i, mark[i], row)
		}
		if j := i - 1; j >= 0 && j < len(wm) {
			if got := ansi.Cut(row, bw, bw+cells(wm[j])); got != wm[j] {
				t.Errorf("row %d holds %q at column %d, want the wordmark's row %d, %q", i, got, bw, j, wm[j])
			}
		} else if strings.TrimRight(row, " ") != mark[i] {
			t.Errorf("row %d is the mark's alone, and holds %q", i, row)
		}
	}
	if checked != len(mark) {
		t.Fatalf("checked %d rows, want the mark's %d", checked, len(mark))
	}
	if !strings.Contains(rows[1], "a made-up tagline") || !strings.Contains(rows[2], "v1.2.3") {
		t.Errorf("the tagline and the version are not beside the wordmark's rows:\n%s", got)
	}
}

// A mark of three rows puts the wordmark level with its top two, as the two-row mark
// does, and one of one row is beside the wordmark's top row.
//
// What it costs when this fails: a program with a mark of one or three rows finds
// its name drawn lower than it was, or a row of its mark lost.
func TestAShorterMarkKeepsTheWordmarkAtItsTop(t *testing.T) {
	t.Parallel()
	ran := 0
	for _, mark := range []string{"<\n<\n<", "<"} {
		ran++
		b := Brand{Name: "app", Mark: mark, Row: "a made-up tagline"}
		rows := strings.Split(ansi.Strip(banner(stream{rich: true}, b, "v1")), "\n")
		wm := wordmarkOf(b.Name)
		if want := max(len(b.markRows()), len(wm)); len(rows) != want {
			t.Fatalf("a %d-row mark drew %d rows, want %d", len(b.markRows()), len(rows), want)
		}
		if !strings.Contains(rows[0], wm[0]) || !strings.Contains(rows[1], wm[1]) {
			t.Errorf("a %d-row mark moved the wordmark from the top: %q", len(b.markRows()), rows)
		}
	}
	if ran != 2 {
		t.Fatalf("ran %d marks, want 2", ran)
	}
}

// markOf is a mark of n rows, each wide cells of "#", with the row's number set in it.
func markOf(n, wide int) string {
	rows := make([]string, n)
	for i := range rows {
		rows[i] = strings.Repeat("#", wide-1) + string(rune('1'+i))
	}
	return strings.Join(rows, "\n")
}

// A mark of any height draws: the banner is as high as the higher of the mark and the
// wordmark's two rows, and the shorter is centred beside the other. The goldens are the
// banners of 1, 2, 4, 5 and 8 rows at 80 columns.
//
// What it costs when this fails: a program with a mark of five or more lines keeps its own
// copy of the banner.
func TestAMarkOfAnyHeightDrawsBesideTheName(t *testing.T) {
	t.Parallel()
	for _, n := range []int{1, 2, 4, 5, 8} {
		b := Brand{Name: "app", Mark: markOf(n, 6), MarkGap: 2, Row: "a made-up tagline"}
		got := ansi.Strip(banner(stream{rich: true, width: 80}, b, "v1.2.3"))
		uitest.Golden(t, fmt.Sprintf("banner-mark-%d-rows", n), got)
		rows := strings.Split(got, "\n")
		if want := max(n, 2); len(rows) != want {
			t.Fatalf("a %d-row mark drew %d rows, want %d:\n%s", n, len(rows), want, got)
		}
		wm := wordmarkOf(b.Name)
		top := (len(rows) - len(wm)) / 2
		for j, w := range wm {
			if !strings.Contains(rows[top+j], w) {
				t.Errorf("a %d-row mark: row %d holds no row %d of the wordmark: %q", n, top+j, j, rows[top+j])
			}
		}
		for k := range n {
			if want := string(rune('1' + k)); !strings.Contains(got, want) {
				t.Errorf("a %d-row mark lost its row %d:\n%s", n, k+1, got)
			}
		}
	}
}

// A mark the screen has no room for is left out: the wordmark stands alone, and no line of
// the banner is wider than the screen at 80, 60 and 40 columns.
//
// What it costs when this fails: a wide mark breaks a line on a narrow terminal, or the
// banner is lost with the mark.
func TestAWideMarkIsLeftOutAndTheWordmarkStandsAlone(t *testing.T) {
	t.Parallel()
	b := Brand{Name: "app", Mark: markOf(5, 20), MarkGap: 1, Row: "a made-up tagline"}
	alone := ansi.Strip(banner(stream{rich: true, width: 40}, Brand{Name: b.Name, Row: b.Row}, "v1"))
	for _, w := range []int{80, 60, 40} {
		got := ansi.Strip(banner(stream{rich: true, width: w}, b, "v1"))
		for line := range strings.SplitSeq(got, "\n") {
			if cells(line) > w {
				t.Errorf("width %d: a line is %d cells: %q", w, cells(line), line)
			}
		}
		if w == 40 && got != alone {
			t.Errorf("width 40: the wide mark was not left out:\n%s", got)
		}
		if w < 40 && got == "" {
			t.Errorf("width %d: no banner", w)
		}
	}
	if got := ansi.Strip(banner(stream{rich: true, width: 80}, b, "v1")); !strings.Contains(got, "####") {
		t.Errorf("width 80: the mark was left out where it fits:\n%s", got)
	}
}

// A Brand whose name holds a letter the font lacks still draws no banner, whatever its mark.
func TestANameTheFontLacksStillDrawsNoBannerWhateverItsMark(t *testing.T) {
	t.Parallel()
	b := Brand{Name: "a-b", Mark: markOf(5, 6)}
	if got := banner(stream{rich: true, width: 80}, b, ""); got != "" {
		t.Errorf("a name with a hyphen drew a banner:\n%s", got)
	}
}
