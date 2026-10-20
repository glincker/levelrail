package network

import (
	"errors"
	"net/netip"
	"testing"
)

func addrs(ss ...string) []netip.Addr {
	out := make([]netip.Addr, 0, len(ss))
	for _, s := range ss {
		out = append(out, netip.MustParseAddr(s))
	}
	return out
}

func TestAllocateAddress(t *testing.T) {
	tests := []struct {
		name  string
		cidr  netip.Prefix
		taken []netip.Addr
		want  string
	}{
		{
			name: "first allocation skips the network address",
			cidr: netip.MustParsePrefix("10.181.0.0/16"),
			want: "10.181.0.1",
		},
		{
			name:  "next free after a run",
			cidr:  netip.MustParsePrefix("10.181.0.0/16"),
			taken: addrs("10.181.0.1", "10.181.0.2", "10.181.0.3"),
			want:  "10.181.0.4",
		},
		{
			name:  "reuses a hole left by a removed node",
			cidr:  netip.MustParsePrefix("10.181.0.0/16"),
			taken: addrs("10.181.0.1", "10.181.0.3"),
			want:  "10.181.0.2",
		},
		{
			name:  "crosses an octet boundary",
			cidr:  netip.MustParsePrefix("10.181.0.0/16"),
			taken: allRange("10.181.0.", 1, 255),
			want:  "10.181.1.0",
		},
		{
			name: "small cidr",
			cidr: netip.MustParsePrefix("192.168.42.0/30"),
			want: "192.168.42.1",
		},
		{
			name:  "broadcast address is usable on a point to point mesh",
			cidr:  netip.MustParsePrefix("192.168.42.0/30"),
			taken: addrs("192.168.42.1", "192.168.42.2"),
			want:  "192.168.42.3",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AllocateAddress(tc.cidr, tc.taken)
			if err != nil {
				t.Fatalf("AllocateAddress: %v", err)
			}
			if got.String() != tc.want {
				t.Errorf("AllocateAddress = %s, want %s", got, tc.want)
			}
			if !tc.cidr.Contains(got) {
				t.Errorf("allocated %s outside %s", got, tc.cidr)
			}
		})
	}
}

func allRange(prefix string, from, to int) []netip.Addr {
	out := make([]netip.Addr, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, netip.MustParseAddr(prefix+itoa(i)))
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [4]byte
	n := 0
	for i > 0 {
		b[n] = byte('0' + i%10)
		i /= 10
		n++
	}
	out := make([]byte, n)
	for j := 0; j < n; j++ {
		out[j] = b[n-1-j]
	}
	return string(out)
}

func TestAllocateAddress_Exhausted(t *testing.T) {
	cidr := netip.MustParsePrefix("192.168.42.0/30")
	taken := addrs("192.168.42.1", "192.168.42.2", "192.168.42.3")

	_, err := AllocateAddress(cidr, taken)
	if !errors.Is(err, ErrNoAddressAvailable) {
		t.Fatalf("AllocateAddress error = %v, want it to wrap %v", err, ErrNoAddressAvailable)
	}
}

func TestAllocateAddress_InvalidCIDR(t *testing.T) {
	tests := []struct {
		name string
		cidr netip.Prefix
	}{
		{name: "unset", cidr: netip.Prefix{}},
		{name: "ipv6", cidr: netip.MustParsePrefix("fd00::/64")},
		{name: "host bits set", cidr: netip.MustParsePrefix("10.181.0.5/16")},
		{name: "too small", cidr: netip.MustParsePrefix("10.181.0.0/32")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := AllocateAddress(tc.cidr, nil); !errors.Is(err, ErrInvalidMeshCIDR) {
				t.Fatalf("error = %v, want it to wrap %v", err, ErrInvalidMeshCIDR)
			}
		})
	}
}

// TestAllocateAddresses_NeverRenumbers is the property that makes an
// address safe to bake into another node's AllowedIPs and into live
// connection state: an existing assignment is never revoked.
func TestAllocateAddresses_NeverRenumbers(t *testing.T) {
	nodes := []NodeInfo{
		{ID: "a", Address: netip.MustParseAddr("10.181.0.7")},
		{ID: "b"},
		{ID: "c", Address: netip.MustParseAddr("10.181.0.1")},
		{ID: "d"},
	}

	got, err := AllocateAddresses(nodes, testCIDR())
	if err != nil {
		t.Fatalf("AllocateAddresses: %v", err)
	}

	if got[0].Address.String() != "10.181.0.7" {
		t.Errorf("node a was renumbered to %s", got[0].Address)
	}
	if got[2].Address.String() != "10.181.0.1" {
		t.Errorf("node c was renumbered to %s", got[2].Address)
	}
	// New nodes fill the lowest free addresses, skipping what is taken.
	if got[1].Address.String() != "10.181.0.2" {
		t.Errorf("node b got %s, want 10.181.0.2", got[1].Address)
	}
	if got[3].Address.String() != "10.181.0.3" {
		t.Errorf("node d got %s, want 10.181.0.3", got[3].Address)
	}

	// The input slice must be untouched, so a caller that fails to
	// persist has not already half-applied the change.
	if nodes[1].Address.IsValid() {
		t.Error("AllocateAddresses mutated its input")
	}
}

func TestAllocateAddresses_Idempotent(t *testing.T) {
	nodes := []NodeInfo{{ID: "a"}, {ID: "b"}, {ID: "c"}}

	first, err := AllocateAddresses(nodes, testCIDR())
	if err != nil {
		t.Fatalf("AllocateAddresses: %v", err)
	}
	second, err := AllocateAddresses(first, testCIDR())
	if err != nil {
		t.Fatalf("AllocateAddresses (second pass): %v", err)
	}

	for i := range first {
		if first[i].Address != second[i].Address {
			t.Errorf("node %q moved from %s to %s on a second pass",
				first[i].ID, first[i].Address, second[i].Address)
		}
	}
}

func TestAllocateAddresses_Exhausted(t *testing.T) {
	// A /30 holds three usable addresses under this allocator; asking for
	// four must fail rather than silently duplicating one.
	nodes := []NodeInfo{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}
	_, err := AllocateAddresses(nodes, netip.MustParsePrefix("192.168.42.0/30"))
	if !errors.Is(err, ErrNoAddressAvailable) {
		t.Fatalf("error = %v, want it to wrap %v", err, ErrNoAddressAvailable)
	}
}

func TestMeshAddresses(t *testing.T) {
	nodes := []NodeInfo{
		{ID: "c", Address: netip.MustParseAddr("10.181.0.9")},
		{ID: "b"},
		{ID: "a", Address: netip.MustParseAddr("10.181.0.2")},
	}
	got := MeshAddresses(nodes)
	if len(got) != 2 {
		t.Fatalf("got %d addresses, want 2 (the unassigned node is skipped)", len(got))
	}
	if got[0].String() != "10.181.0.2" || got[1].String() != "10.181.0.9" {
		t.Errorf("MeshAddresses = %v, want sorted [10.181.0.2 10.181.0.9]", got)
	}
}

func TestDefaultMeshCIDR_IsUsable(t *testing.T) {
	if err := validateCIDR(DefaultMeshCIDR); err != nil {
		t.Fatalf("DefaultMeshCIDR is not a valid mesh CIDR: %v", err)
	}
	if !DefaultMeshCIDR.Addr().IsPrivate() {
		t.Errorf("DefaultMeshCIDR %s is not in RFC 1918 space", DefaultMeshCIDR)
	}
}
