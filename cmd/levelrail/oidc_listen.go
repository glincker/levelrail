package main

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"time"
)

const (
	envOIDCListenRetry     = "APP_OIDC_TOKEN_REQUEST_RETRY"
	defaultOIDCListenRetry = 5 * time.Second
)

// hostAddrAssigned reports whether ip is one of the host's interface
// addresses. Podman's default bridge gateway is only assigned once a
// container is running, so binding earlier fails with "cannot assign".
func hostAddrAssigned(ip string, local []netip.Prefix) bool {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	for _, p := range local {
		if p.Addr() == a {
			return true
		}
	}
	return false
}

func localInterfacePrefixes() []netip.Prefix {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []netip.Prefix
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			if ip, ok := netip.AddrFromSlice(ipn.IP); ok {
				ones, _ := ipn.Mask.Size()
				out = append(out, netip.PrefixFrom(ip.Unmap(), ones))
			}
		}
	}
	return out
}

// listenWhenAssigned waits for the gateway address to exist on the host,
// then listens. It returns nil when ctx ends first.
func listenWhenAssigned(ctx context.Context, addr string, retry time.Duration, local func() []netip.Prefix, logger *slog.Logger) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	warned := false
	for !hostAddrAssigned(host, local()) {
		if !warned {
			logger.Warn("pipeline: oidc token request address not on the host yet, waiting for the container bridge", slog.String("addr", addr))
			warned = true
		}
		select {
		case <-ctx.Done():
			return nil, nil
		case <-time.After(retry):
		}
	}
	var lc net.ListenConfig
	return lc.Listen(ctx, "tcp", addr)
}
