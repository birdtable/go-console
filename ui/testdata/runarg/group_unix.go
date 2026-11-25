//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func init() {
	newGroup = func(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
}
