package standalone

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

// Errors a caller maps to distinct API responses.
var (
	ErrBlocked              = errors.New("standalone: target is not allowed")
	ErrLoop                 = errors.New("standalone: target points back at this server's own ingress")
	ErrConfirmationRequired = errors.New("standalone: target needs explicit confirmation")
)

// ControlPort is one of this platform's own listeners on this server.
// AlwaysBlock ports (the ingress admin API) can never be a target; the rest
// need explicit confirmation.
type ControlPort struct {
	Port        int    `json:"port"`
	Label       string `json:"label"`
	AlwaysBlock bool   `json:"always_block,omitempty"`
}

// Resolver is the DNS surface Assess needs; *net.Resolver satisfies it.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Env is what Assess knows about this server.
type Env struct {
	Resolver     Resolver
	ControlPorts []ControlPort
	// OwnAddresses are this server's public addresses, for loop detection.
	OwnAddresses []string
	// IngressPorts are the ports the ingress itself listens on (80, 443 or
	// their overrides); a target on this server at one of them loops.
	IngressPorts []int
}

// Assessment says what saving a target means.
type Assessment struct {
	Dial        string       `json:"dial,omitempty"`
	Addresses   []string     `json:"addresses,omitempty"`
	Internal    bool         `json:"internal"`
	Loopback    bool         `json:"loopback"`
	ControlPort *ControlPort `json:"control_port,omitempty"`
	// NeedsConfirmation lists why the caller must confirm, empty when not.
	NeedsConfirmation []string `json:"needs_confirmation,omitempty"`
}

var metadataPrefixes = []netip.Prefix{
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fd00:ec2::/32"),
	netip.MustParsePrefix("100.100.100.200/32"),
}

// CheckAddr refuses addresses no standalone domain may ever reach: cloud
// metadata, link-local, unspecified and multicast. Loopback and private are
// allowed on purpose.
func CheckAddr(a netip.Addr) error {
	a = a.Unmap()
	if !a.IsValid() || a.IsUnspecified() || a.IsMulticast() {
		return fmt.Errorf("%w: %s is not a routable address", ErrBlocked, a)
	}
	for _, p := range metadataPrefixes {
		if p.Contains(a) {
			return fmt.Errorf("%w: %s is a cloud metadata or link-local address", ErrBlocked, a)
		}
	}
	return nil
}

func isInternal(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsLoopback() || a.IsPrivate() || netip.MustParsePrefix("100.64.0.0/10").Contains(a) || netip.MustParsePrefix("fd00::/8").Contains(a)
}

const resolveTimeout = 3 * time.Second

// Assess resolves t's address (for port, address and URL kinds) and returns
// what saving it means. App and container targets resolve at reconcile time.
func Assess(ctx context.Context, t Target, env Env) (Assessment, error) {
	var a Assessment
	switch t.Kind {
	case KindApp:
		return a, nil
	case KindContainer:
		a.Internal = true
		a.NeedsConfirmation = append(a.NeedsConfirmation, fmt.Sprintf("this server will connect to container %s on port %d over the container network", t.Container, t.Port))
		return a, nil
	}
	// Target is exported and may not come from ParseTarget; a wrapped port would assess one address and dial another.
	if t.Port < 1 || t.Port > 65535 {
		return a, fmt.Errorf("%w: port %d is out of range (1 to 65535)", ErrInvalidTarget, t.Port)
	}
	addrs, err := resolve(ctx, t.Host, env.Resolver)
	if err != nil {
		return a, err
	}
	for _, addr := range addrs {
		if err := CheckAddr(addr); err != nil {
			return a, err
		}
		a.Addresses = append(a.Addresses, addr.String())
		if isInternal(addr) {
			a.Internal = true
		}
		if addr.Unmap().IsLoopback() {
			a.Loopback = true
		}
	}
	a.Dial = joinDial(addrs[0], t.Port)
	if loop := loopReason(t, addrs, env); loop != "" {
		return a, fmt.Errorf("%w: %s", ErrLoop, loop)
	}
	if a.Loopback {
		for i := range env.ControlPorts {
			cp := env.ControlPorts[i]
			if cp.Port != t.Port {
				continue
			}
			if cp.AlwaysBlock {
				return a, fmt.Errorf("%w: port %d is the %s and can never be exposed", ErrBlocked, cp.Port, cp.Label)
			}
			a.ControlPort = &cp
			a.NeedsConfirmation = append(a.NeedsConfirmation, fmt.Sprintf("port %d is this server's own %s; exposing it publishes that listener on this domain", cp.Port, cp.Label))
		}
	}
	if a.Internal {
		a.NeedsConfirmation = append(a.NeedsConfirmation, fmt.Sprintf("this server will connect to %s, a %s address", strings.Join(a.Addresses, ", "), internalLabel(a.Loopback)))
	}
	return a, nil
}

func internalLabel(loopback bool) string {
	if loopback {
		return "loopback (this server)"
	}
	return "private network"
}

func resolve(ctx context.Context, host string, r Resolver) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{ip.Unmap()}, nil
	}
	if r == nil {
		return nil, fmt.Errorf("standalone: resolve %s: no resolver", host)
	}
	rctx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()
	addrs, err := r.LookupNetIP(rctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("standalone: resolve %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("standalone: resolve %s: no addresses", host)
	}
	out := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.Unmap())
	}
	return out, nil
}

func loopReason(t Target, addrs []netip.Addr, env Env) string {
	own := map[netip.Addr]bool{}
	for _, s := range env.OwnAddresses {
		if a, err := netip.ParseAddr(s); err == nil {
			own[a.Unmap()] = true
		}
	}
	ingress := map[int]bool{80: true, 443: true}
	for _, p := range env.IngressPorts {
		ingress[p] = true
	}
	if !ingress[t.Port] {
		return ""
	}
	for _, a := range addrs {
		if own[a] {
			return fmt.Sprintf("%s:%d is this server's own public address and ingress port", a, t.Port)
		}
		if a.IsLoopback() || a.IsUnspecified() {
			return fmt.Sprintf("port %d on this server is the ingress itself", t.Port)
		}
	}
	return ""
}

func joinDial(a netip.Addr, port int) string {
	return netip.AddrPortFrom(a, uint16(port)).String() //nolint:gosec // port validated to 1..65535 by ParseTarget
}
