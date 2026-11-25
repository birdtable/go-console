// Package shell quotes an argument vector as one line a person pastes into a shell and reads back as the same vector.
// It is built on the standard library alone (no session, terminal or I/O), adds the dialect-correct quoting the
// standard library leaves out, and leaves the Dialect and the argv to the caller. A Dialect has a name (String), and
// ParseDialect reads one back, in any case, for a setting that holds the name. Quote refuses an argument that is not valid UTF-8 (ErrNotUTF8) or holds a rune that does not show as itself (ErrControl) in every dialect, in Cmd and PowerShell a rune Windows best-fit turns into a quote, a backslash or a space (ErrBestFit), and in PowerShell four kinds of
// argument that Windows PowerShell 5.1 passes to a program wrong (ErrPowerShellArg). The Windows dialect is one line that cmd and PowerShell both read as the same argv, or a refusal (ErrWindows) that a caller answers by saying the step in words.
package shell

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Dialect names the shell a line is quoted for.
type Dialect int

// The dialects Quote knows. PowerShell is Windows PowerShell 5.1 and PowerShell 7: the line Quote writes for it
// gives the quoted argv under both, in every $PSNativeCommandArgumentPassing mode (Legacy, Standard, Windows), to a
// native program that reads its command line by the C runtime's rule, and the price is the four refusals of ErrPowerShellArg.
// A batch file (.cmd or .bat, run through cmd.exe) is not such a program: Quote does not make a line safe for one.
// A program that reads its command line as ANSI (main, GetCommandLineA) is given it converted by Windows best-fit: Cmd and PowerShell lines refuse the
// runes that would change where a word ends (ErrBestFit), and a POSIX line pasted into a POSIX shell on Windows that starts such a program is not covered.
// Windows is one line that cmd and PowerShell (5.1, and 7 in every mode) both read as the same argv, for a program that reads its command line by the C runtime's rule: a word that is plain stays bare, any other is in double quotes with nothing escaped, and what no one line serves is refused (ErrWindows).
// The first word is the caller's: in Windows PowerShell an alias wins over a program (curl, wget, sc, where, sort, diff, tee), so a caller names a
// native program with its extension (curl.exe), and Quote does not add it.
const (
	POSIX Dialect = iota + 1
	PowerShell
	Cmd
	Windows
)

// What Quote refuses, by name; the error it returns wraps one of these and says which argument.
var (
	// ErrNUL is an argument that holds a NUL byte, which no shell carries.
	ErrNUL = errors.New("holds a NUL byte")
	// ErrNewline is an argument that holds a raw newline (or a carriage return), which a pasted line cannot carry.
	ErrNewline = errors.New("holds a raw newline")
	// ErrControl is an argument that holds a rune a terminal hides, redraws or shows as nothing: a control (unicode.IsControl: C0 with tab and ESC, DEL, C1),
	// a format character (unicode.Cf: the zero-width, bidi and soft-hyphen set), a line or paragraph separator (Zl, Zp), a default-ignorable rune
	// (Other_Default_Ignorable_Code_Point, Variation_Selector), any space but U+0020 (Zs), the blank braille U+2800 and a few other blanks (U+FFFC, U+1D159, U+16FE4, U+1BC9D, U+1107F, U+11F42, U+13440), a rune no Unicode category holds (unassigned, noncharacters included), or a mark (unicode.M, Other_Grapheme_Extend) that begins an argument or follows U+0020. Private-use runes pass. The person would paste what they did not see.
	ErrControl = errors.New("holds a control or invisible character")
	// ErrNotUTF8 is an argument that is not valid UTF-8: the error names the argument and the byte offset. A shell reads such a byte as it likes, and Cmd would rewrite it to U+FFFD.
	ErrNotUTF8 = errors.New("is not valid UTF-8")
	// ErrCmdChar is, in Cmd, an argument that holds % or !, which cmd expands whatever the quoting.
	ErrCmdChar = errors.New("holds a character cmd expands inside any quoting")
	// ErrPowerShellArg is, in PowerShell, an argument that Windows PowerShell passes to a program wrong whatever the quoting: one that
	// holds a double quote (the four curly ones too), is empty, is --%, or holds whitespace as Unicode defines it and ends in a backslash.
	ErrPowerShellArg = errors.New("holds what Windows PowerShell passes to a program wrong")
	// ErrBestFit is, in Cmd and PowerShell, an argument that holds a rune Windows best-fit turns into a double quote, a backslash, a space or a tab
	// on a Windows ANSI code page (874, 932, 936, 949, 950 or 1250 to 1258): a program that reads its command line as ANSI (main, GetCommandLineA)
	// gets such a rune as that byte, so U+FF02 ends a word as a quote does. The error names the argument and the rune as U+XXXX, never the rune.
	// A rune ErrControl covers answers ErrControl, and a PowerShell curly double quote ErrPowerShellArg. Other runes still reach such a program changed or as ?, which no quoting can help. POSIX passes them.
	ErrBestFit = errors.New("holds a character Windows best-fit turns into a quote, a backslash or a space")
	// ErrWindows is, in Windows, an argument that cmd and PowerShell do not both read as itself in one line: one that holds a $ or a backtick (PowerShell expands them inside double quotes),
	// one that has to be quoted and ends in a backslash (cmd hands the program \" where PowerShell passes the word with no quote, so no one spelling serves both), and a first word that is not plain
	// (PowerShell needs & before a quoted one, which cmd reads as a command separator). A caller says the step in words.
	ErrWindows = errors.New("holds what cmd and PowerShell do not both read as itself")
	// ErrDialect is a Dialect that is none of the four.
	ErrDialect = errors.New("shell: unknown dialect")
)

