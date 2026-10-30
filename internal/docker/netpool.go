package docker

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/filters"
	dockernetwork "github.com/docker/docker/api/types/network"
)

const (
	// EnvNetworkPool overrides the block per-app networks are carved from.
	EnvNetworkPool = "APP_NETWORK_POOL"
	// DefaultNetworkPool is 1024 /24 networks, in a private range Docker's
	// own defaults (172.17/16 to 172.31/16, 192.168/16) never touch.
	DefaultNetworkPool = "10.88.0.0/14"
	// EnvNetworkPoolPodman overrides the default pool on Podman only.
	EnvNetworkPoolPodman = "APP_NETWORK_POOL_PODMAN"
	// DefaultPodmanNetworkPool sits clear of netavark's 10.88 to 10.9x range.
	DefaultPodmanNetworkPool = "10.200.0.0/14"

	networkPoolSubnetBits = 24
	maxAllocAttempts      = 8

	daemonPoolsExhaustedMsg = "all predefined address pools have been fully subnetted"
	daemonPoolOverlapMsg    = "Pool overlaps with other one on this address space"
)

// NetworkPool is the address block explicit per-network subnets come from.
type NetworkPool struct {
	Base netip.Prefix
}

// ParseNetworkPool parses an APP_NETWORK_POOL value; empty means the default.
// "off" returns ok=false so the daemon's own default pools are used.
func ParseNetworkPool(s string) (pool NetworkPool, ok bool, err error) {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "off") {
		return NetworkPool{}, false, nil
	}
	if s == "" {
		s = DefaultNetworkPool
	}
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return NetworkPool{}, false, fmt.Errorf("docker: %s %q is not a CIDR block: %w", EnvNetworkPool, s, err)
	}
	if !p.Addr().Is4() {
		return NetworkPool{}, false, fmt.Errorf("docker: %s %q must be IPv4", EnvNetworkPool, s)
	}
	if p.Bits() > networkPoolSubnetBits {
		return NetworkPool{}, false, fmt.Errorf("docker: %s %q is smaller than one /%d network", EnvNetworkPool, s, networkPoolSubnetBits)
	}
	if p.Masked() != p {
		return NetworkPool{}, false, fmt.Errorf("docker: %s %q has host bits set, did you mean %s", EnvNetworkPool, s, p.Masked())
	}
	return NetworkPool{Base: p}, true, nil
}

// Capacity is how many /24 networks fit in the pool.
func (p NetworkPool) Capacity() int {
	return 1 << (networkPoolSubnetBits - p.Base.Bits())
}

func (p NetworkPool) candidate(i int) netip.Prefix {
	a := p.Base.Addr().As4()
	v := uint32(a[0])<<24 | uint32(a[1])<<16 | uint32(a[2])<<8 | uint32(a[3])
	v += uint32(i) << (32 - networkPoolSubnetBits)                                                                                //nolint:gosec // i is bounded by the pool capacity (at most 2^(24-bits) candidates)
	return netip.PrefixFrom(netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}), networkPoolSubnetBits) //nolint:gosec // byte conversion of masked octets
}

// podmanDefaultSubnet is netavark's default network. It is skipped even when
// the engine does not list it, so the default pool never collides with Podman.
var podmanDefaultSubnet = netip.MustParsePrefix("10.88.0.0/16")

// Allocate returns the first /24 in the pool overlapping nothing in used.
func (p NetworkPool) Allocate(used []netip.Prefix) (netip.Prefix, bool) {
	for i := 0; i < p.Capacity(); i++ {
		c := p.candidate(i)
		if !c.Overlaps(podmanDefaultSubnet) && !overlapsAny(c, used) {
			return c, true
		}
	}
	return netip.Prefix{}, false
}

// Used counts how many of used fall inside the pool.
func (p NetworkPool) Used(used []netip.Prefix) int {
	n := 0
	for _, u := range used {
		if p.Base.Overlaps(u) {
			n++
		}
	}
	return n
}

func overlapsAny(c netip.Prefix, used []netip.Prefix) bool {
	for _, u := range used {
		if c.Overlaps(u) {
			return true
		}
	}
	return false
}

// PoolExhaustedError is returned when no subnet could be allocated for a
// new network. Its message carries the numbers and the fix.
type PoolExhaustedError struct {
	Pool     string
	InUse    int
	Capacity int
	Daemon   bool
}

