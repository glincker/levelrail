package dnszones

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

// RecordTypes are the supported types, in the order the dashboard offers them.
var RecordTypes = []string{"A", "AAAA", "CNAME", "TXT", "MX", "CAA", "SRV", "NS"}

// TTL bounds. TTLAuto (1) is Cloudflare's "automatic".
const (
	TTLAuto = 1
	TTLMin  = 30
	TTLMax  = 604800
)

// Issue severities.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// Issue codes, stable for the dashboard and CLI.
const (
	IssueCNAMEExclusive = "cname_exclusive"
	IssueApexCNAME      = "apex_cname"
	IssueApexFlattened  = "apex_cname_flattened"
	IssueMixedRouting   = "mixed_routing"
	IssueExists         = "exists"
)

// Issue is one validation or conflict finding.
type Issue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// ValidationError carries every problem found in one record set.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string { return strings.Join(e.Problems, "; ") }

func supportedType(t string) bool { return slices.Contains(RecordTypes, t) }

// Normalize validates rs and returns its canonical form: relative lower case
// name, upper case type, deduplicated canonical values and a defaulted TTL.
func Normalize(rs RecordSet, zone string, defaultTTL int) (RecordSet, error) {
	var probs []string
	out := rs
	out.Name = RelativeName(rs.Name, zone)
	out.Type = strings.ToUpper(strings.TrimSpace(rs.Type))
	if out.TTL <= 0 {
		out.TTL = defaultTTL
	}
	if out.Routing == "" {
		out.Routing = RoutingSimple
	}
	switch {
	case !supportedType(out.Type):
		probs = append(probs, fmt.Sprintf("type must be one of %s", strings.Join(RecordTypes, ", ")))
	case out.Type == "NS" && out.Name == Apex:
		probs = append(probs, "apex NS records are managed by the provider; add NS only for a delegated subdomain")
	}
	if err := validateName(out.Name); err != nil {
		probs = append(probs, err.Error())
	}
	if out.TTL != TTLAuto && (out.TTL < TTLMin || out.TTL > TTLMax) {
		probs = append(probs, fmt.Sprintf("ttl must be %d (auto) or between %d and %d seconds", TTLAuto, TTLMin, TTLMax))
	}
	if out.Proxied && out.Type != "A" && out.Type != "AAAA" && out.Type != "CNAME" {
		probs = append(probs, "only A, AAAA and CNAME records can be proxied")
	}
	if out.Alias == nil {
		vals, vprobs := normalizeValues(out, zone)
		out.Values = vals
		probs = append(probs, vprobs...)
	}
	probs = append(probs, routingProblems(out)...)
	if len(probs) > 0 {
		return out, &ValidationError{Problems: probs}
	}
	return out, nil
}

func validateName(rel string) error {
	if rel == Apex {
		return nil
	}
	labels := strings.Split(rel, ".")
	for i, l := range labels {
		switch {
		case l == "":
			return fmt.Errorf("name %q has an empty label", rel)
		case len(l) > 63:
			return fmt.Errorf("name %q has a label longer than 63 characters", rel)
		case l == "*" && i != 0:
			return fmt.Errorf("name %q: a wildcard must be the leftmost label", rel)
		case strings.Contains(l, "*") && l != "*":
			return fmt.Errorf("name %q: a wildcard label must be exactly \"*\"", rel)
		}
	}
	return nil
}

func normalizeValues(rs RecordSet, zone string) ([]string, []string) {
	var probs []string
	out := make([]string, 0, len(rs.Values))
	for _, raw := range rs.Values {
		v := strings.TrimSpace(raw)
		if rs.Type != "TXT" && v == "" {
			continue
		}
		canon, err := canonicalValue(rs.Type, v, zone)
		if err != nil {
			probs = append(probs, fmt.Sprintf("%s value %q: %v", rs.Type, v, err))
			continue
		}
		if !slices.Contains(out, canon) {
			out = append(out, canon)
		}
	}
	switch {
	case len(out) == 0 && len(probs) == 0:
		probs = append(probs, "at least one value is required")
	case rs.Type == "CNAME" && len(out) > 1:
		probs = append(probs, "a CNAME record set holds exactly one value")
	}
	return out, probs
}

// canonicalValue parses v as the RDATA of typ through miekg/dns and renders it back.
func canonicalValue(typ, v, zone string) (string, error) {
	rdata := v
	switch typ {
	case "TXT":
		if len(v) > 4000 {
			return "", fmt.Errorf("text longer than 4000 characters")
		}
		rdata = quoteTXT(v)
	case "CNAME", "NS":
		rdata = absTarget(v, zone)
	case "MX":
		rdata = absLastField(v, 1, zone)
	case "SRV":
		rdata = absLastField(v, 3, zone)
	}
	rr, err := dns.NewRR(fmt.Sprintf("x.%s. 300 IN %s %s", NormalizeDomain(zone), typ, rdata))
	if err != nil || rr == nil {
		return "", fmt.Errorf("not valid %s data", typ)
	}
	if c, ok := rr.(*dns.CAA); ok && c.Tag != "issue" && c.Tag != "issuewild" && c.Tag != "iodef" {
		return "", fmt.Errorf("CAA tag must be issue, issuewild or iodef")
	}
	if s, ok := FormatRR(rr); ok {
		return s, nil
	}
	return "", fmt.Errorf("unsupported type %s", typ)
}