// Quote is argv as one line for the dialect d. An argument that is plain stays bare, any other is quoted.
// In every dialect it refuses an argument with a NUL (ErrNUL) or a raw newline (ErrNewline), one that is not valid UTF-8
// (ErrNotUTF8, which names the argument and the byte offset), and one with a rune that does not show as itself (ErrControl,
// which names the argument and the rune as U+XXXX): any control (unicode.IsControl: tab, ESC, DEL, the C1 set), any format
// character (all of unicode.Cf: the zero-width set, the bidi embeddings, overrides and isolates, the soft hyphen, and U+200D
// too), the line and paragraph separators, the default-ignorable and variation-selector runes (U+FE0F too, with no emoji
// exception), every space but U+0020 (the no-break space, the hair space and the rest), the blank braille U+2800 and a few
// more blanks (U+FFFC, U+1D159, U+16FE4, U+1BC9D, U+1107F, U+11F42, U+13440), any rune no Unicode category holds (unassigned,
// the noncharacters among them), and a mark (unicode.M, Other_Grapheme_Extend) that begins an argument or follows U+0020; a
// mark after any other rune passes. A line is
// printed for a person to read and paste, so Quote passes no such rune. What it leaves to the caller is three things: the order of right-to-left text (letters, digits and punctuation), which a bidi
// terminal may reorder; characters that look like others (homoglyphs, such as U+FF07 for ' and U+2011 for -); and private-use
// characters (WSL and Cygwin keep : there). Everything else that does not show as itself is refused. Every other character passes as it is. In Cmd it refuses an argument with % or ! (ErrCmdChar), as proc.ErrBatch refuses
// them, and in PowerShell one that holds a double quote (a curly one too), is empty, is --% or holds whitespace as
// Unicode defines it and ends in a backslash (ErrPowerShellArg): Windows PowerShell 5.1 passes such a word to a
// program wrong, so the line would not give it the argv. The line gives the argv to a native program that reads its
// command line by the C runtime's rule; a batch file (.cmd or .bat, run through cmd.exe) is not one, and Quote does
// not make a line safe for it. A program that reads its command line as ANSI (main, GetCommandLineA) gets it converted by Windows best-fit, so in Cmd and
// PowerShell Quote also refuses a rune best-fit turns into a double quote, a backslash, a space or a tab on any Windows ANSI code page (ErrBestFit, a table
// generated on Windows, the same on every system; U+FF02 and U+FF3C are in it, U+FF01 and U+FF08 are not): such a rune would change where a word ends.
// Other runes still reach such a program changed or as ?, which no quoting can help (on code pages 932, 936 and 950 a rune that is two bytes, the second 0x5C, such as U+8868 on 932, ending a quoted word reads as a backslash before the quote to a program that reads its line byte by byte and knows no lead bytes; the C runtime knows them), and a POSIX line is not refused: a POSIX line pasted into a POSIX
// shell on Windows (Git Bash, MSYS, Cygwin) that starts an ANSI program is not covered. An MSYS runtime (Git Bash, MSYS2) may also rewrite a word that looks
// like a path after the shell's parse, which quoting does not stop; the line that switches it off comes with shell.MSYS.
// The refusals hold for every word, the first included. An empty argv is "".
func Quote(d Dialect, argv []string) (string, error) {
	if d < POSIX || d > Windows {
		return "", ErrDialect
	}
	words := make([]string, len(argv))
	for i, a := range argv {
		if err := refuse(d, a, i == 0); err != nil {
			return "", fmt.Errorf("shell: argument %d %w", i, err)
		}
		switch d {
		case PowerShell:
			words[i] = powerShell(a)
		case Cmd:
			words[i] = cmd(a)
		case POSIX:
			words[i] = posix(a, i == 0)
		case Windows:
			words[i] = windows(a)
		}
	}
	line := strings.Join(words, " ")
	// A quoted word first is a string to PowerShell, not a command: the call operator runs it.
	if d == PowerShell && len(words) > 0 && words[0][0] == '\'' {
		line = "& " + line
	}
	return line, nil
}

