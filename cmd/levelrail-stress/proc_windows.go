//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

func ownProcessGroup(*exec.Cmd) {}

func signalGroup(cmd *exec.Cmd, _ syscall.Signal) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func rssKB(int) int { return 0 }
