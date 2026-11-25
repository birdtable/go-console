package ui

import "fmt"

// Flags names the flags Execute gives every command, without the dashes: Plain (no live view, no
// pager, questions as plain lines), Accessible (the same, for a screen reader), Yes (answers yes to
// every question before a change), JSON (one JSON document on stdout, the rest on stderr), NoColor
// (strips the escapes). A name left empty gives no flag: nothing is registered and no message of ui names one.
// There is no default spelling, since a spelling is one program's policy.
type Flags struct{ Plain, Accessible, Yes, JSON, NoColor string }

// FlagNames is the spelling of the flags Execute gives the commands (Flags), which the session's
// messages and lines to type name. Without it Execute gives none.
func FlagNames(f Flags) SessionOption { return func(s *Session) { s.flags = f } }

// dashed is the flag as typed, or "" for a flag the program did not name.
func dashed(name string) string {
	if name == "" {
		return ""
	}
	return "--" + name
}

// yesFlag is a message's name for the yes flag: its spelling, or words that name none.
func (s *Session) yesFlag() string { return orElse(dashed(s.flags.Yes), s.messages().YesFlagName) }

func orElse(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// plainLine is the command with the plain flag, or the command alone for a program with none.
func (s *Session) plainLine() string { return s.commandLine(dashed(s.flags.Plain)) }

// refuseOwn is the refusal of a flag name the program gave Execute and also defines itself.
func refuseOwn(cmd, flag string) error {
	return fmt.Errorf("ui: the command %q defines --%s, which Execute gives every program: take that definition out, and read the session instead", cmd, flag)
}
