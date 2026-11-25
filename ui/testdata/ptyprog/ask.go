package main

import (
	"os"

	"charm.land/huh/v2"

	"github.com/birdtable/go-console/ui"
)

// askMenu is the bare run with PTYPROG=ask: a Session.Ask of a select of three, whose ending, when it is
// backed out of, is the closing line the pty test reads. The closing word lands on the last row, so a
// cursor moved to the middle one stays on the screen to be read.
func askMenu(run *ui.Session) ([]string, bool) {
	var got string
	f := huh.NewForm(huh.NewGroup(huh.NewSelect[string]().
		Title("Which region?").
		Options(huh.NewOption("north", "north"), huh.NewOption("south", "south"), huh.NewOption("west", "west")).
		Value(&got)))
	if e := run.Ask(f, "--region"); e != nil {
		_ = run.End(e)
		os.Exit(e.Code())
	}
	return nil, false
}
