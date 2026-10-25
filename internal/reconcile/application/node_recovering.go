package application

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
)

// ReasonNodeRecovering is the Ready=False reason while a node that just came
// back has not yet made its mesh address assignable.
const ReasonNodeRecovering = "NodeRecovering"

// WithNodeRecovering tells the controller its node reconnected recently, so an
// address-not-ready bind error is transient rather than a failed create.
func WithNodeRecovering(recovering bool) Option {
	return func(c *Controller) { c.nodeRecovering = recovering }
}

func isAddressNotReady(err error) bool {
	if errors.Is(err, syscall.EADDRNOTAVAIL) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "cannot assign requested address") || strings.Contains(msg, "address not available")
}

// createFailure picks the reason for a failed create, and reports a bind on a
// not-yet-assignable address of a recovering node as NodeRecovering.
func (c *Controller) createFailure(err error) (string, error) {
	if c.nodeRecovering && isAddressNotReady(err) {
		return ReasonNodeRecovering, fmt.Errorf("node recovered, waiting for the mesh address: %w", err)
	}
	return createFailureReason(err), err
}
