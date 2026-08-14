package network

// This file: mesh address allocation.
//
// The rejected alternative is worth writing down because it is the one
// that looks free: derive each node's mesh address by hashing its node
// ID into the mesh CIDR, so no allocation state has to be persisted at
// all. That breaks on collisions, and not rarely: by the birthday bound,
// hashing into a /24's 253 usable hosts gives a better-than-even chance
// of a collision at only 19 nodes, and two nodes sharing a mesh address
// is the ErrDuplicateAddress failure ValidateInventory exists to catch,
// except that with hashing it is unfixable without renaming a node. A
// /16 pushes the number out but does not change the shape of the
// problem.
//
// So allocation is stateful: the control plane assigns an address once,
// persists it, and never reassigns it while the node exists. What is
// kept pure and testable here is the decision itself, a function from
// (CIDR, already-taken set) to the next address. Persistence is the
// caller's, which is why this takes a slice rather than reaching for a
// store.

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

// DefaultMeshCIDR is the private range the mesh allocates from unless an
// operator overrides it.
//
// 10.181.0.0/16 rather than something more memorable: the whole point of
// picking an obscure corner of RFC 1918 space is that it is unlikely to
// collide with whatever the operator's existing LAN, VPN, or cloud VPC
// already uses. 10.0.0.0/24 and 192.168.1.0/24 are the two ranges most
// likely to already be in use on a machine someone is about to add to a
// fleet, and a mesh address that collides with the host's own LAN route
// is a routing failure that looks like a mesh failure.
//
// A /16 holds 65534 hosts, far beyond the 1-to-10 machines this project
// targets. Sized for headroom rather than fit because narrowing a CIDR
// after nodes hold addresses from the wider one is the migration
// ValidateInventory's ErrAddressOutsideMesh exists to make loud, and
// nobody should have to perform it.
var DefaultMeshCIDR = netip.MustParsePrefix("10.181.0.0/16")

// AllocateAddress returns the lowest address in cidr that is not already
// in taken.
//
// Lowest-free rather than next-sequential or random: it is deterministic
// (the same inputs always produce the same output, which is what makes
// this table-testable), and it reuses the address of a removed node,
// which matters because a mesh that only ever allocates upward would
// eventually exhaust even a /16 in a fleet that churns nodes.
//
// The network address itself (cidr's own base) is skipped: it is not a
// usable host address. The broadcast address is not skipped, because a
// WireGuard interface is point-to-point and has no broadcast domain;
// excluding it would be borrowing an Ethernet convention that does not
// apply here.
func AllocateAddress(cidr netip.Prefix, taken []netip.Addr) (netip.Addr, error) {
	if err := validateCIDR(cidr); err != nil {
		return netip.Addr{}, err
	}

	takenSet := make(map[netip.Addr]struct{}, len(taken))
	for _, a := range taken {
		takenSet[a] = struct{}{}
	}

	base := as32(cidr.Masked().Addr())
	// hostCount is 2^(32-bits); validateCIDR already rejected bits > 30,
	// so this cannot overflow and is always at least 4.
	hostCount := uint32(1) << uint(32-cidr.Bits())

	// Start at offset 1, skipping the network address.
	for offset := uint32(1); offset < hostCount; offset++ {
		candidate := fromUint32(base + offset)
		if _, used := takenSet[candidate]; used {
			continue
		}
		return candidate, nil
	}
	return netip.Addr{}, fmt.Errorf("%w: %s holds %d hosts, all allocated", ErrNoAddressAvailable, cidr, hostCount-1)
}

// AllocateAddresses assigns an address to every node in nodes that does
// not already have a valid one, leaving existing assignments untouched,
// and returns the updated inventory. Existing assignments are never
// revoked or renumbered: a node's mesh address appearing in another
// node's AllowedIPs and in live connection state means renumbering it is
// a disconnection, so allocation is strictly additive.
//
// Returns a new slice rather than mutating in place so a caller that
// fails to persist the result has not already half-applied it to the
// inventory it is still holding.
func AllocateAddresses(nodes []NodeInfo, cidr netip.Prefix) ([]NodeInfo, error) {
	if err := validateCIDR(cidr); err != nil {
		return nil, err
	}

	taken := make([]netip.Addr, 0, len(nodes))
	for _, n := range nodes {
		if n.Address.IsValid() {
			taken = append(taken, n.Address)
		}
	}

	out := make([]NodeInfo, len(nodes))
	copy(out, nodes)
	for i := range out {
		if out[i].Address.IsValid() {
			continue
		}
		addr, err := AllocateAddress(cidr, taken)
		if err != nil {
			return nil, fmt.Errorf("allocate mesh address for node %q: %w", out[i].ID, err)
		}
		out[i].Address = addr
		taken = append(taken, addr)
	}
	return out, nil
}

// as32 converts an IPv4 address to its numeric form. Callers have all
// been through validateCIDR, so Is4 is guaranteed.
func as32(a netip.Addr) uint32 {
	b := a.As4()
	return binary.BigEndian.Uint32(b[:])
}

func fromUint32(v uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return netip.AddrFrom4(b)
}
