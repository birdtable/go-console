package ui

// A table: lipgloss's at a terminal, aligned columns off one. Both faces
// drop the same columns and cut the same cells, so a line is never wider than
// the screen and a script reads what a person sees.

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/x/ansi"
	"github.com/dustin/go-humanize/english"
)

// A TableColumn is one column of a Table.
type TableColumn struct {
	Title string
	// Priority orders what goes when the width is short: the column with the
	// lowest goes first, the rightmost of equals first. The last one left is
	// cut to fit rather than dropped.
	Priority int
}

// NoHeaderRule draws every table the session paints with nothing between the header row and the body, for
// a program whose contract has no rule there. The columns, the wrap, the note of dropped columns and the
// header's weight and colour are as they were. Without it a painted table has the rule under its header;
// the plain face draws none either way.
func NoHeaderRule() SessionOption { return func(s *Session) { s.noHeaderRule = true } }

// Table is cols and rows as text: lipgloss's table at a terminal that renders,
// plain aligned columns, two spaces apart, with no box and no escape sequence
// anywhere else. A cell is a name from outside, so its control characters are
// shown as text (Name), and a row short of cells is padded with empty ones. A
// cell too long for its column wraps onto lines under it, and when columns
// were dropped for width one note line under the table says how many and which.
func (s *Session) Table(cols []TableColumn, rows [][]string) string {
	if len(cols) == 0 {
		return ""
	}
	keep, widths, dropped := fitColumns(cols, rows, s.Width(), 2)
	cell := func(text string, i int) string { return strings.Join(wrapCell(Name(text), widths[i]), "\n") }
	var (
		heads []string
		body  [][]string
	)
	for _, i := range keep {
		heads = append(heads, cell(cols[i].Title, i))
	}
	for _, r := range rows {
		row := make([]string, 0, len(keep))
		for _, i := range keep {
			text := ""
			if i < len(r) {
				text = r[i]
			}
			row = append(row, cell(text, i))
		}
		body = append(body, row)
	}
	out := plainTable(heads, body)
	if s.outStream().paints() {
		out = richTable(s.outStream(), heads, body, !s.noHeaderRule)
	}
	if len(dropped) == 0 {
		return out
	}
	note := english.Plural(len(dropped), "column", "columns") + " not shown: " + Name(strings.Join(dropped, ", "))
	lines := strings.Split(wrapOn(s.outStream(), 0, note), "\n")
	for i, l := range lines {
		lines[i] = roleNote().on(s.outStream(), l) // each line alone: a painted block is padded
	}
	return out + "\n" + strings.Join(lines, "\n")
}

const minLast = 16

// fitColumns is the columns that fit width (0 is unknown, so all), in order,
// each one's width: its widest cell, or less for the last one standing, which
// wraps; and the titles dropped, last dropped first.
func fitColumns(cols []TableColumn, rows [][]string, width, gap int) (keep []int, widths []int, dropped []string) {
	widths = make([]int, len(cols))
	for i, c := range cols {
		widths[i] = cells(Name(c.Title))
		for _, r := range rows {
			if i < len(r) {
				widths[i] = max(widths[i], cells(Name(r[i])))
			}
		}
		keep = append(keep, i)
	}
	if width <= 0 {
		return keep, widths, nil
	}
	total := func() int {
		n := gap * (len(keep) - 1)
		for _, i := range keep[:len(keep)-1] {
			n += widths[i]
		}
		return n + min(widths[keep[len(keep)-1]], minLast)
	}
	for total() > width && len(keep) > 1 {
		drop := lowest(cols, keep)
		dropped = append([]string{cols[drop].Title}, dropped...)
		keep = slices.DeleteFunc(keep, func(i int) bool { return i == drop })
	}
	left := width - gap*(len(keep)-1)
	for _, i := range keep[:len(keep)-1] {
		left -= widths[i]
	}
	last := keep[len(keep)-1]
	widths[last] = max(min(widths[last], left), 1)
	return keep, widths, dropped
}

func lowest(cols []TableColumn, keep []int) int {
	drop := keep[0]
	for _, i := range keep {
		if cols[i].Priority < cols[drop].Priority || (cols[i].Priority == cols[drop].Priority && i > drop) {
			drop = i
		}
	}
	return drop
}

// wrapCell is text broken at spaces into lines of at most n cells, a word longer
// than n cut, and never a word at a hyphen. Empty text is one empty line.
func wrapCell(text string, n int) []string {
	if n <= 0 || cells(text) <= n {
		return []string{text}
	}
	var out []string
	cur := ""
	for w := range strings.FieldsSeq(text) {
		for cells(w) > n {
			if cur != "" {
				out, cur = append(out, cur), ""
			}
			head := cutCells(w, n)
			out, w = append(out, head), strings.TrimPrefix(w, head)
		}
		switch {
		case cur == "":
			cur = w
		case cells(cur)+1+cells(w) <= n:
			cur += " " + w
		default:
			out, cur = append(out, cur), w
		}
	}
	return append(out, cur)
}

// cutCells is the longest head of w of at most n cells, one rune at least.
func cutCells(w string, n int) string {
	head, used := "", 0
	for _, r := range w {
		rw := cells(string(r))
		if head != "" && used+rw > n {
			break
		}
		head, used = head+string(r), used+rw
	}
	return head
}

func richTable(out stream, heads []string, body [][]string, rule bool) string {
	pad := lipgloss.NewStyle().PaddingRight(2)
	last := lipgloss.NewStyle()
	t := table.New().
		Border(ruleBorder(out)).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderColumn(false).BorderRow(false).BorderHeader(rule).
		BorderStyle(mutedStyle(out)).
		StyleFunc(func(row, col int) lipgloss.Style {
			st := pad
			if col == len(heads)-1 {
				st = last
			}
			if row == table.HeaderRow {
				if c := out.colour(nameMuted); c != nil {
					st = st.Foreground(c)
				}
				return st.Bold(true)
			}
			return st
		}).
		Headers(heads...).
		Rows(body...)
	return t.String()
}

// plainTable is the table with no colour and no rule under the header: the same
// lipgloss table, stripped of what it painted, with the space a line ends in
// cut off.
func plainTable(heads []string, body [][]string) string {
	lines := strings.Split(ansi.Strip(richTable(stream{}, heads, body, false)), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

// ruleBorder is the border whose Middle is the rule under a header: a line, or a hyphen where the
// terminal does not draw UTF-8.
func ruleBorder(out stream) lipgloss.Border {
	if out.ascii {
		return lipgloss.Border{Top: "-", Bottom: "-", Middle: "-"}
	}
	return lipgloss.NormalBorder()
}
