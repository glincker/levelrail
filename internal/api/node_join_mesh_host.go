package api

import "net/netip"

// meshAdvertiseHostNeedsIP is true for an advertise host WireGuard cannot use:
// it never resolves names, so a mesh enrolment against one would not handshake.
func meshAdvertiseHostNeedsIP(host string) bool {
	if host == "" || host == "localhost" {
		return false
	}
	_, err := netip.ParseAddr(host)
	return err != nil
}

const meshAdvertiseHostMessage = "the mesh needs an IP address as the advertise host, but it is set to a hostname (%s): WireGuard does not resolve names, so the node would enrol and never handshake. " +
	"Set APP_AGENT_ADVERTISE_HOST to this server's public IP, or enrol without the mesh."