// FormatRR renders an RR's data in this package's canonical value form.
func FormatRR(rr dns.RR) (string, bool) {
	switch r := rr.(type) {
	case *dns.A:
		return r.A.String(), true
	case *dns.AAAA:
		return r.AAAA.String(), true
	case *dns.CNAME:
		return NormalizeDomain(r.Target), true
	case *dns.NS:
		return NormalizeDomain(r.Ns), true
	case *dns.TXT:
		var b strings.Builder
		for _, part := range r.Txt {
			b.WriteString(unquoteTXT(`"` + part + `"`))
		}
		return b.String(), true
	case *dns.MX:
		return fmt.Sprintf("%d %s", r.Preference, NormalizeDomain(r.Mx)), true
	case *dns.SRV:
		return fmt.Sprintf("%d %d %d %s", r.Priority, r.Weight, r.Port, NormalizeDomain(r.Target)), true
	case *dns.CAA:
		return fmt.Sprintf("%d %s %s", r.Flag, r.Tag, strconv.Quote(r.Value)), true
	}
	return "", false
}

// absTarget makes a target absolute so miekg does not append the origin.
func absTarget(t, zone string) string {
	t = strings.TrimSpace(t)
	switch {
	case t == Apex:
		return NormalizeDomain(zone) + "."
	case strings.HasSuffix(t, "."):
		return t
	}
	return t + "."
}

func absLastField(v string, idx int, zone string) string {
	f := strings.Fields(v)
	if len(f) == idx+1 {
		f[idx] = absTarget(f[idx], zone)
	}
	return strings.Join(f, " ")
}

func routingProblems(rs RecordSet) []string {
	var probs []string
	switch rs.Routing {
	case RoutingSimple:
		if rs.SetIdentifier != "" || rs.Weight != nil || rs.Failover != "" {
			probs = append(probs, "set_identifier, weight and failover apply only to weighted, failover or multivalue routing")
		}
		return probs
	case RoutingWeighted:
		if rs.Weight == nil || *rs.Weight < 0 || *rs.Weight > 255 {
			probs = append(probs, "weighted routing needs a weight between 0 and 255")
		}
	case RoutingFailover:
		if rs.Failover != FailoverPrimary && rs.Failover != FailoverSecondary {
			probs = append(probs, "failover routing needs failover PRIMARY or SECONDARY")
		}
	case RoutingMultivalue:
		if rs.Type == "CNAME" || rs.Type == "NS" {
			probs = append(probs, "multivalue routing does not apply to CNAME or NS")
		}
	default:
		return append(probs, "routing must be simple, weighted, failover or multivalue")
	}
	if strings.TrimSpace(rs.SetIdentifier) == "" {
		probs = append(probs, "set_identifier is required for "+rs.Routing+" routing")
	}
	if rs.Proxied {
		probs = append(probs, "proxied records cannot use a routing policy")
	}
	return probs
}

// CheckCapabilities rejects features the provider does not support.
func CheckCapabilities(rs RecordSet, caps Capabilities) error {
	switch {
	case rs.Proxied && !caps.Proxied:
		return fmt.Errorf("%w: proxied records need Cloudflare", ErrUnsupported)
	case rs.TTL == TTLAuto && !caps.Proxied:
		return fmt.Errorf("%w: ttl 1 (auto) is Cloudflare only", ErrUnsupported)
	case rs.Routing != "" && rs.Routing != RoutingSimple && !caps.Routing:
		return fmt.Errorf("%w: %s routing needs Route53", ErrUnsupported, rs.Routing)
	case rs.HealthCheckID != "" && !caps.HealthChecks:
		return fmt.Errorf("%w: health checks need Route53", ErrUnsupported)
	case rs.Alias != nil:
		return fmt.Errorf("%w: alias records are edited in the provider console", ErrUnsupported)
	}
	return nil
}

// Conflicts reports conflicts between rs and the other sets in the zone.
// ignore is the key being replaced by an update, if any.
func Conflicts(existing []RecordSet, rs RecordSet, caps Capabilities, ignore *Key) []Issue {
	var out []Issue
	if rs.Type == "CNAME" && rs.Name == Apex {
		if caps.ApexCNAME {
			out = append(out, Issue{SeverityWarning, IssueApexFlattened, "a CNAME at the apex is flattened by the provider; other resolvers never see a CNAME there"})
		} else {
			out = append(out, Issue{SeverityError, IssueApexCNAME, "a CNAME cannot sit at the zone apex; use an A/AAAA record or a Route53 alias"})
		}
	}
	for _, e := range existing {
		if e.Name != rs.Name || (ignore != nil && e.Key() == *ignore) {
			continue
		}
		if e.Key() == rs.Key() {
			out = append(out, Issue{SeverityError, IssueExists, fmt.Sprintf("a %s record set for %s already exists; edit it instead", rs.Type, rs.Name)})
			continue
		}
		if (e.Type == "CNAME") != (rs.Type == "CNAME") && !IsManagedApexType(e) {
			out = append(out, Issue{SeverityError, IssueCNAMEExclusive, fmt.Sprintf("%s already has a %s record; a CNAME cannot share a name with any other record", rs.Name, e.Type)})
			continue
		}
		if e.Type == rs.Type && (e.SetIdentifier == "") != (rs.SetIdentifier == "") {
			out = append(out, Issue{SeverityError, IssueMixedRouting, fmt.Sprintf("%s %s mixes simple and routed record sets", rs.Name, rs.Type)})
		}
	}
	return out
}

// HasErrors reports whether issues includes an error.
func HasErrors(issues []Issue) bool {
	return slices.ContainsFunc(issues, func(i Issue) bool { return i.Severity == SeverityError })
}
