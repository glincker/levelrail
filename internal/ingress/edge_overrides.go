package ingress

import (
	"fmt"
	"strings"
	"time"
)

// EdgeOverrides are dashboard-managed edge values. Each applies only when its
// APP_INGRESS_* variable is unset, so an env value always wins.
type EdgeOverrides struct {
	TrustedProxies        string
	MaxHeaderBytes        *int64
	MaxBodyBytes          *int64
	ReadHeaderTimeoutSecs *int64
	IdleTimeoutSecs       *int64
}

// ValidateTrustedProxies checks a comma or space separated list of IPs, CIDRs
// and the private_ranges shorthand.
func ValidateTrustedProxies(raw string) error {
	_, err := parseTrustedProxies(raw)
	return err
}

// EnvSet reports whether the named variable has a non-blank value.
func EnvSet(getenv func(string) string, name string) bool {
	return strings.TrimSpace(getenv(name)) != ""
}

// WithOverrides layers dashboard values over h for every setting whose env
// variable is unset.
func (h Hardening) WithOverrides(o EdgeOverrides, getenv func(string) string) (Hardening, error) {
	if !EnvSet(getenv, EnvTrustedProxies) && strings.TrimSpace(o.TrustedProxies) != "" {
		ranges, err := parseTrustedProxies(o.TrustedProxies)
		if err != nil {
			return h, fmt.Errorf("ingress: trusted proxies setting: %w", err)
		}
		h.TrustedProxies = ranges
	}
	if o.MaxHeaderBytes != nil && !EnvSet(getenv, EnvMaxHeaderBytes) {
		h.MaxHeaderBytes = int(*o.MaxHeaderBytes)
	}
	if o.MaxBodyBytes != nil && !EnvSet(getenv, EnvMaxBodyBytes) {
		h.MaxBodyBytes = *o.MaxBodyBytes
	}
	if o.ReadHeaderTimeoutSecs != nil && !EnvSet(getenv, EnvReadHeaderTimeout) {
		h.ReadHeaderTimeout = time.Duration(*o.ReadHeaderTimeoutSecs) * time.Second
	}
	if o.IdleTimeoutSecs != nil && !EnvSet(getenv, EnvIdleTimeout) {
		h.IdleTimeout = time.Duration(*o.IdleTimeoutSecs) * time.Second
	}
	return h, nil
}
