// Command removed names each line of the promise index at the last tag that the head's index lacks and the declared file does
// not list (api/contract.txt). It is run by `task api` with three files: the index at the base tag, the
// index at the head, and api/breaks/<base tag>.contract.txt (empty when there is none).
package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

// lost is each line of old that cur lacks and declared does not list.
func lost(old, cur, declared string) []string {
	var out []string
	kept := strings.Split(cur, "\n")
	listed := strings.Split(declared, "\n")
	for l := range strings.SplitSeq(old, "\n") {
		if l == "" || slices.Contains(kept, l) || slices.Contains(listed, l) {
			continue
		}
		out = append(out, l)
	}
	return out
}

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: removed <base tag> <old> <current> <declared>")
		os.Exit(2)
	}
	var text [3]string
	for i := range text {
		b, err := os.ReadFile(os.Args[i+2])
		if err != nil {
			fmt.Fprintln(os.Stderr, "api:", err)
			os.Exit(1)
		}
		text[i] = string(b)
	}
	bad := lost(text[0], text[1], text[2])
	for _, l := range bad {
		fmt.Fprintf(os.Stderr, "api: api/contract.txt lost a line since %s, and api/breaks/%s.contract.txt does not list it: %s\n", os.Args[1], os.Args[1], l)
	}
	if len(bad) > 0 {
		os.Exit(1)
	}
}
