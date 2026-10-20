//go:build unix

package dockerssh

import "syscall"

const noFollowFlag = syscall.O_NOFOLLOW
