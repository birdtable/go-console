package ui

// Help drawn at the session's width. fang sizes its help from a package-level width it reads once from the
// process's terminal (120 off one) and takes no option to change it, so Execute draws help itself, in fang's
// layout and with the colours fang is given, at Width() and no wider than helpCap. fang keeps the rest.

import (
	"cmp"
	"fmt"
	"image/color"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"charm.land/fang/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	helpCap      = 120 // fang's own cap, kept
	helpMinKey   = 10
	helpShortPad = 2
	helpLongPad  = 4
	helpGap      = 2
	helpMinText  = 12 // the least a description gets beside its key, else the key takes a line
)

// wireHelp makes every help of root's tree this file's. fang sets root's help func inside Execute, after this
// runs, so root's own is put in place from three places fang's func is reached by: the --help flag, which sets
// it as the flags are read; the help command; and a root that has no run of its own, which draws it as its run.
// Every other command is given the func now.
func wireHelp(root *cobra.Command, run *Session) {
	draw := func(c *cobra.Command, _ []string) { drawHelp(c, run) }
	var each func(c *cobra.Command)
	each = func(c *cobra.Command) {
		if c != root {
			c.SetHelpFunc(draw)
		}
		for _, sub := range c.Commands() {
			each(sub)
		}
	}
	each(root)
	if root.Flags().Lookup("help") == nil && root.PersistentFlags().Lookup("help") == nil {
		short := "h"
		if shorthandUsed(root, "h") {
			short = ""
		}
		f := &helpFlag{set: func() { root.SetHelpFunc(draw) }}
		root.PersistentFlags().VarP(f, "help", short, fmt.Sprintf(run.messages().HelpFlagHelp, root.Name()))
		fl := root.PersistentFlags().Lookup("help")
		fl.NoOptDefVal, fl.Hidden = "true", true
	}
	if !root.Runnable() {
		root.RunE = func(c *cobra.Command, _ []string) error { drawHelp(c, run); return nil }
	}
	root.SetHelpCommand(&cobra.Command{
		Use:   "help " + run.messages().HelpCommandArgs,
		Short: run.messages().HelpAnyShort,
		RunE: func(c *cobra.Command, args []string) error {
			cmd, _, err := c.Root().Find(args)
			if cmd == nil || err != nil {
				c.Printf(run.messages().HelpUnknownTopic+"\n", args)
				return c.Root().Usage()
			}
			drawHelp(cmd, run)
			return nil
		},
	})
}

// helpFlag is the bool --help, whose being set is what puts this file's help in place before cobra asks for it.
type helpFlag struct {
	on  bool
	set func()
}

func (f *helpFlag) String() string { return strconv.FormatBool(f.on) }
func (*helpFlag) Type() string     { return "bool" }
func (f *helpFlag) Set(s string) error {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return fmt.Errorf("help flag %q: %w", s, err)
	}
	if f.on = b; b {
		f.set()
	}
	return nil
}

func shorthandUsed(c *cobra.Command, short string) bool {
	used := false
	for _, set := range []*pflag.FlagSet{c.Flags(), c.PersistentFlags()} {
		set.VisitAll(func(f *pflag.Flag) { used = used || f.Shorthand == short })
	}
	for _, sub := range c.Commands() {
		used = used || shorthandUsed(sub, short)
	}
	return used
}

// helpWidth is the width help is drawn to: the session's, which MaxWidth has capped, and fang's 120 where none
// is known or it is more.
func helpWidth(run *Session) int {
	if w := run.Width(); w > 0 && w < helpCap {
		return w
	}
	return helpCap
}

// helpPaint is the styles of one help page. The code ones sit on the code ground, the rest on none.
type helpPaint struct {
	text, title, desc, def, flag, block lipgloss.Style
	code, row                           useStyle
	comment, arg, dimCode               lipgloss.Style
	helpFlag                            string // the help flag's description, a format of the command's name
}

// useStyle paints a usage line: the program's name, the words after it and the dimmed [command] and [--flags].
type useStyle struct{ name, cmd, dim lipgloss.Style }

func newHelpPaint(cs fang.ColorScheme) helpPaint {
	on := func(fg color.Color, ground bool) lipgloss.Style {
		s := lipgloss.NewStyle().Foreground(fg)
		if ground {
			s = s.Background(cs.Codeblock)
		}
		return s
	}
	return helpPaint{
		text:  lipgloss.NewStyle().Foreground(cs.Base),
		title: lipgloss.NewStyle().Bold(true).Foreground(cs.Title).Transform(strings.ToUpper).Padding(1, 0).Margin(0, helpShortPad),
		desc:  on(cs.Description, false),
		def:   on(cs.FlagDefault, false),
		flag:  on(cs.Flag, false),
		block: lipgloss.NewStyle().Background(cs.Codeblock).Foreground(cs.Base).MarginLeft(helpShortPad).Padding(1, helpShortPad),
		code:  useStyle{on(cs.Program, true), on(cs.Command, true), on(cs.DimmedArgument, true)},
		row:   useStyle{on(cs.Program, false), on(cs.Command, false), on(cs.DimmedArgument, false)},
		// A comment, a plain word and the space between words, in an example on the code ground.
		comment: on(cs.Comment, true), arg: on(cs.Argument, true), dimCode: on(cs.DimmedArgument, true),
	}
}

