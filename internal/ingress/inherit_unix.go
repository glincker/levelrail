//go:build !windows

package ingress

import "syscall"

func closeOnExec(fd int) { syscall.CloseOnExec(fd) }

func dupFD(fd int) (int, error) { return syscall.Dup(fd) }

func closeFD(fd int) { _ = syscall.Close(fd) }
