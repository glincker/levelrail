// Package bindaddr resolves the small bind-address vocabulary app
// services and managed databases use to choose which network interface
// a published port binds to. Shared by internal/spec, internal/api,
// internal/store, and internal/reconcile so all four layers validate
// and resolve it identically rather than drifting apart.
package bindaddr

import (
	"fmt"
	"net"
)

const (
	// Private binds a published port to loopback only, unreachable from
	// any other host. The default whenever a caller's value is empty.
	Private = "private"
	// Public binds a published port to every interface, reachable from
	// any network that can route to this host. Requires this exact
	// literal value: Resolve never treats an empty or malformed value as
	// Public, the actual security gap this package exists to close.
	Public = "public"
	// Default is what Resolve returns for an empty value.
	Default = Private

	privateIP = "127.0.0.1"
	publicIP  = "0.0.0.0"
)

// Resolve turns value into the literal IP Docker should bind a published
// port to. Private and Public expand to their concrete address; anything
// else must already be a valid IP literal (a specific interface, or a
// WireGuard mesh peer address once internal/network's mesh is wired to
// this); empty resolves to Default, never to Public.
func Resolve(value string) (string, error) {
	switch value {
	case "":
		return privateIP, nil
	case Private:
		return privateIP, nil
	case Public:
		return publicIP, nil
	}
	if net.ParseIP(value) == nil {
		return "", fmt.Errorf("bindaddr: %q is not %q, %q, or a valid IP address", value, Private, Public)
	}
	return value, nil
}

// Validate reports whether value is acceptable input for Resolve,
// without needing the resolved IP itself.
func Validate(value string) error {
	_, err := Resolve(value)
	return err
}
