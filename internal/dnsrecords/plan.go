package dnsrecords

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/libdns/libdns"
)

// Outcome values a record plan or apply reports per domain.
const (
	OutcomeCreated   = "created"
	OutcomeUpdated   = "updated"
	OutcomeUnchanged = "unchanged"
	OutcomeConflict  = "conflict"
)

// ErrZoneNotFound means no zone the provider manages contains the domain.
var ErrZoneNotFound = errors.New("dnsrecords: no managed zone contains the domain")

// ErrNoTarget means neither a public address nor a CNAME target is known.
var ErrNoTarget = errors.New("dnsrecords: no public address or CNAME target is known for this server")

// Desired is the one record a domain should have.
type Desired struct {
	Name  string
	Type  string
	Value string
	TTL   time.Duration
}

// Plan is what applying Desired would do against a zone's existing records.
type Plan struct {
	Outcome string
	// Replace holds the existing records an update removes first; for a
	// conflict it lists the records that block the create.
	Replace []libdns.RR
}

// Candidates returns every zone name domain could belong to, longest first,
// down to two labels (a bare TLD is never a zone).
func Candidates(domain string) []string {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	labels := strings.Split(domain, ".")
	var out []string
	for i := 0; i+2 <= len(labels); i++ {
		out = append(out, strings.Join(labels[i:], "."))
	}
	return out
}

// MatchZone returns the longest zone in zones that contains domain, with a
// trailing dot, or false when none does.
func MatchZone(domain string, zones []string) (string, bool) {
	best := ""
	for _, c := range Candidates(domain) {
		for _, z := range zones {
			if strings.EqualFold(strings.TrimSuffix(z, "."), c) && len(c) > len(best) {
				best = c
			}
		}
	}
	if best == "" {
		return "", false
	}
	return best + ".", true
}

// RelativeName is domain's record name inside zone: "@" for the apex.
func RelativeName(domain, zone string) string {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	zone = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(zone)), ".")
	if domain == zone {
		return "@"
	}
	return strings.TrimSuffix(domain, "."+zone)
}

// TargetFor picks the record type and value pointing at a server reachable
// at ips: an A record for the first IPv4 address, else AAAA for the first
// IPv6 one. ips that are not IP literals are ignored.
func TargetFor(ips []string) (recType, value string, err error) {
	var v6 string
	for _, s := range ips {
		ip := net.ParseIP(strings.TrimSpace(s))
		switch {
		case ip == nil:
		case ip.To4() != nil:
			return "A", ip.String(), nil
		case v6 == "":
			v6 = ip.String()
		}
	}
	if v6 != "" {
		return "AAAA", v6, nil
	}
	return "", "", ErrNoTarget
}

// ValidateTarget rejects a CNAME at a zone apex for providers that cannot
// flatten it.
func ValidateTarget(d Desired, provider string) error {
	if d.Type == "CNAME" && d.Name == "@" && provider != "cloudflare" {
		return fmt.Errorf("a CNAME cannot be created at the zone apex with %s; use an address record instead", provider)
	}
	return nil
}

func sameData(a, b, recType string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if recType == "CNAME" {
		a, b = strings.TrimSuffix(a, "."), strings.TrimSuffix(b, ".")
	}
	return strings.EqualFold(a, b)
}

func conflicts(desiredType, existingType string) bool {
	switch desiredType {
	case "CNAME":
		return existingType == "A" || existingType == "AAAA" || existingType == "CNAME"
	case "A":
		return existingType == "A" || existingType == "CNAME"
	case "AAAA":
		return existingType == "AAAA" || existingType == "CNAME"
	}
	return false
}

// PlanRecord decides how to bring d into existing. An identical record is
// unchanged even next to siblings; a different A/AAAA/CNAME at the same name
// is a conflict unless replace is set, and a foreign TXT or MX is never
// touched.
func PlanRecord(d Desired, existing []libdns.Record, replace bool) Plan {
	var blocking []libdns.RR
	for _, rec := range existing {
		rr := rec.RR()
		if !strings.EqualFold(rr.Name, d.Name) {
			continue
		}
		if rr.Type == d.Type && sameData(rr.Data, d.Value, d.Type) {
			return Plan{Outcome: OutcomeUnchanged}
		}
		if conflicts(d.Type, rr.Type) {
			blocking = append(blocking, rr)
		}
	}
	switch {
	case len(blocking) == 0:
		return Plan{Outcome: OutcomeCreated}
	case replace:
		return Plan{Outcome: OutcomeUpdated, Replace: blocking}
	default:
		return Plan{Outcome: OutcomeConflict, Replace: blocking}
	}
}
