// Package standalone models domains that are not owned by any app and proxy
// to a target the operator picks: a local port, an address, a URL, another
// app's port or a container.
package standalone

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// Scope is the reserved {name} path segment the per-domain endpoints accept
// for standalone domains, so every per-domain feature works unchanged.
const Scope = "_standalone"

// Target kinds.
const (
	KindPort      = "port"
	KindAddress   = "address"
	KindURL       = "url"
	KindApp       = "app"
	KindContainer = "container"
)

// Host header modes.
const (
	HostPreserve = "preserve"
	HostUpstream = "upstream"
)

const (
	appPrefix       = "app:"
	containerPrefix = "container:"
	maxTargetLength = 512
)

// ErrInvalidTarget wraps every target parse failure.
var ErrInvalidTarget = errors.New("standalone: invalid target")

// Target is a parsed, canonical proxy target.
type Target struct {
	Kind string
	// Host is the address host for KindAddress, "127.0.0.1" for KindPort.
	Host string
	// Port is the dialed port, or the container port for app and container
	// kinds (0 for an app means its main port).
	Port      int
	URL       *url.URL
	App       string
	Container string
}

// ParseTarget accepts "3000", ":3000", "127.0.0.1:3000", "localhost:3000",
// "host:port", "[v6]:port", "http(s)://host[:port][/base]", "app:NAME[:PORT]"
// and "container:NAME:PORT".
func ParseTarget(raw string) (Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Target{}, fmt.Errorf("%w: target is required", ErrInvalidTarget)
	}
	if len(raw) > maxTargetLength || strings.ContainsAny(raw, "{} \t\r\n\\\"'`") {
		return Target{}, fmt.Errorf("%w: target must not contain spaces, quotes, braces or backslashes", ErrInvalidTarget)
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return parseURL(raw)
	case strings.Contains(lower, "://"):
		return Target{}, fmt.Errorf("%w: only http and https URLs are supported", ErrInvalidTarget)
	case strings.HasPrefix(lower, appPrefix):
		return parseApp(raw[len(appPrefix):])
	case strings.HasPrefix(lower, containerPrefix):
		return parseContainer(raw[len(containerPrefix):])
	}
	if bare := strings.TrimPrefix(raw, ":"); isDigits(bare) {
		p, err := parsePort(bare)
		if err != nil {
			return Target{}, err
		}
		return Target{Kind: KindPort, Host: "127.0.0.1", Port: p}, nil
	}
	host, portStr, err := net.SplitHostPort(raw)
	if err != nil {
		return Target{}, fmt.Errorf("%w: use a port, host:port, an http(s) URL, app:NAME[:PORT] or container:NAME:PORT", ErrInvalidTarget)
	}
	port, err := parsePort(portStr)
	if err != nil {
		return Target{}, err
	}
	host = strings.ToLower(host)
	if host == "localhost" {
		return Target{Kind: KindPort, Host: "127.0.0.1", Port: port}, nil
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Zone() != "" {
			return Target{}, fmt.Errorf("%w: zoned IPv6 addresses are not supported", ErrInvalidTarget)
		}
		if ip.Unmap() == netip.MustParseAddr("127.0.0.1") {
			return Target{Kind: KindPort, Host: "127.0.0.1", Port: port}, nil
		}
		return Target{Kind: KindAddress, Host: ip.Unmap().String(), Port: port}, nil
	}
	if !ValidHostname(host) {
		return Target{}, fmt.Errorf("%w: %q is not a valid hostname or IP address", ErrInvalidTarget, host)
	}
	return Target{Kind: KindAddress, Host: host, Port: port}, nil
}

