package standalone

import (
	"fmt"
	"strings"
)

// Limits on per-domain timeouts, in seconds.
const (
	MaxDialTimeoutSeconds = 120
	MaxReadTimeoutSeconds = 3600
	maxPrefixLength       = 256
)

// Options shape how the ingress talks to the target.
type Options struct {
	UpstreamTLS   bool
	TLSSkipVerify bool
	TLSServerName string
	// HostHeader is "" (kind default), preserve, upstream, or a literal hostname.
	HostHeader         string
	WebSocket          bool
	DialTimeoutSeconds int
	ReadTimeoutSeconds int
	StripPrefix        string
}

// Validate checks o against t and returns a normalized copy.
func (o Options) Validate(t Target) (Options, error) {
	o.TLSServerName = strings.ToLower(strings.TrimSpace(o.TLSServerName))
	o.HostHeader = strings.TrimSpace(o.HostHeader)
	o.StripPrefix = strings.TrimSpace(o.StripPrefix)
	if t.Kind == KindURL {
		o.UpstreamTLS = t.URL.Scheme == "https"
	}
	if !o.UpstreamTLS && (o.TLSSkipVerify || o.TLSServerName != "") {
		return o, fmt.Errorf("%w: TLS verify and server name only apply with upstream TLS", ErrInvalidTarget)
	}
	if o.TLSServerName != "" && !ValidHostname(o.TLSServerName) {
		return o, fmt.Errorf("%w: TLS server name must be a hostname", ErrInvalidTarget)
	}
	switch strings.ToLower(o.HostHeader) {
	case "", HostPreserve, HostUpstream:
		o.HostHeader = strings.ToLower(o.HostHeader)
	default:
		if !ValidHostname(o.HostHeader) && !validHostPort(o.HostHeader) {
			return o, fmt.Errorf("%w: host header must be preserve, upstream or a hostname", ErrInvalidTarget)
		}
		o.HostHeader = strings.ToLower(o.HostHeader)
	}
	if o.DialTimeoutSeconds < 0 || o.DialTimeoutSeconds > MaxDialTimeoutSeconds {
		return o, fmt.Errorf("%w: connect timeout must be between 0 (default) and %d seconds", ErrInvalidTarget, MaxDialTimeoutSeconds)
	}
	if o.ReadTimeoutSeconds < 0 || o.ReadTimeoutSeconds > MaxReadTimeoutSeconds {
		return o, fmt.Errorf("%w: read timeout must be between 0 (default) and %d seconds", ErrInvalidTarget, MaxReadTimeoutSeconds)
	}
	if o.StripPrefix != "" {
		o.StripPrefix = "/" + strings.Trim(o.StripPrefix, "/")
		if o.StripPrefix == "/" || len(o.StripPrefix) > maxPrefixLength || strings.ContainsAny(o.StripPrefix, "{}?#%*\\ \t") {
			return o, fmt.Errorf("%w: strip prefix must be a plain path such as /app", ErrInvalidTarget)
		}
	}
	return o, nil
}

func validHostPort(s string) bool {
	host, port, ok := strings.Cut(s, ":")
	if !ok {
		return false
	}
	_, err := parsePort(port)
	return err == nil && ValidHostname(host)
}

// EffectiveHostMode resolves "" to the target kind's default.
func (o Options) EffectiveHostMode(t Target) string {
	if o.HostHeader == "" {
		return t.DefaultHostMode()
	}
	return o.HostHeader
}