// use is c's usage line, painted: its name, the rest of its Use, and what a person may add.
func (p helpPaint) use(c *cobra.Command, full bool, st useStyle) string {
	u := c.Use
	if full {
		u = c.UseLine()
	}
	hasCommands := strings.Contains(u, "[command]") || c.HasAvailableSubCommands()
	hasFlags := strings.Contains(u, "[flags]") || strings.Contains(u, "[--flags]") || c.HasAvailableFlags()
	hasArgs := strings.Contains(u, "[args]")
	for _, k := range []string{"[args]", "[flags]", "[--flags]", "[command]"} {
		u = strings.ReplaceAll(u, k, "")
	}
	parts := strings.Fields(u)
	if len(parts) == 0 {
		return ""
	}
	out := []string{st.cmd.Render(strings.Join(parts, " "))}
	if full {
		out = []string{st.name.Render(parts[0])}
		if len(parts) > 1 {
			out = append(out, st.cmd.Render(" "+strings.Join(parts[1:], " ")))
		}
	}
	for _, add := range []struct {
		on   bool
		text string
	}{{hasCommands, " [command]"}, {hasArgs, " [args]"}, {hasFlags, " [--flags]"}} {
		if add.on {
			out = append(out, st.dim.Render(add.text))
		}
	}
	return strings.Join(out, "")
}

// examples is c's Example, a line each: a comment dimmed, the program's name, its flags and the words between.
func (p helpPaint) examples(c *cobra.Command) []string {
	var out []string
	for line := range strings.SplitSeq(c.Example, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			out = append(out, p.comment.Render(line))
			continue
		}
		var words []string
		for i, a := range strings.Fields(line) {
			st := p.arg
			switch {
			case i == 0 && a == c.Root().Name():
				st = p.code.name
			case strings.HasPrefix(a, "-"):
				st = p.flag.Background(p.block.GetBackground())
			}
			words = append(words, st.Render(a))
		}
		out = append(out, strings.Join(words, p.dimCode.Render(" ")))
	}
	return out
}

type helpRow struct {
	key   string
	desc  string
	extra string
}

// lines is the row's description broken to limit cells, then its default, dimmed, on the last line when
// that fits and on a line of its own when not.
func (p helpPaint) lines(r helpRow, limit int) []string {
	var out []string
	for l := range strings.SplitSeq(breakAtSpaces(capFirst(r.desc), limit), "\n") {
		out = append(out, p.desc.Render(l))
	}
	if r.extra == "" {
		return out
	}
	if last := len(out) - 1; ansi.StringWidth(out[last])+len(r.extra) <= limit {
		out[last] += p.def.Render(r.extra)
	} else {
		out = append(out, p.def.Render(strings.TrimSpace(r.extra)))
	}
	return out
}

// drawHelp writes c's help, in fang's layout, at the session's width.
func drawHelp(c *cobra.Command, run *Session) {
	c.InitDefaultHelpFlag()
	// The session says whether the screen takes colour, as the logger's profile does; the process's
	// environment is Detect's to read, so this reads none.
	profile := colorprofile.NoTTY
	if run.term.Colour {
		profile = colorprofile.TrueColor
	}
	var page strings.Builder
	defer func() { _, _ = io.WriteString(c.OutOrStdout(), trailing.ReplaceAllStringFunc(page.String(), bare)) }()
	w := &colorprofile.Writer{Forward: &page, Profile: profile}
	width := helpWidth(run)
	p := newHelpPaint(helpColours(run.outStream()))
	p.helpFlag = run.messages().HelpFlagHelp

	if text := cmp.Or(c.Long, c.Short); text != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, p.text.Render(indent(breakAtSpaces(text, width-helpShortPad), helpShortPad)))
	}
	usage, examples := p.use(c, true, p.code), p.examples(c)
	pad := p.block.GetHorizontalPadding()
	bw := lipgloss.Width(usage)
	for _, ex := range examples {
		bw = max(bw, lipgloss.Width(ex))
	}
	block := p.block.Width(min(width-pad, bw+pad))
	cw := max(block.GetWidth()-pad, 1)
	_, _ = fmt.Fprintln(w, p.title.Render("usage"))
	_, _ = fmt.Fprintln(w, block.Render(breakAtSpaces(usage, cw)))
	if len(examples) > 0 {
		for i, ex := range examples {
			examples[i] = breakAtSpaces(ex, cw)
		}
		_, _ = fmt.Fprintln(w, p.title.Render("examples"))
		_, _ = fmt.Fprintln(w, block.Render(strings.Join(examples, "\n")))
	}

	drawRows(w, p, c, width)
}