func parsePort(s string) (int, error) {
	p, err := strconv.Atoi(s)
	if err != nil || s == "" || strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("%w: port must be a number", ErrInvalidTarget)
	}
	if p < 1 || p > 65535 {
		return 0, fmt.Errorf("%w: port %d is out of range (1 to 65535)", ErrInvalidTarget, p)
	}
	return p, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func parseURL(raw string) (Target, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return Target{}, fmt.Errorf("%w: invalid URL", ErrInvalidTarget)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.User != nil {
		return Target{}, fmt.Errorf("%w: URL must not include credentials; use the domain's basic auth or headers instead", ErrInvalidTarget)
	}
	if u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return Target{}, fmt.Errorf("%w: URL must not include a query or a fragment", ErrInvalidTarget)
	}
	if _, _, err := net.SplitHostPort(u.Host); err == nil {
		if _, err := parsePort(u.Port()); err != nil {
			return Target{}, err
		}
	}
	host := strings.ToLower(u.Hostname())
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Zone() != "" {
			return Target{}, fmt.Errorf("%w: zoned IPv6 addresses are not supported", ErrInvalidTarget)
		}
	} else if !ValidHostname(host) {
		return Target{}, fmt.Errorf("%w: %q is not a valid hostname", ErrInvalidTarget, host)
	}
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.RawPath = ""
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 80
		if u.Scheme == "https" {
			port = 443
		}
	}
	return Target{Kind: KindURL, Host: host, Port: port, URL: u}, nil
}

func parseApp(rest string) (Target, error) {
	name, portStr, hasPort := strings.Cut(rest, ":")
	if !validName(name) {
		return Target{}, fmt.Errorf("%w: app:NAME needs a valid app name", ErrInvalidTarget)
	}
	t := Target{Kind: KindApp, App: name}
	if hasPort {
		p, err := parsePort(portStr)
		if err != nil {
			return Target{}, err
		}
		t.Port = p
	}
	return t, nil
}

func parseContainer(rest string) (Target, error) {
	name, portStr, ok := strings.Cut(rest, ":")
	if !ok || !validName(name) {
		return Target{}, fmt.Errorf("%w: use container:NAME:PORT", ErrInvalidTarget)
	}
	p, err := parsePort(portStr)
	if err != nil {
		return Target{}, err
	}
	return Target{Kind: KindContainer, Container: name, Port: p}, nil
}

func validName(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for i, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || (i > 0 && (r == '-' || r == '_' || r == '.'))
		if !ok {
			return false
		}
	}
	return true
}

// ValidHostname reports whether h is a syntactically valid DNS name.
func ValidHostname(h string) bool {
	h = strings.TrimSuffix(h, ".")
	if h == "" || len(h) > 253 {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !isLabelRune(r) {
				return false
			}
		}
	}
	return true
}

func isLabelRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
}

// String is the canonical form stored and shown, round-trippable by ParseTarget.
func (t Target) String() string {
	switch t.Kind {
	case KindPort:
		return net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
	case KindAddress:
		return net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
	case KindURL:
		return t.URL.String()
	case KindApp:
		if t.Port > 0 {
			return appPrefix + t.App + ":" + strconv.Itoa(t.Port)
		}
		return appPrefix + t.App
	case KindContainer:
		return containerPrefix + t.Container + ":" + strconv.Itoa(t.Port)
	}
	return ""
}

// Scheme is the scheme the ingress speaks to the target.
func (t Target) Scheme(upstreamTLS bool) string {
	if t.Kind == KindURL {
		return t.URL.Scheme
	}
	if upstreamTLS {
		return "https"
	}
	return "http"
}

// TLS reports whether the upstream connection uses TLS.
func (t Target) TLS(upstreamTLS bool) bool {
	return t.Scheme(upstreamTLS) == "https"
}

// Display is the human form of where traffic goes, e.g. "http://127.0.0.1:3000".
func (t Target) Display(upstreamTLS bool) string {
	switch t.Kind {
	case KindURL:
		return t.URL.String()
	case KindApp:
		if t.Port > 0 {
			return fmt.Sprintf("%s://app %s port %d", t.Scheme(upstreamTLS), t.App, t.Port)
		}
		return fmt.Sprintf("%s://app %s", t.Scheme(upstreamTLS), t.App)
	case KindContainer:
		return fmt.Sprintf("%s://%s:%d", t.Scheme(upstreamTLS), t.Container, t.Port)
	}
	return t.Scheme(upstreamTLS) + "://" + net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
}

// BasePath is the URL target's path prefix, "" for every other kind.
func (t Target) BasePath() string {
	if t.Kind != KindURL {
		return ""
	}
	return t.URL.EscapedPath()
}

// DefaultHostMode is upstream for a URL, where the remote expects its own
// name, and preserve for everything on this server or network.
func (t Target) DefaultHostMode() string {
	if t.Kind == KindURL {
		return HostUpstream
	}
	return HostPreserve
}
