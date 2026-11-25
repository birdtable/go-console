package ui

import "golang.org/x/sys/windows"

// unicodeOf is whether the console draws UTF-8: its output code page is 65001. The locale names play no part
// on Windows. Where there is no console the call fails and the answer is no.
func unicodeOf([]string, EnvRules) bool {
	cp, err := windows.GetConsoleOutputCP()
	return err == nil && cp == 65001
}
