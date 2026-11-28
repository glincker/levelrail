// Package dnsrecords builds a provider-agnostic DNS record manager from
// whichever ACME DNS-01 provider a control plane already has credentials
// for (Cloudflare or Route53), reusing libdns's own interfaces instead of
// inventing a second one: github.com/libdns/cloudflare and
// github.com/libdns/route53, both already a dependency via
// internal/ingress's Caddy DNS-01 wiring, implement them already.
package dnsrecords

import (
	"context"

	"github.com/libdns/cloudflare"
	"github.com/libdns/libdns"
	"github.com/libdns/route53"

	"github.com/glincker/levelrail/internal/dnszones"
)

// Manager is the subset of libdns's interfaces a DNS records view needs:
// list, non-destructive add, replace-by-exact-match, and delete. Any
// provider implementing it already satisfies libdns.RecordGetter,
// libdns.RecordAppender, libdns.RecordSetter, and libdns.RecordDeleter.
type Manager interface {
	GetRecords(ctx context.Context, zone string) ([]libdns.Record, error)
	AppendRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error)
	SetRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error)
	DeleteRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error)
}

// NewCloudflareManager wraps github.com/libdns/cloudflare with the same
// scoped API token (Zone:DNS:Edit) already stored for Cloudflare ACME
// DNS-01, so records management needs no separate credential.
func NewCloudflareManager(apiToken string) Manager {
	return &cloudflare.Provider{APIToken: apiToken}
}

// NewRoute53Manager wraps github.com/libdns/route53 with the same IAM
// access key pair already stored for Route53 ACME DNS-01. Region and
// hostedZoneID may be empty: the provider resolves them itself the same
// way it does for DNS-01 (see internal/reconcile/ingress's own use).
func NewRoute53Manager(accessKeyID, secretAccessKey, region, hostedZoneID string) Manager {
	return &route53.Provider{
		AccessKeyId:     accessKeyID, //nolint:staticcheck // libdns/route53's own field name, not ours to rename
		SecretAccessKey: secretAccessKey,
		Region:          region,
		HostedZoneID:    hostedZoneID,
	}
}

// NewRoute53ZoneManager serves record automation through the Route 53 API.
// The libdns module lists the whole zone before every read and write and
// fails on one record it cannot parse, so only ACME DNS-01 keeps using it.
func NewRoute53ZoneManager(accessKeyID, secretAccessKey, region, hostedZoneID string) Manager {
	return ZoneManager{P: dnszones.NewRoute53(accessKeyID, secretAccessKey, region, hostedZoneID)}
}
