// Package dnszones manages whole DNS zones at Cloudflare and Route53: zones,
// their assigned name servers, record sets with routing policies, delegation
// checks, propagation checks, zone file import and export, and templates.
package dnszones

import (
	"context"
	"errors"
)

// Provider names, matching the DNS-01 provider names used elsewhere.
const (
	ProviderCloudflare = "cloudflare"
	ProviderRoute53    = "route53"
)

// Routing policies a record set can use. Only Route53 supports anything but simple.
const (
	RoutingSimple     = "simple"
	RoutingWeighted   = "weighted"
	RoutingFailover   = "failover"
	RoutingMultivalue = "multivalue"
)

// Failover roles for RoutingFailover record sets.
const (
	FailoverPrimary   = "PRIMARY"
	FailoverSecondary = "SECONDARY"
)

// Apex is the relative name of a zone's own apex.
const Apex = "@"

var (
	// ErrZoneNotFound means no zone the credentials can see matches the reference.
	ErrZoneNotFound = errors.New("dnszones: zone not found")
	// ErrUnsupported means the provider has no equivalent for the requested feature.
	ErrUnsupported = errors.New("dnszones: not supported by this provider")
)

// Zone is one hosted zone. RecordCount is nil when the provider cannot report it cheaply.
type Zone struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Provider    string   `json:"provider"`
	Status      string   `json:"status,omitempty"`
	RecordCount *int     `json:"record_count,omitempty"`
	NameServers []string `json:"name_servers"`
	Private     bool     `json:"private,omitempty"`
	ModifiedAt  string   `json:"modified_at,omitempty"`
}

// RecordSet is every value of one name, type and set identifier. Values use
// zone file presentation syntax, except TXT which holds the raw text.
type RecordSet struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	TTL           int      `json:"ttl"`
	Values        []string `json:"values"`
	Proxied       bool     `json:"proxied,omitempty"`
	Routing       string   `json:"routing,omitempty"`
	SetIdentifier string   `json:"set_identifier,omitempty"`
	Weight        *int64   `json:"weight,omitempty"`
	Failover      string   `json:"failover,omitempty"`
	HealthCheckID string   `json:"health_check_id,omitempty"`
	Alias         *Alias   `json:"alias,omitempty"`
}

// Alias is a Route53 alias target. Alias sets are shown but edited only through the provider.
type Alias struct {
	DNSName              string `json:"dns_name"`
	HostedZoneID         string `json:"hosted_zone_id"`
	EvaluateTargetHealth bool   `json:"evaluate_target_health"`
}

// Key identifies a record set within a zone.
type Key struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	SetIdentifier string `json:"set_identifier,omitempty"`
}

// Key returns rs's identity.
func (rs RecordSet) Key() Key {
	return Key{Name: rs.Name, Type: rs.Type, SetIdentifier: rs.SetIdentifier}
}

// HealthCheck is a Route53 health check usable by failover and multivalue sets.
type HealthCheck struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	IPAddress        string `json:"ip_address,omitempty"`
	FQDN             string `json:"fqdn,omitempty"`
	Port             int32  `json:"port,omitempty"`
	ResourcePath     string `json:"resource_path,omitempty"`
	RequestInterval  int32  `json:"request_interval,omitempty"`
	FailureThreshold int32  `json:"failure_threshold,omitempty"`
}

// Capabilities says which optional features a provider supports, so callers
// can validate before sending anything upstream.
type Capabilities struct {
	Proxied      bool `json:"proxied"`
	Routing      bool `json:"routing"`
	HealthChecks bool `json:"health_checks"`
	ApexCNAME    bool `json:"apex_cname"`
}

// Provider is the zone-level surface both Cloudflare and Route53 implement.
type Provider interface {
	Name() string
	Capabilities() Capabilities
	ListZones(ctx context.Context) ([]Zone, error)
	GetZone(ctx context.Context, id string) (Zone, error)
	CreateZone(ctx context.Context, name string) (Zone, error)
	DeleteZone(ctx context.Context, id string) error
	ListRecordSets(ctx context.Context, zone Zone) ([]RecordSet, error)
	UpsertRecordSet(ctx context.Context, zone Zone, rs RecordSet) error
	DeleteRecordSet(ctx context.Context, zone Zone, key Key) error
}

// HealthChecker is implemented by providers with health checks (Route53).
type HealthChecker interface {
	ListHealthChecks(ctx context.Context) ([]HealthCheck, error)
	CreateHealthCheck(ctx context.Context, hc HealthCheck) (HealthCheck, error)
	DeleteHealthCheck(ctx context.Context, id string) error
}

// FindZone resolves ref (a zone id or a zone name) among zones.
func FindZone(zones []Zone, ref string) (Zone, error) {
	name := NormalizeDomain(ref)
	for _, z := range zones {
		if z.ID == ref || NormalizeDomain(z.Name) == name {
			return z, nil
		}
	}
	return Zone{}, ErrZoneNotFound
}

// ValidateHealthCheck checks the fields CreateHealthCheck needs.
func ValidateHealthCheck(hc HealthCheck) error {
	switch hc.Type {
	case "HTTP", "HTTPS", "TCP":
	default:
		return errors.New("type must be HTTP, HTTPS or TCP")
	}
	if hc.IPAddress == "" && hc.FQDN == "" {
		return errors.New("ip_address or fqdn is required")
	}
	if hc.Port < 0 || hc.Port > 65535 || (hc.Type == "TCP" && hc.Port == 0) {
		return errors.New("port must be between 1 and 65535")
	}
	return nil
}

// IsManagedApexType reports whether a set is provider managed (apex NS and SOA).
func IsManagedApexType(rs RecordSet) bool {
	return rs.Type == "SOA" || (rs.Type == "NS" && rs.Name == Apex)
}
