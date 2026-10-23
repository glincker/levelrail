package trafficexport

import (
	"fmt"
	"net/netip"
	"sort"
)

// Optional record fields a sink may opt into. Everything else in Event is
// always exported; cookies, auth headers, query strings and bodies never exist.
const (
	FieldReferrerHost = "referrer_host"
	FieldClientIP     = "client_ip"
	FieldUserAgent    = "user_agent"
	FieldVisitorID    = "visitor_id"
)

// optionalFields maps each opt-in field to whether it can identify a visitor.
var optionalFields = map[string]bool{
	FieldReferrerHost: false,
	FieldClientIP:     true,
	FieldUserAgent:    true,
	FieldVisitorID:    true,
}

// OptionalFields lists every field a sink can opt into, sorted.
func OptionalFields() []string {
	out := make([]string, 0, len(optionalFields))
	for f := range optionalFields {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// IsIdentityField reports whether f can single out a visitor.
func IsIdentityField(f string) bool { return optionalFields[f] }

// HasIdentity reports whether any of fields identifies a visitor.
func HasIdentity(fields []string) bool {
	for _, f := range fields {
		if IsIdentityField(f) {
			return true
		}
	}
	return false
}

// ValidateFields rejects an unknown field name.
func ValidateFields(fields []string) error {
	for _, f := range fields {
		if _, ok := optionalFields[f]; !ok {
			return fmt.Errorf("unknown field %q", f)
		}
	}
	return nil
}

// Policy decides which optional fields one sink receives. The zero value
// exports no visitor identity and no referrer.
type Policy struct {
	Fields     []string
	IPTruncate bool
}

func (p Policy) allows(f string) bool {
	for _, x := range p.Fields {
		if x == f {
			return true
		}
	}
	return false
}

// Apply returns a copy of e holding only what the policy allows. It scrubs
// first, so a field added to Event later is private by default.
func (p Policy) Apply(e Event) Event {
	out := e
	out.ClientIP, out.UserAgent, out.VisitorID, out.ReferrerHost, out.UAFamily = "", "", "", "", ""
	if p.allows(FieldReferrerHost) {
		out.ReferrerHost = e.ReferrerHost
	}
	if p.allows(FieldUserAgent) {
		out.UserAgent = e.UserAgent
	}
	if p.allows(FieldVisitorID) {
		out.VisitorID = e.VisitorID
	}
	if p.allows(FieldClientIP) {
		out.ClientIP = e.ClientIP
		if p.IPTruncate {
			out.ClientIP = TruncateIP(e.ClientIP)
		}
	}
	return out
}

// TruncateIP zeroes the last octet of an IPv4 address and the last 80 bits of
// an IPv6 address. An unparsable value yields "" so a raw string never leaks.
func TruncateIP(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	a = a.Unmap().WithZone("")
	bits := 24
	if a.Is6() {
		bits = 48
	}
	p, err := a.Prefix(bits)
	if err != nil {
		return ""
	}
	return p.Masked().Addr().String()
}
