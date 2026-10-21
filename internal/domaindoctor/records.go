package domaindoctor

import (
	"fmt"
	"net"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// IsApex reports whether domain is a registrable domain (example.com,
// example.co.uk), where a CNAME is not allowed.
func IsApex(domain string) bool {
	apex, err := publicsuffix.EffectiveTLDPlusOne(strings.TrimPrefix(domain, "*."))
	return err == nil && apex == domain
}

// PlanRecords returns the one record domain needs to reach this server: an
// address record at an apex, a CNAME to serverHost for a subdomain when the
// server has a public hostname, an address record otherwise. It returns nil
// when nothing is known about this server's address.
func PlanRecords(domain string, serverIPs []string, serverHost string) []Record {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(serverHost), "."))
	if host != "" && host != domain && net.ParseIP(host) == nil && !IsApex(domain) {
		return []Record{{Name: domain, Type: "CNAME", Value: host}}
	}
	var v6 string
	for _, s := range serverIPs {
		ip := net.ParseIP(strings.TrimSpace(s))
		switch {
		case ip == nil:
		case ip.To4() != nil:
			return []Record{{Name: domain, Type: "A", Value: ip.String()}}
		case v6 == "":
			v6 = ip.String()
		}
	}
	if v6 != "" {
		return []Record{{Name: domain, Type: "AAAA", Value: v6}}
	}
	return nil
}

func (r Record) copyValue() string {
	return fmt.Sprintf("%s %s %s", r.Name, r.Type, r.Value)
}

// relativeName is the record name inside zone, "@" for the apex.
func relativeName(domain, zone string) string {
	zone = strings.ToLower(strings.TrimSuffix(zone, "."))
	if zone == "" || domain == zone {
		return "@"
	}
	return strings.TrimSuffix(domain, "."+zone)
}

// recordFix offers rec: created by the connected DNS provider when one is
// connected, else the exact values to enter at the DNS provider.
func (rn *run) recordFix(summary string, rec Record) *Fix {
	f := &Fix{Summary: summary, Records: []Record{rec}}
	if rn.facts.DNSProvider != "" && rn.facts.App != "" {
		f.Summary += " Your connected " + providerLabel(rn.facts.DNSProvider) + " account can create it."
		f.Action = &Action{
			Kind: ActionRequest, Label: "Create the record",
			API:  "POST " + rn.appDomainAPI("/dns-records"),
			Body: map[string]string{"name": relativeName(rec.Name, rn.facts.DNSZone), "type": rec.Type, "value": rec.Value},
		}
		return f
	}
	f.Action = &Action{Kind: ActionCopy, Label: "Copy record", Value: rec.copyValue()}
	return f
}

func providerLabel(p string) string {
	switch p {
	case "cloudflare":
		return "Cloudflare"
	case "route53":
		return "Route 53"
	}
	return p
}
