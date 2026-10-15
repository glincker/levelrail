package maintenance

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Bounds on a domain's maintenance settings.
const (
	MaxDomainTitle      = 120
	MaxDomainMessage    = 2000
	MaxBypassEntries    = 32
	MaxRetryAfterSecond = 86400
)

// DomainInput is a request to put one domain into maintenance.
type DomainInput struct {
	Title             string
	Message           string
	RetryAfterSeconds int
	// Until, when set, ends maintenance automatically through a linked window.
	Until             *time.Time
	Bypass            []string
	BypassHealthCheck bool
}

// Validate checks in and returns the settings to store, without a window id.
func (in DomainInput) Validate(domain string, now time.Time) (store.DomainMaintenance, error) {
	title, msg := strings.TrimSpace(in.Title), strings.TrimSpace(in.Message)
	switch {
	case utf8.RuneCountInString(title) > MaxDomainTitle:
		return store.DomainMaintenance{}, fmt.Errorf("title must be at most %d characters", MaxDomainTitle)
	case utf8.RuneCountInString(msg) > MaxDomainMessage:
		return store.DomainMaintenance{}, fmt.Errorf("message must be at most %d characters", MaxDomainMessage)
	case in.RetryAfterSeconds < 0 || in.RetryAfterSeconds > MaxRetryAfterSecond:
		return store.DomainMaintenance{}, fmt.Errorf("retry_after_seconds must be between 0 and %d", MaxRetryAfterSecond)
	}
	if in.Until != nil {
		switch {
		case !in.Until.After(now):
			return store.DomainMaintenance{}, errors.New("until is already in the past")
		case in.Until.Sub(now) > MaxDuration:
			return store.DomainMaintenance{}, fmt.Errorf("until may be at most %s ahead", MaxDuration)
		}
	}
	cidrs, err := NormalizeBypass(in.Bypass)
	if err != nil {
		return store.DomainMaintenance{}, err
	}
	return store.DomainMaintenance{
		Domain: domain, Title: title, Message: msg, RetryAfterSeconds: in.RetryAfterSeconds,
		BypassCIDRs: cidrs, BypassHealthCheck: in.BypassHealthCheck,
	}, nil
}

// NormalizeBypass turns IP addresses and CIDRs into deduplicated, masked
// CIDRs. A catch-all range is refused: that is ending maintenance.
func NormalizeBypass(entries []string) ([]string, error) {
	if len(entries) > MaxBypassEntries {
		return nil, fmt.Errorf("at most %d bypass entries are allowed", MaxBypassEntries)
	}
	seen := map[string]bool{}
	var out []string
	for _, raw := range entries {
		e := strings.TrimSpace(raw)
		if e == "" {
			continue
		}
		var p netip.Prefix
		if strings.Contains(e, "/") {
			parsed, err := netip.ParsePrefix(e)
			if err != nil {
				return nil, fmt.Errorf("bypass entry %q is not an IP address or CIDR", e)
			}
			p = parsed.Masked()
		} else {
			addr, err := netip.ParseAddr(e)
			if err != nil {
				return nil, fmt.Errorf("bypass entry %q is not an IP address or CIDR", e)
			}
			p = netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen())
		}
		if p.Bits() == 0 {
			return nil, fmt.Errorf("bypass entry %q covers every address; end maintenance instead", e)
		}
		if s := p.String(); !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out, nil
}

// DomainWindow is the scheduled window that carries a domain's end time, so
// the banner, status page and announcer see it without a second state.
func DomainWindow(m store.DomainMaintenance, app string, now, until time.Time) store.MaintenanceWindow {
	title := m.Title
	if title == "" {
		title = "Maintenance on " + m.Domain
	}
	return store.MaintenanceWindow{
		Title: title, Message: m.Message, Scope: store.MaintenanceScopeApp, Target: app,
		StartsAt: now.UTC(), EndsAt: until.UTC(),
	}
}
