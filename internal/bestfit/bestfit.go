// Package bestfit asks Windows which runes its best-fit conversion turns into a byte that moves a word boundary on a
// program's ANSI command line: the double quote, the backslash, the space and the tab. A program that reads its command
// line as ANSI (main, GetCommandLineA) is handed the line converted to the system's ANSI code page, and on that page a
// rune with no character of its own is given the nearest one: U+FF02, the fullwidth quotation mark, becomes ". The scan
// asks WideCharToMultiByte itself, for every rune and every code page of Pages, so the set is what Windows does today and
// not a table copied from a document. tools/bestfit writes the set as shell's table, and a Windows test in shell holds
// the table to the scan. It is internal, so it is no part of the module's API, and it runs on Windows only: on any other
// system Scan and Hits answer ErrNotWindows.
package bestfit

import (
	"errors"
	"slices"
)

// ErrNotWindows is what Scan and Hits answer on a system that is not Windows: the conversion asked is Windows's own.
var ErrNotWindows = errors.New("bestfit: the scan asks Windows's own conversion (WideCharToMultiByte), so it runs on Windows only")

// Pages is the Windows ANSI code pages the scan asks, ascending: Thai (874), Japanese (932), simplified Chinese (936),
// Korean (949), traditional Chinese (950) and the nine of 1250 to 1258. Each call returns a new slice.
func Pages() []int {
	return []int{874, 932, 936, 949, 950, 1250, 1251, 1252, 1253, 1254, 1255, 1256, 1257, 1258}
}

// Bytes is the four bytes of the rule, ascending: the tab, the space, the double quote and the backslash. They are the
// bytes the C runtime's argv rule reads, so the only ones that move where a word ends. Each call returns a new slice.
func Bytes() []byte {
	return []byte{0x09, 0x20, 0x22, 0x5C}
}

// Hit is one finding of the scan: best-fit turns Rune into the single byte Byte on each code page of Pages, ascending.
type Hit struct {
	Rune  rune
	Byte  byte
	Pages []int
}

// Scan is the set, ascending and each rune once: every rune from U+0080 to U+10FFFF, the surrogates left out, that
// WideCharToMultiByte with no flags and no default character turns into exactly one byte, a byte of Bytes, on a code
// page of Pages. It fails, and answers no runes, when Windows does not know one of the code pages or a conversion
// fails, so a machine that cannot ask all of them never yields a short set.
func Scan() ([]rune, error) {
	hits, err := Hits()
	if err != nil {
		return nil, err
	}
	var runes []rune
	for _, h := range hits {
		runes = append(runes, h.Rune)
	}
	return slices.Compact(runes), nil
}
