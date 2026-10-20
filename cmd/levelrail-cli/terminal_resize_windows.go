//go:build windows

package main

import "os"

// notifyResize has no Windows equivalent (there is no SIGWINCH), so a
// session started there keeps the size it opened with.
func notifyResize() (<-chan os.Signal, func()) {
	return make(chan os.Signal), func() {}
}
