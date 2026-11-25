// Command wideline writes one line of 100 columns and exits: a program that overruns a pty narrower than that.
package main

import (
	"os"
	"strings"
)

func main() {
	_, _ = os.Stdout.WriteString(strings.Repeat("x", 100) + "\r\n")
}
