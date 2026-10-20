//go:build !linux

package hostdiag

func collect(string, Info) (string, error) { return "", ErrUnsupported }
