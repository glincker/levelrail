//go:build unix

package cliconsole

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

// dropCredential returns the uid/gid the child runs as. Only a root control
// plane can switch users; it must, so the CLI never runs as root. A non-root
// control plane runs the child as itself (nil).
func dropCredential(name string) (*syscall.Credential, error) {
	if os.Geteuid() != 0 {
		return nil, nil
	}
	u, err := user.Lookup(name)
	if err != nil {
		return nil, fmt.Errorf("cli console: running as root needs an unprivileged user %q (%s): %w", name, EnvRunAsUser, err)
	}
	uid, uerr := strconv.ParseUint(u.Uid, 10, 32)
	gid, gerr := strconv.ParseUint(u.Gid, 10, 32)
	if uerr != nil || gerr != nil || uid == 0 {
		return nil, fmt.Errorf("cli console: user %q is not a usable unprivileged account", name)
	}
	return &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: []uint32{}}, nil
}

func chownForChild(dir string, cred *syscall.Credential) error {
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // a directory needs the x bit; owner only
		return fmt.Errorf("cli console: chmod temp dir: %w", err)
	}
	if cred == nil {
		return nil
	}
	if err := os.Chown(dir, int(cred.Uid), int(cred.Gid)); err != nil {
		return fmt.Errorf("cli console: chown temp dir: %w", err)
	}
	return nil
}

// configureProcess puts the child in its own session (and so process group)
// with no controlling terminal, so cancel and timeout kill everything it started.
func configureProcess(cmd *exec.Cmd, cred *syscall.Credential) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Credential: cred}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
