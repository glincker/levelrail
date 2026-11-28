package dockerguard

import (
	"regexp"
	"strings"
)

// Grant tokens ride in a service's granted caps next to docker_api tokens.
const (
	HostNetworkGrantCap = "HOST_NETWORK"
	DeviceGrantPrefix   = "DEVICE:"
)

// grantableDevice is the whole device allowlist. Anything else, including block
// devices, /dev/mem, /dev/kmem, /dev/fuse and /dev/kvm, simply never matches.
var grantableDevice = regexp.MustCompile(`^/dev/(dri(/[A-Za-z0-9_-]+)?|ttyUSB[0-9]+|ttyACM[0-9]+|video[0-9]+)$`)

// GrantableDevice reports whether p is a device node a devices grant may name.
// The match is on the literal string, so dot segments, globs and symlink-style paths never pass.
func GrantableDevice(p string) bool {
	if strings.Contains(p, "..") || strings.Contains(p, "//") {
		return false
	}
	return grantableDevice.MatchString(p)
}

// DeviceGrantCap is the granted-cap token for a device node path.
func DeviceGrantCap(p string) string { return DeviceGrantPrefix + p }

// IsHostNetworkGrant reports whether c is the host network token.
func IsHostNetworkGrant(c string) bool {
	return strings.EqualFold(strings.TrimSpace(c), HostNetworkGrantCap)
}

// IsDeviceGrant reports whether c is a device token.
func IsDeviceGrant(c string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(c)), DeviceGrantPrefix)
}

// IsAnyGrantCap reports whether c is a grant token of any kind rather than a Linux capability.
func IsAnyGrantCap(c string) bool {
	return IsGrantCap(c) || IsHostNetworkGrant(c) || IsDeviceGrant(c)
}

// HasHostNetworkGrant reports whether caps carry the host network token.
func HasHostNetworkGrant(caps []string) bool {
	for _, c := range caps {
		if IsHostNetworkGrant(c) {
			return true
		}
	}
	return false
}

// DevicesFromCaps returns the allowlisted device paths the caps grant.
func DevicesFromCaps(caps []string) []string {
	var out []string
	for _, c := range caps {
		if !IsDeviceGrant(c) {
			continue
		}
		p := strings.TrimSpace(c)[len(DeviceGrantPrefix):]
		if GrantableDevice(p) {
			out = append(out, p)
		}
	}
	return out
}
