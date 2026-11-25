package bestfit

import (
	"fmt"
	"slices"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// finding is a rune and the byte best-fit gave for it; the scan gathers the code pages of each.
type finding struct {
	r rune
	b byte
}

// Hits is every finding of the scan, by rune and then by byte, ascending. Scan is its runes. The two calls it makes
// are kernel32's, reached through the standard library's syscall and not golang.org/x/sys, so the module takes no
// requirement for a generator.
func Hits() ([]Hit, error) {
	convert, err := converter()
	if err != nil {
		return nil, err
	}
	found := map[finding][]int{}
	for _, page := range Pages() {
		for r := rune(0x80); r <= 0x10FFFF; r++ {
			if r >= 0xD800 && r <= 0xDFFF {
				continue
			}
			b, one, err := bestFit(convert, page, r)
			if err != nil {
				return nil, err
			}
			if one && slices.Contains(Bytes(), b) {
				found[finding{r, b}] = append(found[finding{r, b}], page)
			}
		}
	}
	hits := make([]Hit, 0, len(found))
	for f, pages := range found {
		hits = append(hits, Hit{Rune: f.r, Byte: f.b, Pages: pages})
	}
	slices.SortFunc(hits, func(a, b Hit) int {
		if a.Rune != b.Rune {
			return int(a.Rune - b.Rune)
		}
		return int(a.Byte) - int(b.Byte)
	})
	return hits, nil
}

// converter is the address of WideCharToMultiByte, once Windows has said it knows every code page of Pages: a machine
// that lacks one answers an error here, before any rune is asked.
func converter() (uintptr, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	valid, convert := kernel32.NewProc("IsValidCodePage"), kernel32.NewProc("WideCharToMultiByte")
	for _, proc := range []*syscall.LazyProc{valid, convert} {
		if err := proc.Find(); err != nil {
			return 0, fmt.Errorf("bestfit: %w", err)
		}
	}
	for _, page := range Pages() {
		ok, _, _ := syscall.SyscallN(valid.Addr(), uintptr(page)) //nolint:errcheck // IsValidCodePage answers yes or no and sets no error
		if ok == 0 {
			return 0, fmt.Errorf("bestfit: Windows does not know code page %d here, so the set would be short", page)
		}
	}
	return convert.Addr(), nil
}

// bestFit is what WideCharToMultiByte, at the address convert, gives for r on the code page: the first byte, and whether
// the answer was exactly one byte. The call takes no flags, so best-fit is on, and no default character, so the code
// page's own stands in for a rune it cannot show. A rune above U+FFFF goes in as its UTF-16 pair.
func bestFit(convert uintptr, page int, r rune) (byte, bool, error) {
	var pair [2]uint16
	in := utf16.AppendRune(pair[:0], r)
	var out [8]byte
	n, _, errno := syscall.SyscallN(convert,
		uintptr(page), 0,
		uintptr(unsafe.Pointer(&in[0])), uintptr(len(in)), //nolint:gosec // the UTF-16 units the call reads, alive for the call
		uintptr(unsafe.Pointer(&out[0])), uintptr(len(out)), //nolint:gosec // the bytes the call writes, alive for the call
		0, 0)
	if n == 0 {
		return 0, false, fmt.Errorf("bestfit: WideCharToMultiByte for U+%04X on code page %d: %w", r, page, errno)
	}
	return out[0], n == 1, nil
}
