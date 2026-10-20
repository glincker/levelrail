package proxyroutes

import (
	"fmt"
	"net"
	"strings"
)

// UpstreamUnreachableCode is the error code reported when a listener cannot
// be reached from the proxy container.
const UpstreamUnreachableCode = "upstream_unreachable"

// Listener is one of this instance's HTTP listeners the proxy forwards to.
type Listener struct {
	// Label names it in messages, e.g. "ingress" or "dashboard".
	Label string
	Addr  string
	// EnvVar is the variable that moves it, e.g. APP_INGRESS_HTTP_ADDR.
	EnvVar string
}

// UpstreamPath is how the proxy reaches this host.
type UpstreamPath struct {
	// Host is written into the route (a name or an address).
	Host string
	// IP is what Host resolves to inside the proxy container.
	IP          string
	HostNetwork bool
	// Unit is the systemd unit the drop-in fix targets.
	Unit string
}

// UnreachableError says a listener is bound where the proxy cannot reach it,
// with the exact drop-in that fixes it.
type UnreachableError struct {
	Reason string
	Fix    string
}

func (e *UnreachableError) Error() string {
	if e.Fix == "" {
		return UpstreamUnreachableCode + ": " + e.Reason
	}
	return UpstreamUnreachableCode + ": " + e.Reason + ". Fix: " + e.Fix
}

func isLoopbackHost(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func isWildcardHost(h string) bool {
	if h == "" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsUnspecified()
}

// Resolve returns host:port the proxy should forward to for l, or an
// *UnreachableError. It never substitutes a default address.
func (p UpstreamPath) Resolve(l Listener) (string, error) {
	host, port, err := net.SplitHostPort(l.Addr)
	if err != nil || port == "" {
		return "", &UnreachableError{Reason: fmt.Sprintf("the %s listen address %q is not host:port", l.Label, l.Addr)}
	}
	if p.Host == "" {
		return "", &UnreachableError{Reason: "the address the proxy uses to reach this host is unknown; set upstream_host"}
	}
	switch {
	case p.HostNetwork && (isWildcardHost(host) || isLoopbackHost(host)):
		return net.JoinHostPort("127.0.0.1", port), nil
	case p.HostNetwork:
		return net.JoinHostPort(host, port), nil
	case isWildcardHost(host):
		return net.JoinHostPort(p.Host, port), nil
	case p.IP != "" && net.ParseIP(host) != nil && net.ParseIP(host).Equal(net.ParseIP(p.IP)):
		return net.JoinHostPort(p.Host, port), nil
	case p.IP == "" && !isLoopbackHost(host):
		// Saved upstream name without a resolved address: setup validated it.
		return net.JoinHostPort(p.Host, port), nil
	}
	reason := fmt.Sprintf("the %s listens on %s, which the proxy container cannot reach", l.Label, l.Addr)
	if !isLoopbackHost(host) {
		reason = fmt.Sprintf("the %s listens on %s only, but the proxy reaches this host at %s", l.Label, l.Addr, p.IP)
	}
	return "", &UnreachableError{Reason: reason, Fix: p.dropIn(l, port)}
}

func (p UpstreamPath) dropIn(l Listener, port string) string {
	if p.IP == "" || p.Unit == "" {
		return ""
	}
	dir := "/etc/systemd/system/" + p.Unit + ".d"
	return fmt.Sprintf("create %s/proxy-upstream-%s.conf containing \"[Service]\" and \"Environment=%s=%s\", then run systemctl daemon-reload && systemctl restart %s (the address is internal to this host and not reachable from the internet)",
		dir, l.Label, l.EnvVar, net.JoinHostPort(p.IP, port), p.Unit)
}
