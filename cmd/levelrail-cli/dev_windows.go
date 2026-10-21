//go:build windows

package main

import "errors"

func processAlive(pid int) bool { return false }

func terminateProcess(pid int) error {
	return errors.New("stopping from another terminal is not supported on Windows: press Ctrl-C in the terminal running dev")
}
