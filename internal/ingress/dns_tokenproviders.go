package ingress

import (
	"context"
	"fmt"

	"github.com/caddyserver/caddy/v2"
	"github.com/libdns/libdns"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/dnszones"
)

// TokenDNSProvider is the wire shape for the in-repo Caddy modules
// dns.providers.digitalocean, .hetzner, .azure, .porkbun and .desec, used as a DNSChallengeConfig's
// Provider. Credential is an API token, or the credential JSON for Azure.
type TokenDNSProvider struct {
	Name       string `json:"name"`
	Credential string `json:"credential"`
}

func (p TokenDNSProvider) dnsProviderModule() any { return p }

// tokenDNSModule is shared by the modules, all backed by dnsrecords.ZoneManager
// so these providers need no third-party libdns module.
type tokenDNSModule struct {
	Credential string `json:"credential,omitempty"`

	mgr dnsrecords.Manager
}

func (m *tokenDNSModule) provision(name string) error {
	p, err := dnszones.NewTokenProvider(name, m.Credential)
	if err != nil {
		return fmt.Errorf("ingress: provision %s dns: %w", name, err)
	}
	m.mgr = dnsrecords.ZoneManager{P: p}
	return nil
}

// AppendRecords implements libdns.RecordAppender.
func (m *tokenDNSModule) AppendRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	return m.mgr.AppendRecords(ctx, zone, recs)
}

// DeleteRecords implements libdns.RecordDeleter.
func (m *tokenDNSModule) DeleteRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	return m.mgr.DeleteRecords(ctx, zone, recs)
}

// DigitalOceanDNSModule is dns.providers.digitalocean.
type DigitalOceanDNSModule struct{ tokenDNSModule }

// HetznerDNSModule is dns.providers.hetzner.
type HetznerDNSModule struct{ tokenDNSModule }

// PorkbunDNSModule is dns.providers.porkbun.
type PorkbunDNSModule struct{ tokenDNSModule }

// DeSECDNSModule is dns.providers.desec.
type DeSECDNSModule struct{ tokenDNSModule }

// AzureDNSModule is dns.providers.azure.
type AzureDNSModule struct{ tokenDNSModule }

func init() {
	caddy.RegisterModule(&DigitalOceanDNSModule{})
	caddy.RegisterModule(&HetznerDNSModule{})
	caddy.RegisterModule(&AzureDNSModule{})
	caddy.RegisterModule(&PorkbunDNSModule{})
	caddy.RegisterModule(&DeSECDNSModule{})
}

// CaddyModule implements caddy.Module.
func (*PorkbunDNSModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{ID: "dns.providers.porkbun", New: func() caddy.Module { return new(PorkbunDNSModule) }}
}

// CaddyModule implements caddy.Module.
func (*DeSECDNSModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{ID: "dns.providers.desec", New: func() caddy.Module { return new(DeSECDNSModule) }}
}

// Provision implements caddy.Provisioner.
func (m *PorkbunDNSModule) Provision(caddy.Context) error {
	return m.provision(dnszones.ProviderPorkbun)
}

// Provision implements caddy.Provisioner.
func (m *DeSECDNSModule) Provision(caddy.Context) error { return m.provision(dnszones.ProviderDeSEC) }

// CaddyModule implements caddy.Module.
func (*DigitalOceanDNSModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{ID: "dns.providers.digitalocean", New: func() caddy.Module { return new(DigitalOceanDNSModule) }}
}

// CaddyModule implements caddy.Module.
func (*HetznerDNSModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{ID: "dns.providers.hetzner", New: func() caddy.Module { return new(HetznerDNSModule) }}
}

// CaddyModule implements caddy.Module.
func (*AzureDNSModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{ID: "dns.providers.azure", New: func() caddy.Module { return new(AzureDNSModule) }}
}

// Provision implements caddy.Provisioner.
func (m *DigitalOceanDNSModule) Provision(caddy.Context) error {
	return m.provision(dnszones.ProviderDigitalOcean)
}

// Provision implements caddy.Provisioner.
func (m *HetznerDNSModule) Provision(caddy.Context) error {
	return m.provision(dnszones.ProviderHetzner)
}

// Provision implements caddy.Provisioner.
func (m *AzureDNSModule) Provision(caddy.Context) error { return m.provision(dnszones.ProviderAzure) }

var (
	_ caddy.Provisioner     = (*DigitalOceanDNSModule)(nil)
	_ caddy.Provisioner     = (*HetznerDNSModule)(nil)
	_ caddy.Provisioner     = (*AzureDNSModule)(nil)
	_ caddy.Provisioner     = (*PorkbunDNSModule)(nil)
	_ caddy.Provisioner     = (*DeSECDNSModule)(nil)
	_ libdns.RecordAppender = (*DigitalOceanDNSModule)(nil)
	_ libdns.RecordDeleter  = (*AzureDNSModule)(nil)
)
