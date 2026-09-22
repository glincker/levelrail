package network

// This file: SystemLinkConfigurator, a real implementation of the
// LinkConfigurator seam device.go documents and deliberately leaves
// unimplemented there ("a netlink dependency or a few hundred lines of
// platform-specific, root-only, untestable syscall marshalling. Neither
// belongs in the same change as the mesh's decision logic"). That
// reasoning still holds for a *netlink-based* implementation; this is a
// smaller, different thing. wg-quick(8) itself does not talk to netlink
// directly either, on any platform: it shells out to ip(8)/ifconfig(8),
// the same system tools every operator's own manual "bring this
// interface up" muscle memory already uses. This file does exactly that,
// nothing more: two short external-command invocations per OS, small
// enough for a reviewer to see it is a faithful translation of "assign
// addr to iface and bring it up," the identical "auditable in a few
// lines" bar NewDevice's own doc comment sets for what belongs here
// versus what needs its own dedicated change.
//
// Requires the process already hold whatever privilege creating the TUN
// device itself required (CAP_NET_ADMIN, or root on a platform that has
// no finer-grained capability); this file does not change that
// requirement, only fulfills the interface configuration NewDevice's own
// doc comment says nothing here does without a LinkConfigurator supplied.

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

// setAddressLinux uses ip(8)'s "replace" form rather than "add":
// idempotent by construction (LinkConfigurator's own contract requires
// it), where "ip address add" would instead fail the second time the
// same address is applied on an unchanged reconcile pass.
func setAddressLinux(ctx context.Context, iface string, addr netip.Prefix) error {
	if out, err := exec.CommandContext(ctx, "ip", "address", "replace", addr.String(), "dev", iface).CombinedOutput(); err != nil { //nolint:gosec // addr/iface come from this node's own mesh CIDR and TUN device, not external input
		return fmt.Errorf("network: ip address replace %s dev %s: %w: %s", addr, iface, err, out)
	}
	if out, err := exec.CommandContext(ctx, "ip", "link", "set", iface, "up").CombinedOutput(); err != nil { //nolint:gosec // iface comes from this node's own TUN device, not external input
		return fmt.Errorf("network: ip link set %s up: %w: %s", iface, err, out)
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
