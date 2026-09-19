package store

import (
	"context"
	"fmt"
)

// Route53DNSSecretsKey is the internal/secrets serviceName the Route53
// DNS-01 credential pair is stored under, mirroring
// CloudflareDNSSecretsKey's shape but for an AWS IAM access key pair
// instead of a single API token.
func Route53DNSSecretsKey() string {
	return "route53-dns"
}

// Route53DNSAccessKeyIDEnvKey and Route53DNSSecretAccessKeyEnvKey are
// the fixed envKeys used within that namespace.
const (
	Route53DNSAccessKeyIDEnvKey     = "access_key_id"
	Route53DNSSecretAccessKeyEnvKey = "secret_access_key"
)

// Route53DNSSettings is the single platform-wide row: whether ACME
// DNS-01 via Route53 is enabled for wildcard domains, plus the two
// non-secret fields libdns/route53 accepts. Region and HostedZoneID are
// both optional; empty lets the library resolve them itself. The access
// key pair goes through internal/secrets instead, the same split
// CloudflareDNSSettings uses for its own token.
type Route53DNSSettings struct {
	Enabled      bool
	Region       string
	HostedZoneID string
}

// GetRoute53DNSSettings returns the single route53_dns_settings row.
// Always succeeds: the migration itself inserts the row (id = 1).
func (db *DB) GetRoute53DNSSettings(ctx context.Context) (Route53DNSSettings, error) {
	var (
		enabled            int
		region, hostedZone string
	)
	err := db.QueryRowContext(ctx, `
		SELECT enabled, region, hosted_zone_id FROM route53_dns_settings WHERE id = 1
	`).Scan(&enabled, &region, &hostedZone)
	if err != nil {
		return Route53DNSSettings{}, fmt.Errorf("store: get route53 dns settings: %w", err)
	}
	return Route53DNSSettings{Enabled: enabled != 0, Region: region, HostedZoneID: hostedZone}, nil
}

// UpdateRoute53DNSSettings replaces the single row's fields.
func (db *DB) UpdateRoute53DNSSettings(ctx context.Context, s Route53DNSSettings) error {
	enabled := 0
	if s.Enabled {
		enabled = 1
	}
	_, err := db.ExecContext(ctx, `
		UPDATE route53_dns_settings SET enabled = ?, region = ?, hosted_zone_id = ? WHERE id = 1
	`, enabled, s.Region, s.HostedZoneID)
	if err != nil {
		return fmt.Errorf("store: update route53 dns settings: %w", err)
	}
	return nil
}