// trailing is the spaces, tabs and styling a line ends in; bare is the styling of it alone, so no line of help ends
// in whitespace, piped or at a terminal.
var (
	trailing = regexp.MustCompile(`(?m)(?:[ \t]|\x1b\[[0-9;:]*m)+$`)
	styling  = regexp.MustCompile(`\x1b\[[0-9;:]*m`)
)

func bare(m string) string { return strings.Join(styling.FindAllString(m, -1), "") }

// commandRows is the rows of c's commands by group, with the titles and the order of the groups.
func commandRows(c *cobra.Command, p helpPaint) (groups map[string][]helpRow, titles map[string]string, ids []string) {
	groups, titles, ids = map[string][]helpRow{}, map[string]string{"": "commands"}, []string{""}
	for _, g := range c.Groups() {
		titles[g.ID], ids = g.Title, append(ids, g.ID)
	}
	for _, sc := range c.Commands() {
		if !sc.Hidden {
			groups[sc.GroupID] = append(groups[sc.GroupID], helpRow{key: p.use(sc, false, p.row), desc: sc.Short})
		}
	}
	return groups, titles, ids
}

func flagRows(c *cobra.Command, p helpPaint) []helpRow {
	var flags []helpRow
	c.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden && f.Name != "help" {
			return
		}
		r := helpRow{key: p.flag.Render("--" + f.Name), desc: f.Usage}
		if f.Shorthand != "" {
			r.key = p.flag.Render("-" + f.Shorthand + " --" + f.Name)
		}
		if f.Name == "help" {
			r.desc = fmt.Sprintf(p.helpFlag, c.Name())
		}
		if d := f.DefValue; d != "" && d != "false" && d != "0" && d != "[]" {
			r.extra = " (" + d + ")"
		}
		flags = append(flags, r)
	})
	return flags
}

// drawRows writes the command and flag sections, the keys in one column and the text wrapped beside it.
func drawRows(w io.Writer, p helpPaint, c *cobra.Command, width int) {
	groups, titles, ids := commandRows(c, p)
	flags := flagRows(c, p)
	space, text := helpMinKey, helpMinText
	for _, rows := range append(slices.Collect(maps.Values(groups)), flags) {
		for _, r := range rows {
			space = max(space, lipgloss.Width(r.key)+helpGap)
			// A word is never split, so the text column is at least as wide as the longest one.
			for word := range strings.FieldsSeq(r.desc + r.extra) {
				text = max(text, ansi.StringWidth(word))
			}
		}
	}
	space = max(min(space, width-helpLongPad-text), 1)
	section := func(title string, rows []helpRow) {
		if len(rows) == 0 {
			return
		}
		_, _ = fmt.Fprintln(w, p.title.Render(title))
		for _, r := range rows {
			lines := p.lines(r, max(width-helpLongPad-space, 1))
			head := strings.Repeat(" ", helpLongPad) + r.key
			if lipgloss.Width(r.key)+helpGap <= space {
				head += strings.Repeat(" ", space-lipgloss.Width(r.key)) + lines[0]
				lines = lines[1:]
			}
			_, _ = fmt.Fprintln(w, head)
			for _, l := range lines {
				_, _ = fmt.Fprintln(w, strings.Repeat(" ", helpLongPad+space)+l)
			}
		}
	}
	for _, id := range ids {
		section(titles[id], groups[id])
	}
	section("flags", flags)
	_, _ = fmt.Fprintln(w)
}

func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

// capFirst capitalises the first word's first letter, as fang does the description of a command and a flag.
func capFirst(s string) string {
	rs := []rune(s)
	for i, r := range rs {
		if !unicode.IsSpace(r) {
			rs[i] = unicode.ToUpper(r)
			break
		}
	}
	return string(rs)
}

// wireOwnHelp is wireHelp for a program that keeps its own help (WithoutBuiltins). fang sets root's help func inside
// Execute, over the program's, so the program's is put back from the places wireHelp uses: the --help flag, a root
// with no run of its own, and a help command when the program has none.
func wireOwnHelp(root *cobra.Command) {
	own := root.HelpFunc()
	if root.Flags().Lookup("help") == nil && root.PersistentFlags().Lookup("help") == nil {
		short := "h"
		if shorthandUsed(root, "h") {
			short = ""
		}
		root.PersistentFlags().VarP(&helpFlag{set: func() { root.SetHelpFunc(own) }}, "help", short, "help for "+root.Name())
		fl := root.PersistentFlags().Lookup("help")
		fl.NoOptDefVal, fl.Hidden = "true", true
	}
	if !root.Runnable() {
		root.RunE = func(c *cobra.Command, args []string) error { own(c, args); return nil }
	}
	if slices.ContainsFunc(root.Commands(), func(c *cobra.Command) bool { return c.Name() == "help" }) {
		return
	}
	root.SetHelpCommand(&cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		RunE: func(c *cobra.Command, args []string) error {
			cmd, _, err := c.Root().Find(args)
			if cmd == nil || err != nil {
				c.Printf("Unknown help topic %#q\n", args)
				return c.Root().Usage()
			}
			own(cmd, args)
			return nil
		},
	})
}
