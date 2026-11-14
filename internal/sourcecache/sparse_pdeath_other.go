//go:build !linux && !windows

package sourcecache

import "syscall"

func dieWithParent(*syscall.SysProcAttr) {}
