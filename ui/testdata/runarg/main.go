// Command runarg runs its arguments as a program, as a child, and exits with its status. Built under the name of
// a shell, it stands in for that shell in the parent chain of the program it runs, with no shell installed. With
// -g as its first argument it starts the child in a new process group, as a shell with job control does; without
// it the child shares its group, as a wrapper script's child does. -g does nothing on Windows.
package main

import (
	"errors"
	"os"
	"os/exec"
)

// newGroup makes cmd start in a process group of its own, on a system that has process groups.
var newGroup = func(*exec.Cmd) {}

func main() {
	args := os.Args[1:]
	group := len(args) > 0 && args[0] == "-g"
	if group {
		args = args[1:]
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if group {
		newGroup(cmd)
	}
	err := cmd.Run()
	if exit := (*exec.ExitError)(nil); errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	} else if err != nil {
		os.Exit(1)
	}
}
