package main

import "syscall"

// A person at a Windows console who wants the banner and the borders has chosen UTF-8 (chcp 65001); a
// pseudoconsole starts in the machine's own code page (850 on some), where ui rightly draws ASCII.
func init() {
	_, _, _ = syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleOutputCP").Call(65001)
}