func refuse(d Dialect, a string, first bool) error {
	switch {
	case strings.IndexByte(a, 0) >= 0:
		return ErrNUL
	case !utf8.ValidString(a):
		return notUTF8(a)
	case strings.ContainsAny(a, "\n\r"):
		return ErrNewline
	case hasHidden(a):
		return controlError(a)
	case (d == Cmd || d == Windows) && strings.ContainsAny(a, "%!"):
		return ErrCmdChar
	case (d == PowerShell || d == Windows) && powerShellWrong(a):
		return ErrPowerShellArg
	case d != POSIX && bestFit(a) != 0:
		return bestFitError(a)
	case d == Windows && strings.ContainsAny(a, "$`"):
		return fmt.Errorf("%w (a $ or a backtick)", ErrWindows)
	case d == Windows && !windowsPlain(a) && (first || strings.HasSuffix(a, `\`)):
		return fmt.Errorf("%w (a first word that is quoted, or a quoted word that ends in a backslash)", ErrWindows)
	}
	return nil
}

// Invisible is whether a person cannot be shown the rune r as it is, the set ErrControl names (a control, NUL, LF and CR included).
// It holds for the rune alone: a mark or a Hangul jamo that begins a text or follows a space is refused by Quote and Shown, not Invisible.
func Invisible(r rune) bool {
	return unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp, unicode.Other_Default_Ignorable_Code_Point, unicode.Variation_Selector) ||
		(unicode.Is(unicode.Zs, r) && r != ' ') || unassigned(r) || blankLike(r)
}

// unassigned is a rune no Unicode category holds (Cn, the noncharacters among them). Private use (Co) is held, and passes (the table C itself holds Cn in Go).
func unassigned(r rune) bool {
	return !unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S, unicode.Z, unicode.Cc, unicode.Cf, unicode.Co, unicode.Cs)
}

// blankLike is a rune that draws as nothing yet is no space, format or default-ignorable character: U+2800 and a hand list.
func blankLike(r rune) bool {
	switch r {
	case 0x2800, 0xFFFC, 0x1D159, 0x16FE4, 0x1BC9D, 0x1107F, 0x11F42, 0x13440:
		return true
	}
	return false
}

// hidden is a rune Quote refuses with ErrControl in the context prev: Invisible, or a mark (unicode.M, Other_Grapheme_Extend)
// that begins an argument or follows U+0020, where it draws over the quote or the space. A mark after any other rune passes.
// The Hangul medial and final jamo (U+1161 to U+11FF, U+D7B0 to U+D7FF) join the marks: they join the syllable before them and, with none, are drawn over the quote or the space.
func hidden(r, prev rune) bool {
	return Invisible(r) || (prev == 0 || prev == ' ') && (unicode.In(r, unicode.M, unicode.Other_Grapheme_Extend) || jamo(r))
}

// jamo is a Hangul medial or final jamo, which a terminal draws joined to the letter before it.
func jamo(r rune) bool { return r >= 0x1161 && r <= 0x11FF || r >= 0xD7B0 && r <= 0xD7FF }

// firstHidden is the first rune of a that hidden reports.
func firstHidden(a string) (rune, bool) {
	var prev rune
	for _, r := range a {
		if hidden(r, prev) {
			return r, true
		}
		prev = r
	}
	return 0, false
}

func hasHidden(a string) bool { _, ok := firstHidden(a); return ok }

// Shown is whether s shows as written: false when it holds an Invisible rune, or a mark or a Hangul jamo that begins s or follows U+0020.
// It is the test Quote applies for ErrControl, so for a valid UTF-8 s with no NUL, LF or CR, Shown(s) is !errors.Is(err, ErrControl) of Quote
// in every dialect. A NUL, LF and CR are not Shown. It says nothing of the other refusals (ErrBestFit, ErrPowerShellArg and the rest).
func Shown(s string) bool { return !hasHidden(s) }

// bestFit is the first rune of a that the generated table holds, or 0.
func bestFit(a string) rune {
	for _, r := range a {
		if _, ok := slices.BinarySearch(bestFitRunes, r); ok {
			return r
		}
	}
	return 0
}

// bestFitError wraps ErrBestFit and names the first such rune as U+XXXX, never the rune itself.
func bestFitError(a string) error {
	return fmt.Errorf("%w (U+%04X)", ErrBestFit, bestFit(a))
}

// notUTF8 wraps ErrNotUTF8 and names the byte offset of the first invalid byte.
func notUTF8(a string) error {
	for i := 0; i < len(a); {
		r, n := utf8.DecodeRuneInString(a[i:])
		if r == utf8.RuneError && n == 1 {
			return fmt.Errorf("%w (byte %d)", ErrNotUTF8, i)
		}
		i += n
	}
	return ErrNotUTF8
}

// controlError wraps ErrControl and names the first such rune as U+XXXX, never the rune itself.
func controlError(a string) error {
	r, _ := firstHidden(a)
	return fmt.Errorf("%w (U+%04X)", ErrControl, r)
}

// powerShellWrong is an argument that Windows PowerShell 5.1 builds into a program's command line wrong: a double quote (it escapes
// none, and reads the four curly ones as one) splits a word or drops the quote, an empty word is dropped and the later words move up,
// and a word with whitespace (unicode.IsSpace, and U+180E, which a .NET version may count, a superset of what 5.1 quotes on) that ends in a
// backslash is wrapped in quotes the backslash then escapes. --% is PowerShell's stop-parsing token, and review found that a quoted one is still read as it, so no quoting carries it.
func powerShellWrong(a string) bool {
	return a == "" || a == "--%" || strings.ContainsAny(a, "\"“”„‟") || (strings.HasSuffix(a, `\`) && strings.ContainsFunc(a, space))
}

// space is whitespace as Unicode defines it, and U+180E, the Mongolian vowel separator, which a .NET version may count as space.
func space(r rune) bool { return unicode.IsSpace(r) || r == 0x180E }

// dashed is a word that begins with - and holds anything after it but letters, digits and -: 5.1 can split such a bare word (PowerShell#6291,
// a : or a . in it), so it is quoted, and a quoted word reaches a program whole.
func dashed(a string) bool {
	return a[0] == '-' && len(a) > 1 && !plain(a[1:], "-")
}

// plain is a word of letters, digits and the characters in extra: nothing to quote.
func plain(a, extra string) bool {
	return a != "" && !strings.ContainsFunc(a, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && !strings.ContainsRune(extra, r)
	})
}

// posix: single quotes, each single quote in a word closes it, is escaped and opens it again. A first word with = would be an assignment, so it is quoted.
// A word that begins with = is quoted too: zsh's EQUALS option, on by default, replaces =name with the path of the command name. An = later in the word stays bare.
func posix(a string, first bool) string {
	extra := "@%+=:,./-_"
	if first {
		extra = "@%+:,./-_"
	}
	if plain(a, extra) && a[0] != '=' {
		return a
	}
	return "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
}

// powerShell: single quotes, each character PowerShell reads as one (the four curly ones too) doubled.
// A comma, a percent sign and an at sign are not bare: they are an array, an alias and a splat.
// A word that begins with - stays bare only if the rest is letters, digits and -; any other dash word is quoted, since 5.1 can split it (PowerShell#6291).
func powerShell(a string) string {
	if plain(a, "+=:./-_") && !dashed(a) {
		return a
	}
	return "'" + strings.NewReplacer("'", "''", "‘", "‘‘", "’", "’’", "‚", "‚‚", "‛", "‛‛").Replace(a) + "'"
}

// windowsPlain is a word both shells leave alone: letters, digits and +=:./\-_, and PowerShell's dash rule.
func windowsPlain(a string) bool { return plain(a, `+=:./\-_`) && !dashed(a) }

// windows: a plain word bare, any other in double quotes with nothing escaped: cmd and PowerShell both read what is inside as text.
func windows(a string) string {
	if windowsPlain(a) {
		return a
	}
	return `"` + a + `"`
}

// cmd: the argument as the C runtime reads an argv word, then a caret before every character cmd reads
// as its own, the double quote among them, so cmd never sees a quoted region and the program is given the argv rule's word.
func cmd(a string) string {
	quote := a == "" || strings.ContainsAny(a, " \t\"")
	var b strings.Builder
	if quote {
		b.WriteByte('"')
	}
	slashes := 0
	for i := 0; i < len(a); i++ {
		c := a[i]
		if c == '\\' {
			slashes++
			continue
		}
		if c == '"' {
			b.WriteString(strings.Repeat(`\`, 2*slashes+1))
		} else {
			b.WriteString(strings.Repeat(`\`, slashes))
		}
		slashes = 0
		b.WriteByte(c)
	}
	if quote {
		b.WriteString(strings.Repeat(`\`, 2*slashes))
		b.WriteByte('"')
	} else {
		b.WriteString(strings.Repeat(`\`, slashes))
	}
	var out strings.Builder
	for _, c := range b.String() {
		if strings.ContainsRune(`^&|<>()"`, c) {
			out.WriteByte('^')
		}
		out.WriteRune(c)
	}
	return out.String()
}
