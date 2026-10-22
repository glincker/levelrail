package ingress

import (
	"context"
	"fmt"

	"github.com/caddyserver/caddy/v2"
	"github.com/libdns/libdns"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
)

// GoogleCloudDNSProvider is the wire shape for the in-repo Caddy module
// dns.providers.googleclouddns, used as a DNSChallengeConfig's Provider.
// ServiceAccountJSON needs the dns.admin or DNS Administrator role on the project.
type GoogleCloudDNSProvider struct {
	Name               string `json:"name"`
	ServiceAccountJSON string `json:"service_account_json"`
}

func (p GoogleCloudDNSProvider) dnsProviderModule() any { return p }

// GoogleCloudDNSModule is the Caddy DNS provider module backed by
// dnsrecords.ZoneManager, so Cloud DNS needs no third-party libdns module.
type GoogleCloudDNSModule struct {
	ServiceAccountJSON string `json:"service_account_json,omitempty"`

	mgr dnsrecords.Manager
}

func init() {
	caddy.RegisterModule(&GoogleCloudDNSModule{})
}

// CaddyModule implements caddy.Module.
func (*GoogleCloudDNSModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "dns.providers.googleclouddns",
		New: func() caddy.Module { return new(GoogleCloudDNSModule) },
	}
}

// Provision implements caddy.Provisioner.
func (m *GoogleCloudDNSModule) Provision(caddy.Context) error {
	mgr, err := dnsrecords.NewGoogleCloudManager(m.ServiceAccountJSON)
	if err != nil {
		return fmt.Errorf("ingress: provision google cloud dns: %w", err)
	}
	m.mgr = mgr
	return nil
}

// AppendRecords implements libdns.RecordAppender.
func (m *GoogleCloudDNSModule) AppendRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	return m.mgr.AppendRecords(ctx, zone, recs)
}

// DeleteRecords implements libdns.RecordDeleter.
func (m *GoogleCloudDNSModule) DeleteRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	return m.mgr.DeleteRecords(ctx, zone, recs)
}

var (
	_ caddy.Provisioner     = (*GoogleCloudDNSModule)(nil)
	_ libdns.RecordAppender = (*GoogleCloudDNSModule)(nil)
	_ libdns.RecordDeleter  = (*GoogleCloudDNSModule)(nil)
)
