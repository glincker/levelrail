package network

import (
	"context"
	"fmt"
	"net/netip"

	"golang.org/x/sys/unix"
)

// setAddressLinux configures iface with ioctls, not ip(8): the minimal
// agent image ships no iproute2. SIOCSIFADDR replaces the address, so it
// stays idempotent, and the connected route appears when the link is up.
func setAddressLinux(_ context.Context, iface string, addr netip.Prefix) error {
	if !addr.Addr().Is4() {
		return fmt.Errorf("network: set address %s on %q: only IPv4 mesh addresses are supported", addr, iface)
	}
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("network: open control socket: %w", err)
	}
	defer func() { _ = unix.Close(fd) }()

	ip := addr.Addr().As4()
	maskBytes := netipMask4(addr.Bits())

	ifr, err := unix.NewIfreq(iface)
	if err != nil {
		return fmt.Errorf("network: interface name %q: %w", iface, err)
	}
	if err := ifr.SetInet4Addr(ip[:]); err != nil {
		return fmt.Errorf("network: encode address %s: %w", addr, err)
	}
	if err := unix.IoctlIfreq(fd, unix.SIOCSIFADDR, ifr); err != nil {
		return fmt.Errorf("network: assign %s to interface %q: %w", addr, iface, err)
	}
	if err := ifr.SetInet4Addr(maskBytes[:]); err != nil {
		return fmt.Errorf("network: encode netmask for %s: %w", addr, err)
	}
	if err := unix.IoctlIfreq(fd, unix.SIOCSIFNETMASK, ifr); err != nil {
		return fmt.Errorf("network: set netmask %s on interface %q: %w", addr, iface, err)
	}

	flagsReq, err := unix.NewIfreq(iface)
	if err != nil {
		return fmt.Errorf("network: interface name %q: %w", iface, err)
	}
	if err := unix.IoctlIfreq(fd, unix.SIOCGIFFLAGS, flagsReq); err != nil {
		return fmt.Errorf("network: read flags of interface %q: %w", iface, err)
	}
	flagsReq.SetUint16(flagsReq.Uint16() | unix.IFF_UP | unix.IFF_RUNNING)
	if err := unix.IoctlIfreq(fd, unix.SIOCSIFFLAGS, flagsReq); err != nil {
		return fmt.Errorf("network: bring up interface %q: %w", iface, err)
	}
	return nil
}

func netipMask4(bits int) [4]byte {
	var m uint32
	if bits > 0 {
		m = ^uint32(0) << (32 - bits)
	}
	return [4]byte{byte(m >> 24), byte(m >> 16), byte(m >> 8), byte(m)}
}
