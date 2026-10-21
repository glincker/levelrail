package api

import (
	"context"
	"fmt"
)

// doctorCheckInstallState reports the install's upgrade state. It is
// informational: a development build ahead of every release is not a fault.
func (rt *Router) doctorCheckInstallState(ctx context.Context) doctorCheckResource {
	const code, name = "install_state", "Upgrade state"
	release, known := rt.latestReleaseForCurrentChannel(ctx)
	pre := rt.evaluatePreflight(ctx, release, known && release != nil)
	if pre.State == UpdateStateUnknown {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: pre.StateReason}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%s: %s", pre.State, pre.StateReason)}
}
