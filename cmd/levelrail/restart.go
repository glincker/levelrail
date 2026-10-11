package main

import (
	"errors"
	"os"
)

// exitRestart is a non-zero status so systemd's Restart=on-failure and
// Docker's on-failure policy relaunch the process; a clean 0 would not.
const exitRestart = 75

var errRestartRequested = errors.New("restart requested")

// restartSupervised reports whether something will relaunch this process
// after it exits. APP_RESTART_SUPERVISED overrides detection.
func restartSupervised(lookup func(string) (string, bool), exists func(string) bool) bool {
	if v, ok := lookup("APP_RESTART_SUPERVISED"); ok {
		return v == "1" || v == "true"
	}
	if _, ok := lookup("INVOCATION_ID"); ok {
		return true
	}
	return exists("/.dockerenv")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
