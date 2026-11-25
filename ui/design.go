// Package ui is the only thing in a program that prints: the terminal look every
// command-line program shares. It holds the roles, the closing words, the
// progress line, prompts and tables, and every rule for text that may reach
// a screen, a pipe or a log.
//
// It is built on lipgloss (styles, colours, widths), bubbletea and bubbles
// (the live view, the pager), huh (forms and menus), fang with cobra (help,
// version and errors) and go-run's exit (endings and codes). It adds what those
// leave out: a line with its role, plain in a pipe, the banner, tables, Pick,
// Strip and Main. The program configures it: Brand, Theme, EnvRules, Endings,
// WithMessages and FlagNames.
//
// The look is written down once for every tool that uses it. A change to a
// role, the layout or the closing words is made there and in every tool,
// never in one alone.
//
// # The rule everything else follows from
//
// Presentation is what only exists at a known terminal; behaviour is what
// survives a pipe. No information may live only in presentation, colour is a
// second channel for somebody at a screen, never the first one. A program runs
// unattended from a scheduler as often as it runs at a prompt, and the
// unattended reader gets bytes out of a log months later.
//
// Two capabilities, which the caller says of each stream (Terminal, Detect):
//
//	Width > 0   the screen width is known, so text may be reflowed
//	Colour      escapes will render, so colour may be used
//
// # The roles
//
// Each is painted in the session's theme (WithTheme); the colours named are
// the default's, the terminal's own.
//
//	Bad     red     state that is wrong. A file is at risk, or lost.
//	Attn    yellow  state that is undecided. True, but a person must rule on it.
//	Good    green   state that is right.
//	Action  cyan    text the reader will type. Commands, paths to paste.
//	Note    muted   text the reader may skip. Provenance, counts, rationale.
//	Bold   ,       one thing per screen: the heading, or the answer.
//
// Attention carries more weight here than in most tools. "Cannot tell" is a
// routine, non-failing outcome, so it must read as neither success nor error.
package ui
