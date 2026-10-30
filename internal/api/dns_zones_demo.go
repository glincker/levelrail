package api

import (
	"context"
	"slices"
	"sync"

	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/miekg/dns"
)

// Demo zones: one delegated to its assigned nameservers, one still at its registrar.
const (
	demoZoneDelegated    = "example.com"
	demoZoneNotDelegated = "example.net"
)

var demoRegistrarNS = []string{"ns1.registrar.example", "ns2.registrar.example"}

// demoZoneProvider is the in-memory Cloudflare stand-in demo mode serves on the DNS zones page.
// It also answers NS lookups and registrar hints, so nothing leaves the process.
type demoZoneProvider struct {
	mu    sync.Mutex
	zones []dnszones.Zone
	sets  map[string][]dnszones.RecordSet
}

func newDemoZoneProvider() *demoZoneProvider {
	count := 4
	return &demoZoneProvider{
		zones: []dnszones.Zone{
			{ID: "demo-zone-1", Name: demoZoneDelegated, Provider: dnszones.ProviderCloudflare, Status: "active", RecordCount: &count, NameServers: []string{"ada.ns.cloudflare.com", "bob.ns.cloudflare.com"}},
			{ID: "demo-zone-2", Name: demoZoneNotDelegated, Provider: dnszones.ProviderCloudflare, Status: "pending", NameServers: []string{"cleo.ns.cloudflare.com", "dan.ns.cloudflare.com"}},
		},
		sets: map[string][]dnszones.RecordSet{
			"demo-zone-1": {
				{Name: dnszones.Apex, Type: "NS", TTL: 86400, Values: []string{"ada.ns.cloudflare.com.", "bob.ns.cloudflare.com."}},
				{Name: dnszones.Apex, Type: "A", TTL: 300, Values: []string{"203.0.113.10"}, Proxied: true},
				{Name: "www", Type: "CNAME", TTL: 300, Values: []string{"example.com."}, Proxied: true},
				{Name: dnszones.Apex, Type: "TXT", TTL: 300, Values: []string{"v=spf1 -all"}},
			},
			"demo-zone-2": {
				{Name: dnszones.Apex, Type: "NS", TTL: 86400, Values: []string{"cleo.ns.cloudflare.com.", "dan.ns.cloudflare.com."}},
				{Name: "app", Type: "A", TTL: 300, Values: []string{"198.51.100.20"}},
			},
		},
	}
}

// demoDNSZones is non-nil only in demo mode (dev mode plus APP_MAIL_DEMO).
func (rt *Router) demoDNSZones() *demoZoneProvider {
	if rt.mail.demo == nil {
		return nil
	}
	rt.dnsNS.demoOnce.Do(func() { rt.dnsNS.demo = newDemoZoneProvider() })
	return rt.dnsNS.demo
}

func (d *demoZoneProvider) Name() string { return dnszones.ProviderCloudflare }

func (d *demoZoneProvider) Capabilities() dnszones.Capabilities {
	return dnszones.Capabilities{Proxied: true, ApexCNAME: true}
}

func (d *demoZoneProvider) ListZones(context.Context) ([]dnszones.Zone, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.zones), nil
}

func (d *demoZoneProvider) GetZone(_ context.Context, id string) (dnszones.Zone, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return dnszones.FindZone(d.zones, id)
}

func (d *demoZoneProvider) CreateZone(context.Context, string) (dnszones.Zone, error) {
	return dnszones.Zone{}, dnszones.ErrUnsupported
}

func (d *demoZoneProvider) DeleteZone(context.Context, string) error { return dnszones.ErrUnsupported }

func (d *demoZoneProvider) ListRecordSets(_ context.Context, z dnszones.Zone) ([]dnszones.RecordSet, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.sets[z.ID]), nil
}

func (d *demoZoneProvider) UpsertRecordSet(_ context.Context, z dnszones.Zone, rs dnszones.RecordSet) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sets[z.ID] = append(slices.DeleteFunc(d.sets[z.ID], func(e dnszones.RecordSet) bool { return e.Key() == rs.Key() }), rs)
	return nil
}

func (d *demoZoneProvider) DeleteRecordSet(_ context.Context, z dnszones.Zone, k dnszones.Key) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sets[z.ID] = slices.DeleteFunc(d.sets[z.ID], func(e dnszones.RecordSet) bool { return e.Key() == k })
	return nil
}

// Query answers NS lookups for the demo zones only.
func (d *demoZoneProvider) Query(_ context.Context, server, name string, qtype uint16) dnszones.Answer {
	if qtype != dns.TypeNS {
		return dnszones.Answer{Server: server}
	}
	switch dnszones.NormalizeDomain(name) {
	case demoZoneDelegated:
		return dnszones.Answer{Server: server, Values: []string{"ada.ns.cloudflare.com.", "bob.ns.cloudflare.com."}, TTL: 300}
	case demoZoneNotDelegated:
		return dnszones.Answer{Server: server, Values: demoRegistrarNS, TTL: 300}
	}
	return dnszones.Answer{Server: server, Error: "demo mode: no answer"}
}

// Registrar returns a fixed fake registrar for the demo zones.
func (d *demoZoneProvider) Registrar(_ context.Context, domain string) (dnszones.RegistrarHint, bool) {
	switch dnszones.NormalizeDomain(domain) {
	case demoZoneDelegated, demoZoneNotDelegated:
		return dnszones.RegistrarHint{Name: "Example Registrar (demo)", URL: "https://registrar.example"}, true
	}
	return dnszones.RegistrarHint{}, false
}
