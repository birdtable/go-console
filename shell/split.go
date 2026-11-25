package shell

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotQuoted is a line Split refuses: one that Quote(d, argv) could not have written for any argv. The error says
// the byte offset of the first thing wrong, and what it is.
var ErrNotQuoted = errors.New("shell: not a line Quote writes")

// Split is the argv that Quote(d, argv) wrote as line, for POSIX, PowerShell and Windows: the inverse
// of Quote, and no more. It reads the words and the quotes Quote writes and nothing else, and it expands nothing: a
// line with an unclosed quote, an operator, a $ or a backtick, a glob, a comment, a backslash form Quote never writes, a
// word with more space than one between, a bare word Quote would have quoted, or a word Quote would refuse is
// ErrNotQuoted, with the offset. So Split(d, Quote(d, a)) is a wherever Quote accepts a, and Quote(d, Split(d, line)) is line
// whenever Split accepts it. The empty line is the empty argv. Cmd is not read (its carets are a second layer, and a
// line for it is for cmd.exe alone): it is ErrDialect, as is any other value that is none of the three.
func Split(d Dialect, line string) ([]string, error) {
	if d != POSIX && d != PowerShell && d != Windows {
		return nil, fmt.Errorf("%w: Split reads posix, powershell and windows, not %s", ErrDialect, d)
	}
	words, starts, err := lex(d, line)
	if err != nil {
		return nil, err
	}
	if back, err := Quote(d, words); err != nil {
		at := 0
		if n := argumentOf(err); n >= 0 && n < len(starts) {
			at = starts[n]
		}
		return nil, notQuoted(at, "a word Quote refuses: "+err.Error())
	} else if back != line {
		at := 0
		for at < len(line) && at < len(back) && line[at] == back[at] {
			at++
		}
		return nil, notQuoted(at, "Quote writes this argv another way")
	}
	return words, nil
}

func notQuoted(at int, why string) error {
	return fmt.Errorf("%w (byte %d: %s)", ErrNotQuoted, at, why)
}

// argumentOf is the index in "shell: argument N ..." that Quote's error names, or -1.
func argumentOf(err error) int {
	var n int
	if _, e := fmt.Sscanf(err.Error(), "shell: argument %d ", &n); e != nil {
		return -1
	}
	return n
}

// lex reads the words as a shell would with no expansion: a word is bare bytes and quoted runs, ended by a space that
// is outside a quote. What it reads is only what Split then holds against Quote.
func lex(d Dialect, line string) (words []string, starts []int, err error) {
	i := 0
	if d == PowerShell && strings.HasPrefix(line, "& ") {
		i = 2
	}
	for i < len(line) {
		if line[i] == ' ' {
			return nil, nil, notQuoted(i, "an empty word or more than one space")
		}
		var w strings.Builder
		starts = append(starts, i)
		for i < len(line) && line[i] != ' ' {
			var step int
			if step, err = lexAt(d, line, i, &w); err != nil {
				return nil, nil, err
			}
			i += step
		}
		words = append(words, w.String())
		if i < len(line) {
			i++
			if i == len(line) {
				return nil, nil, notQuoted(i-1, "a space at the end")
			}
		}
	}
	if words == nil {
		words = []string{}
	}
	return words, starts, nil
}

// lexAt reads one piece of a word at i, bare or quoted, into w, and says how many bytes it took.
func lexAt(d Dialect, line string, i int, w *strings.Builder) (int, error) {
	switch c := line[i]; {
	case d == POSIX && c == '\\' && strings.HasPrefix(line[i:], `\'`):
		w.WriteByte('\'')
		return 2, nil
	case d == POSIX && c == '\'':
		end := strings.IndexByte(line[i+1:], '\'')
		if end < 0 {
			return 0, notQuoted(i, "an unclosed quote")
		}
		w.WriteString(line[i+1 : i+1+end])
		return end + 2, nil
	case d == PowerShell && c == '\'':
		return powerShellQuoted(line, i, w)
	case d == Windows && c == '"':
		end := strings.IndexByte(line[i+1:], '"')
		if end < 0 {
			return 0, notQuoted(i, "an unclosed quote")
		}
		w.WriteString(line[i+1 : i+1+end])
		return end + 2, nil
	}
	w.WriteByte(line[i])
	return 1, nil
}

// powerShellQuoted reads '...' at i, where a quote and each curly single quote stand doubled for themselves.
func powerShellQuoted(line string, i int, w *strings.Builder) (int, error) {
	for j := i + 1; j < len(line); {
		q, ok := psQuoteAt(line[j:])
		switch {
		case !ok:
			w.WriteByte(line[j])
			j++
		case strings.HasPrefix(line[j+len(q):], q):
			w.WriteString(q)
			j += 2 * len(q)
		case q == "'":
			return j + 1 - i, nil
		default:
			return 0, notQuoted(j, "a curly quote that is not doubled")
		}
	}
	return 0, notQuoted(i, "an unclosed quote")
}

// psQuoteAt is the quote PowerShell reads as one (the straight single quote and the four curly ones) that s begins with.
func psQuoteAt(s string) (string, bool) {
	for _, q := range []string{"'", "‘", "’", "‚", "‛"} {
		if strings.HasPrefix(s, q) {
			return q, true
		}
	}
	return "", false
}
