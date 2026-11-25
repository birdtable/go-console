// Command heredialect prints ui.HereDialect() as a number, for the test that runs it under each shell. An error
// goes to stderr and the exit status is 1.
package main

import (
	"fmt"
	"os"

	"github.com/birdtable/go-console/ui"
)

func main() {
	d, err := ui.HereDialect()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(int(d))
}
