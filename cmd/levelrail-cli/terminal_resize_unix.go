//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// notifyResize delivers a value whenever the controlling terminal
// changes size, plus the function that stops delivering.
func notifyResize() (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	return ch, func() { signal.Stop(ch) }
}
