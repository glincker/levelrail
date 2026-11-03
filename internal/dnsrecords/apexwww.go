package dnsrecords

import (
	"fmt"
	"strings"
	"time"

	"github.com/libdns/libdns"
)

// WWW modes: which of apex and www get records.
const (
	WWWBoth     = "both"
	WWWApexOnly = "apex_only"
	WWWOnly     = "www_only"
)

// Canonical values: which host visitors end up on.
const (
	CanonicalApex = "apex"
	CanonicalWWW  = "www"
)

// ApexWWWItem is one record of an apex and www plan.
type ApexWWWItem struct {
	Desired Desired
	Plan    Plan
}

// ValidWWWMode reports whether m is a known domains.www_mode value.
func ValidWWWMode(m string) bool { return m == WWWBoth || m == WWWApexOnly || m == WWWOnly }

// ValidCanonical reports whether c is a known domains.canonical value.
func ValidCanonical(c string) bool { return c == CanonicalApex || c == CanonicalWWW }

// PlanApexWWW lists the records a zone-apex domain needs for mode. The apex
// gets the address record; www follows it with a CNAME so one change moves
// both, except in www_only mode where www carries the address itself.
func PlanApexWWW(apex, mode string, target Desired, existing []libdns.Record, replace bool, ttl time.Duration) ([]ApexWWWItem, error) {
	if !ValidWWWMode(mode) {
		return nil, fmt.Errorf("dnsrecords: unknown www mode %q", mode)
	}
	if target.Type == "" || target.Value == "" {
		return nil, ErrNoTarget
	}
	apex = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(apex)), ".")
	var want []Desired
	switch mode {
	case WWWBoth:
		want = []Desired{
			{Name: "@", Type: target.Type, Value: target.Value, TTL: ttl},
			{Name: "www", Type: "CNAME", Value: apex + ".", TTL: ttl},
		}
	case WWWApexOnly:
		want = []Desired{{Name: "@", Type: target.Type, Value: target.Value, TTL: ttl}}
	case WWWOnly:
		want = []Desired{{Name: "www", Type: target.Type, Value: target.Value, TTL: ttl}}
	}
	out := make([]ApexWWWItem, 0, len(want))
	for _, d := range want {
		out = append(out, ApexWWWItem{Desired: d, Plan: PlanRecord(d, existing, replace)})
	}
	return out, nil
}

// CanonicalRedirect names the 308 the plan implies: from the non-canonical
// host to the canonical one. Both are empty unless mode serves both hosts.
func CanonicalRedirect(apex, mode, canonical string) (from, to string) {
	if mode != WWWBoth || !ValidCanonical(canonical) {
		return "", ""
	}
	apex = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(apex)), ".")
	if canonical == CanonicalWWW {
		return apex, "www." + apex
	}
	return "www." + apex, apex
}
