//go:build windows

package ingress

import "errors"

// systemd socket activation does not exist on Windows; these keep the package
// compiling for the CLI, which imports it only for access log types.
func closeOnExec(int) {}

func dupFD(int) (int, error) { return 0, errors.New("socket inheritance is not supported on windows") }

func closeFD(int) {}
