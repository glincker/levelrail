package network

// This file: SystemLinkConfigurator, a real implementation of the
// LinkConfigurator seam device.go documents and deliberately leaves
// unimplemented there ("a netlink dependency or a few hundred lines of.

import (
	"context"
	"fmt"
	"net/netip"
	"os/exec"
	"runtime"
)

// SystemLinkConfigurator is the real LinkConfigurator: it runs this
// machine's own ip(8) (Linux) or ifconfig(8) (Darwin/BSD) to address and
// bring up iface. Unsupported platforms (anything else GOOS reports) get
// a clear error rather than a silent no-op, so a node running on one logs
// exactly the warning device.go's own configureLink already produces for
// "no LinkConfigurator at all," not a different, more confusing failure.
type SystemLinkConfigurator struct{}

// SetAddress implements LinkConfigurator.
func (SystemLinkConfigurator) SetAddress(ctx context.Context, iface string, addr netip.Prefix) error {
	if !addr.IsValid() {
		return fmt.Errorf("network: set address on %q: invalid prefix", iface)
	}
	switch runtime.GOOS {
	case "darwin":
		return setAddressDarwin(ctx, iface, addr)
	case "linux":
		return setAddressLinux(ctx, iface, addr)
	default:
		return fmt.Errorf("network: SystemLinkConfigurator does not support GOOS %q", runtime.GOOS)
	}
}

// setAddressDarwin configures a point-to-point utun interface the way
// wg-quick's own macOS backend does: "ifconfig utunN inet <addr> <addr>
// netmask <mask>", the same local==remote form a utun device requires
// (it has no broadcast segment, so there is no separate peer address to
// name), then a separate "up".
func setAddressDarwin(ctx context.Context, iface string, addr netip.Prefix) error {
	ip := addr.Addr().String()
	mask := maskString(addr.Bits())
	if out, err := exec.CommandContext(ctx, "ifconfig", iface, "inet", ip, ip, "netmask", mask).CombinedOutput(); err != nil { //nolint:gosec // iface/ip/mask come from this node's own TUN device and mesh CIDR, not external input
		return fmt.Errorf("network: ifconfig %s inet %s %s netmask %s: %w: %s", iface, ip, ip, mask, err, out)
	}
	if out, err := exec.CommandContext(ctx, "ifconfig", iface, "up").CombinedOutput(); err != nil { //nolint:gosec // iface comes from this node's own TUN device, not external input
		return fmt.Errorf("network: ifconfig %s up: %w: %s", iface, err, out)
	}
	return nil
}

// maskString renders a prefix length as a dotted-decimal IPv4 netmask
// ("/16" -> "255.255.0.0"), the form ifconfig(8) requires and the one
// value netip has no built-in conversion for (this package otherwise
// covers every address operation through netip alone, network.go's own
// header comment). validateCIDR (config.go) already limits bits to
// 0-30 for any mesh CIDR that reaches here, so no bounds check is needed
// beyond the uint32 shift itself.
func maskString(bits int) string {
	var m uint32
	if bits > 0 {
		m = ^uint32(0) << (32 - bits)
	}
	return netip.AddrFrom4([4]byte{byte(m >> 24), byte(m >> 16), byte(m >> 8), byte(m)}).String()
}
