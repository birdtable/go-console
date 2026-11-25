package ui

import (
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

// On Windows the code page says Unicode, not the locale names: Detect's answer is the console's output code page
// being 65001, and a locale in the environment changes nothing.
//
// What it costs when this fails: a console in UTF-8 gets the ASCII forms, or one on a legacy code page gets
// blocks it prints as question marks.
//
// Not parallel: the console's output code page is process-wide state. This test sets it (437, then 65001) and
// every parallel test that reads it through Detect would see those values, and one that sets it would race this
// one. It runs serially and puts the page back in t.Cleanup.
func TestOnWindowsTheCodePageSaysUnicode(t *testing.T) { //nolint:paralleltest // serial: it sets the console's output code page, which is process-wide
	cp32, err := windows.GetConsoleOutputCP()
	if err != nil {
		cp32 = 0 // no console, so the page reads 0
	}
	cp := uintptr(cp32)
	want := cp == 65001
	for _, env := range [][]string{nil, {"LANG=en_US.UTF-8"}, {"LC_ALL=C"}} {
		if got := Detect(nil, nil, nil, env, StandardEnv()).Unicode; got != want {
			t.Errorf("code page %d, env %v: Unicode is %v, want %v", cp, env, got, want)
		}
	}
	// Where the process has a console the page can be set, and the read follows it: 437 is no Unicode, 65001 is.
	k := syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleOutputCP")
	if cp == 0 {
		return // no console, so the page reads 0 and cannot be set; the read above is all there is to check
	}
	t.Cleanup(func() { _, _, _ = k.Call(cp) })
	for page, uni := range map[uintptr]bool{437: false, 65001: true} {
		if r, _, _ := k.Call(page); r == 0 {
			return // the console refuses a code page, so there is nothing to read back
		}
		if got := unicodeOf(nil, StandardEnv()); got != uni {
			t.Errorf("code page %d: unicodeOf is %v, want %v", page, got, uni)
		}
	}
}
