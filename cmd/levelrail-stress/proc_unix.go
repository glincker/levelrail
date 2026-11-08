//go:build !windows

package main

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// ownProcessGroup lets teardown signal the control plane and anything it spawned together.
func ownProcessGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func signalGroup(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, sig)
	}
}

// rssKB asks ps, which works on both Linux and macOS without cgo.
func rssKB(pid int) int {
	out, err := exec.CommandContext(context.Background(), "ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // fixed argv, pid is our own child
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
}
