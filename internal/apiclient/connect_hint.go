package apiclient

import (
	"errors"
	"syscall"
)

// connectHint tells the operator how to point the CLI at the right
// server when it never reached one. Empty for other failures.
func connectHint(err error) string {
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) {
		return " (could not reach the control plane; set " + EnvAPIURL + " or pass --api-url to the address the service listens on, see APP_HTTP_ADDR in the unit)"
	}
	return ""
}
