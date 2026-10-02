package firewall

import (
	"errors"
	"fmt"
)

// DefaultRequiredPorts are this platform's own ports when run with every
// default address: the management API (APP_HTTP_ADDR), the agent's
// inbound mTLS gRPC listener (APP_AGENT_ADDR), and the two embedded-Caddy
// ingress ports every deployed app depends on (APP_INGRESS_HTTP_ADDR,
// APP_INGRESS_HTTPS_ADDR). cmd/levelrail passes the actually configured
// ports to Validate; this slice is only the fallback for callers (tests,
// a Manager built without that wiring) that have none.
var DefaultRequiredPorts = []int{8080, 9443, 80, 443}

// ErrWouldLockOut is wrapped by Validate's error when a rule would deny,
// or narrow to a specific source, inbound access to one of the
// platform's own required ports. Refused unconditionally: a locked-out
// control plane has no way to fix itself back open.
var ErrWouldLockOut = errors.New("firewall: rule would lock out a required platform port")

// Validate refuses a Rule that would deny, or CIDR-restrict, inbound
// access to any port in requiredPorts. A CIDR-scoped allow is unsafe
// too: it implicitly excludes every other source that port needs.
func Validate(r Rule, requiredPorts []int) error {
	for _, p := range requiredPorts {
		if r.Port != p {
			continue
		}
		if r.action() == ActionDeny {
			return fmt.Errorf("%w: port %d/%s is required by the control plane (management API, agent connections, or ingress) and cannot be denied", ErrWouldLockOut, r.Port, r.proto())
		}
		if normalizeSource(r.SourceCIDR) != "" {
			return fmt.Errorf("%w: port %d/%s is required by the control plane and cannot be restricted to a single source CIDR", ErrWouldLockOut, r.Port, r.proto())
		}
	}
	return nil
}
