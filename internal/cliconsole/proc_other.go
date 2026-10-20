//go:build !unix

package cliconsole

import (
	"errors"
	"os/exec"
)

type credential struct{}

func dropCredential(string) (*credential, error) {
	return nil, errors.New("cli console: only supported on unix hosts")
}

func chownForChild(string, *credential) error { return nil }

func configureProcess(*exec.Cmd, *credential) {}