func (e *PoolExhaustedError) Error() string {
	if e.Daemon {
		return fmt.Sprintf("no address space left for a new app network: the Docker daemon's default address pools are fully subnetted (%d networks in use). "+
			"Fix: remove unused networks (network prune from the system networks page or CLI), set %s to a free private block (default %s) so networks get explicit subnets, "+
			"or widen \"default-address-pools\" in the Docker daemon.json", e.InUse, EnvNetworkPool, DefaultNetworkPool)
	}
	return fmt.Sprintf("no address space left for a new app network: pool %s is full (%d of %d networks in use). "+
		"Fix: remove unused networks (network prune from the system networks page or CLI), or widen %s to a larger private block (for example a /12)",
		e.Pool, e.InUse, e.Capacity, EnvNetworkPool)
}

// IsPoolExhausted reports whether err is a daemon address-pool exhaustion.
func IsPoolExhausted(err error) bool {
	return err != nil && strings.Contains(err.Error(), daemonPoolsExhaustedMsg)
}

// networkAPI is the slice of the Engine client the allocator needs.
type networkAPI interface {
	NetworkList(ctx context.Context, options dockernetwork.ListOptions) ([]dockernetwork.Summary, error)
	NetworkCreate(ctx context.Context, name string, options dockernetwork.CreateOptions) (dockernetwork.CreateResponse, error)
}

// hostPrefixes returns host-visible prefixes (interface addresses and
// routes) a new network must not overlap. A var so tests can pin it.
var hostPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				if p, ok := netip.AddrFromSlice(ipn.IP); ok && p.Unmap().Is4() {
					ones, _ := ipn.Mask.Size()
					out = append(out, netip.PrefixFrom(p.Unmap(), ones).Masked())
				}
			}
		}
	}
	return append(out, procRoutePrefixes("/proc/net/route")...)
}

func procRoutePrefixes(path string) []netip.Prefix {
	f, err := os.Open(path) //nolint:gosec // fixed kernel path
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var out []netip.Prefix
	sc := bufio.NewScanner(f)
	sc.Scan()
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 8 {
			continue
		}
		dst, err1 := strconv.ParseUint(fields[1], 16, 32)
		mask, err2 := strconv.ParseUint(fields[7], 16, 32)
		if err1 != nil || err2 != nil || mask == 0 {
			continue
		}
		ones := 0
		for m := uint32(mask); m&1 == 1; m >>= 1 { // /proc values are little-endian, low bits set first
			ones++
		}
		a := netip.AddrFrom4([4]byte{byte(dst), byte(dst >> 8), byte(dst >> 16), byte(dst >> 24)}) //nolint:gosec // dst is a parsed 32-bit value, octets are intentional truncation
		out = append(out, netip.PrefixFrom(a, ones).Masked())
	}
	return out
}

func networkSubnets(n dockernetwork.Summary) []netip.Prefix {
	var out []netip.Prefix
	for _, c := range n.IPAM.Config {
		if p, err := netip.ParsePrefix(c.Subnet); err == nil && p.Addr().Is4() {
			out = append(out, p)
		}
	}
	return out
}

// poolAllocator serializes subnet picks within this process; the daemon's
// overlap error is the cross-process backstop.
type poolAllocator struct {
	mu sync.Mutex
}

// create makes name as a bridge network on an explicit free subnet.
func (a *poolAllocator) create(ctx context.Context, api networkAPI, pool NetworkPool, name string, labels map[string]string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var rejected []netip.Prefix
	for attempt := 0; attempt < maxAllocAttempts; attempt++ {
		nets, err := api.NetworkList(ctx, dockernetwork.ListOptions{Filters: filters.NewArgs()})
		if err != nil {
			return "", fmt.Errorf("docker: list networks for subnet allocation: %w", err)
		}
		used := append([]netip.Prefix{}, hostPrefixes()...)
		used = append(used, rejected...)
		for _, n := range nets {
			used = append(used, networkSubnets(n)...)
		}
		subnet, ok := pool.Allocate(used)
		if !ok {
			return "", &PoolExhaustedError{Pool: pool.Base.String(), InUse: poolUsage(pool, nets), Capacity: pool.Capacity()}
		}
		resp, err := api.NetworkCreate(ctx, name, dockernetwork.CreateOptions{
			Driver: "bridge",
			Labels: labels,
			IPAM:   &dockernetwork.IPAM{Config: []dockernetwork.IPAMConfig{{Subnet: subnet.String()}}},
		})
		if err == nil {
			return resp.ID, nil
		}
		if !strings.Contains(err.Error(), daemonPoolOverlapMsg) && !strings.Contains(err.Error(), "invalid pool request") {
			return "", err
		}
		rejected = append(rejected, subnet)
	}
	return "", &PoolExhaustedError{Pool: pool.Base.String(), InUse: len(rejected), Capacity: pool.Capacity()}
}

func poolUsage(pool NetworkPool, nets []dockernetwork.Summary) int {
	n := 0
	for _, s := range nets {
		for _, p := range networkSubnets(s) {
			if pool.Base.Overlaps(p) {
				n++
				break
			}
		}
	}
	return n
}
