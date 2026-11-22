//go:build windows

package serverready

import "errors"

// The readiness probe targets Linux servers, so Windows reports no figure.
func diskFreeGB(_ string) (float64, error) {
	return 0, errors.New("disk free probe is not supported on windows")
}
