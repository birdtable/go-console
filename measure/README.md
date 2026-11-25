Pure text and colour measures, with no I/O: how wide a string is in terminal cells, and how readable one colour is on another.

`github.com/birdtable/go-console/measure` is a leaf. It takes the standard library and `github.com/charmbracelet/x/ansi` and nothing else, and it reads no environment, file or terminal, at init or later, so a layer that must stay pure takes it without taking `ui` (cobra, lipgloss and the session code).

It adds the column with its gap and the two WCAG formulas, which `x/ansi` leaves out, and the caller configures every width, threshold and colour.

| name | what it does |
|---|---|
| `Column(s, n)` | pads `s` to `n` display cells, counted by `ansi.StringWidth`, and always leaves one space after a label that fills or overflows its column |
| `RelativeLuminance(c)` | the WCAG 2.1 relative luminance of a colour, 0 for black to 1 for white; alpha is not used |
| `ContrastRatio(a, b)` | the WCAG 2.1 contrast ratio of two colours, 1 to 21, in either order; it holds no threshold |

`ui.Column`, `ui.RelativeLuminance` and `ui.ContrastRatio` keep their names and call these, so no caller changes. Every exported name is experimental below v1.
