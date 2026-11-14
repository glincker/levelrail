//go:build linux

package sourcecache

import "syscall"

// The child is killed if the server dies, so no git outlives it.
func dieWithParent(a *syscall.SysProcAttr) { a.Pdeathsig = syscall.SIGKILL }
