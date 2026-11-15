//go:build windows

package sourcecache

import "os/exec"

func groupKill(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Kill() }
}
