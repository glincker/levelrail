//go:build !windows

package sourcecache

import (
	"os/exec"
	"syscall"
)

// A own process group lets cancel reach git-remote-https and index-pack too.
func groupKill(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
