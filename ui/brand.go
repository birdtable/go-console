package ui

// What differs from one program to the next, passed in and never spelled here.

import "strings"

// Brand is a program's identity for the terminal: what its banner and its
// wordmark are made from.
type Brand struct {
	// Name is the program's name, letters a to z and digits, in any case: the wordmark is lower-case. The wordmark
	// spells it in block letters.
	Name string
	// Mark is the program's mark (a logo), rows of text joined by newlines, of any
	// height. It is drawn in the action colour to the left of the wordmark. The
	// banner is as high as the higher of the mark and the wordmark's two rows, and
	// the shorter is centred beside the other. A mark the terminal has no room for
	// is the first thing to go when it is narrow, and the wordmark stands alone.
	// Empty draws none.
	Mark string
	// MarkNote lists the runes of Mark drawn in the dim note role, and MarkInk
	// those drawn in the terminal's own ink, with no colour at all (a dot or a
	// line in a logo). Every other rune is in the action colour.
	MarkNote, MarkInk string
	// MarkGap is the spaces between the mark's widest row and the wordmark,
	// one when zero.
	MarkGap int
	// Row is the line of text beside the wordmark's top row, a tagline. The
	// build's version goes beside the bottom row.
	Row string
	// Beside, when set, says what goes beside the two rows of the wordmark
	// for a build's version, in place of Row and the version: for a program
	// whose version leads and whose tagline follows.
	Beside func(version string) [2]string
}

// glyphs is the font as data: the letter, its top row, its bottom row, split
// by a bar because a row may hold spaces.
const glyphs = `
a|▄▀█|█▀█
b|█▄▄|█▄█
c|█▀▀|█▄▄
d|█▀▄|█▄▀
e|█▀▀|██▄
f|█▀▀|█▀ 
g|█▀▀|█▄█
h|█ █|█▀█
i|▀█▀|▄█▄
j|  █|█▄█
k|█▄▀|█ █
l|█  |█▄▄
m|█▀▄▀█|█ ▀ █
n|█▄ █|█ ▀█
o|█▀█|█▄█
p|█▀█|█▀▀
q|█▀█|▀▀▄
r|█▀█|█▀▄
s|▄▀▀|▄▄▀
t|▀█▀| █ 
u|█ █|█▄█
v|█ █|▀▄▀
w|█ █ █|▀▄▀▄▀
x|▀▄▀|█ █
y|█ █| █ 
z|▀▀█|█▄▄
0|█▀█|▀▄▀
1|▄█ | █ 
2|▀▀▄|█▄▄
3|▀▀█|▄▄█
4|█ █|▀▀█
5|█▀▀|▄▄█
6|█▀ |█▄█
7|▀▀█|  █
8|▄▀▄|▀▄▀
9|█▀█|▀▀█
`

// font is glyphs, read from the table each time it is asked: a banner asks once a run.
func font() map[rune][2]string {
	m := map[rune][2]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(glyphs), "\n") {
		f := strings.SplitN(line, "|", 3)
		m[[]rune(f[0])[0]] = [2]string{f[1], f[2]}
	}
	return m
}

// wordmarkOf spells name in the font, a space between letters. A rune the font
// lacks is left out, so it is the caller's check that Name is spellable
// (Brand.valid).
func wordmarkOf(name string) []string {
	top, bottom := []string{}, []string{}
	glyph := font()
	for _, r := range strings.ToLower(name) {
		g, ok := glyph[r]
		if !ok {
			continue
		}
		top, bottom = append(top, g[0]), append(bottom, g[1])
	}
	return []string{strings.Join(top, " "), strings.Join(bottom, " ")}
}

// A Brand that is not valid draws no banner.
func (b Brand) valid() bool {
	if b.Name == "" {
		return false
	}
	glyph := font()
	for _, r := range strings.ToLower(b.Name) {
		if _, ok := glyph[r]; !ok {
			return false
		}
	}
	return true
}

func (b Brand) markRows() []string {
	if b.Mark == "" {
		return nil
	}
	return strings.Split(b.Mark, "\n")
}

func (b Brand) markWidth() int {
	w := 0
	for _, row := range b.markRows() {
		w = max(w, cells(row))
	}
	if w == 0 {
		return 0
	}
	return w + max(b.MarkGap, 1)
}

func (b Brand) drawMark(st stream, row string) string {
	var out, run strings.Builder
	role := func(r rune) int {
		switch {
		case strings.ContainsRune(b.MarkNote, r):
			return 1
		case strings.ContainsRune(b.MarkInk, r):
			return 2
		}
		return 0
	}
	cur := 0
	flush := func() {
		if run.Len() == 0 {
			return
		}
		switch cur {
		case 1:
			out.WriteString(roleNote().on(st, run.String()))
		case 2:
			out.WriteString(run.String())
		default:
			out.WriteString(roleAction().on(st, run.String()))
		}
		run.Reset()
	}
	for _, r := range row {
		if k := role(r); k != cur {
			flush()
			cur = k
		}
		run.WriteRune(r)
	}
	flush()
	return out.String()
}

func (b Brand) besides(version string) []string {
	if b.Beside != nil {
		v := b.Beside(version)
		return v[:]
	}
	return []string{b.Row, version}
}

// asciiForm is the wordmark drawn in ASCII: a full block is #, an upper half is a quote and a lower half an underscore.
func asciiForm(rows []string) []string {
	r := strings.NewReplacer("█", "#", "▀", `"`, "▄", "_")
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = r.Replace(row)
	}
	return out
}
