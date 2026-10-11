package dataexport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

// ErrBlockedDestination wraps a refusal to reach an internal address.
var ErrBlockedDestination = errors.New("dataexport: destination resolves to a blocked internal address")

func allowPrivate() bool {
	for _, name := range []string{AllowPrivateEnv, netguard.AllowPrivateEnv} {
		if v, err := strconv.ParseBool(os.Getenv(name)); err == nil && v {
			return true
		}
	}
	return false
}

// newHTTPClient returns an HTTP client that refuses internal addresses at
// dial time (including redirects) unless an allow env var is set.
func newHTTPClient() *http.Client {
	return netguard.NewClientAllowing(AllowPrivateEnv, netguard.AllowPrivateEnv)
}

// ValidateHTTPEndpoint rejects, at configuration time, an endpoint that is
// not an absolute http(s) URL or that points at an internal address.
func ValidateHTTPEndpoint(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return fmt.Errorf("endpoint must be an absolute http or https URL")
	}
	if u.User != nil {
		return fmt.Errorf("endpoint must not embed credentials, use headers instead")
	}
	if allowPrivate() {
		return nil
	}
	if err := netguard.ValidateURL(ctx, raw); err != nil {
		return fmt.Errorf("%w (set %s=true to allow internal destinations)", err, AllowPrivateEnv)
	}
	return nil
}

// ParseSyslogEndpoint splits "tcp://host:port" or "tls://host:port".
func ParseSyslogEndpoint(raw string) (scheme, hostport string, err error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "tcp" && u.Scheme != "tls") || u.Hostname() == "" || u.Port() == "" {
		return "", "", fmt.Errorf("syslog endpoint must be tcp://host:port or tls://host:port")
	}
	return u.Scheme, u.Host, nil
}

// ValidateSyslogEndpoint applies the same internal-address rule as
// ValidateHTTPEndpoint to a syslog endpoint.
func ValidateSyslogEndpoint(ctx context.Context, raw string) error {
	_, hostport, err := ParseSyslogEndpoint(raw)
	if err != nil {
		return err
	}
	host, _, _ := net.SplitHostPort(hostport)
	return ValidateHTTPEndpoint(ctx, "https://"+net.JoinHostPort(host, "443"))
}

// guardedDialer returns a dialer whose connections are refused when they
// target an internal address and the allow switches are off.
func guardedDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if allowPrivate() {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("dataexport: parse dial address %q: %w", address, err)
			}
			ip, err := netip.ParseAddr(strings.Trim(host, "[]"))
			if err != nil {
				return fmt.Errorf("dataexport: parse dial address %q: %w", address, err)
			}
			if netguard.IsBlocked(ip) {
				return fmt.Errorf("%w: %s", ErrBlockedDestination, ip)
			}
			return nil
		},
	}
}
