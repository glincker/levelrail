package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/libdns/libdns"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/plugin"
)

// errDNSNotConfirmed means a plugin accepted a write but did not serve the record afterwards.
var errDNSNotConfirmed = errors.New("the DNS plugin accepted the record but did not confirm it is live")

// dnsPluginCaller is the part of the plugin runtime the DNS path uses.
type dnsPluginCaller interface {
	DNSListZones(ctx context.Context, name, secret string) ([]string, error)
	DNSListRecords(ctx context.Context, name, secret, zone string) ([]plugin.DNSRecord, error)
	DNSUpsert(ctx context.Context, name, secret, zone string, rec plugin.DNSRecord) error
	DNSDelete(ctx context.Context, name, secret, zone string, rec plugin.DNSRecord) error
	DNSVerify(ctx context.Context, name, secret, zone string, rec plugin.DNSRecord) (bool, error)
}

func (rt *Router) dnsPlugins() dnsPluginCaller {
	if rt.domainAuto.pluginDNS != nil {
		return rt.domainAuto.pluginDNS
	}
	if rt.plugins != nil {
		return rt.plugins
	}
	return nil
}

// pluginDNSManager adapts an installed dns_provider plugin to dnsrecords.Manager.
// The credential is handed to the plugin on every call and never kept here.
type pluginDNSManager struct {
	plugins dnsPluginCaller
	name    string
	token   string
}

func toPluginRecord(rr libdns.RR) plugin.DNSRecord {
	return plugin.DNSRecord{Name: rr.Name, Type: rr.Type, Value: rr.Data, TTL: rr.TTL}
}

func (p pluginDNSManager) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	recs, err := p.plugins.DNSListRecords(ctx, p.name, p.token, zone)
	if err != nil {
		return nil, err
	}
	out := make([]libdns.Record, 0, len(recs))
	for _, r := range recs {
		out = append(out, libdns.RR{Name: r.Name, Type: r.Type, Data: r.Value, TTL: r.TTL})
	}
	return out, nil
}

// write upserts each record, then asks the plugin to verify it. A write the
// plugin accepted but cannot see afterwards is reported, not assumed good.
func (p pluginDNSManager) write(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	for i, rec := range recs {
		pr := toPluginRecord(rec.RR())
		if err := p.plugins.DNSUpsert(ctx, p.name, p.token, zone, pr); err != nil {
			return recs[:i], err
		}
		ok, err := p.plugins.DNSVerify(ctx, p.name, p.token, zone, pr)
		if err != nil {
			return recs[:i+1], fmt.Errorf("%w: %w", errDNSNotConfirmed, err)
		}
		if !ok {
			return recs[:i+1], errDNSNotConfirmed
		}
	}
	return recs, nil
}

func (p pluginDNSManager) AppendRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	return p.write(ctx, zone, recs)
}

func (p pluginDNSManager) SetRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	return p.write(ctx, zone, recs)
}

func (p pluginDNSManager) DeleteRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	for i, rec := range recs {
		if err := p.plugins.DNSDelete(ctx, p.name, p.token, zone, toPluginRecord(rec.RR())); err != nil {
			return recs[:i], err
		}
	}
	return recs, nil
}

// dnsProviderPluginName is the dns_provider plugin the operator chose, or "".
func (rt *Router) dnsProviderPluginName(ctx context.Context) string {
	if rt.dnsPlugins() == nil {
		return ""
	}
	return rt.storedAutomation(ctx).dnsPluginName()
}

// pluginDNSTarget builds the target for domain through the chosen plugin. It
// returns (nil, nil) when no plugin is chosen so the built-in path runs.
func (rt *Router) pluginDNSTarget(ctx context.Context, domain string) (*dnsTarget, error) {
	name := rt.dnsProviderPluginName(ctx)
	if name == "" {
		return nil, nil
	}
	token, err := rt.cloudflareDNSToken(ctx)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("the DNS plugin %q needs the Cloudflare DNS credential to be connected first", name)
	}
	callee := rt.dnsPlugins()
	mgr := pluginDNSManager{plugins: callee, name: name, token: token}
	zones, err := callee.DNSListZones(ctx, name, token)
	if err != nil {
		return nil, err
	}
	zone, ok := dnsrecords.MatchZone(domain, zones)
	if !ok {
		return nil, dnsrecords.ErrZoneNotFound
	}
	return &dnsTarget{Manager: mgr, Provider: dnsProviderCloudflare, Zone: zone, Plugin: name}, nil
}

// dnsPluginTestResult is the outcome of testing a dns_provider plugin.
type dnsPluginTestResult struct {
	OK      bool     `json:"ok"`
	Zones   []string `json:"zones,omitempty"`
	Message string   `json:"message"`
}

// testDNSPlugin lists zones through the plugin with the stored credential.
func (rt *Router) testDNSPlugin(ctx context.Context, name string) dnsPluginTestResult {
	token, err := rt.cloudflareDNSToken(ctx)
	if err != nil || token == "" {
		return dnsPluginTestResult{Message: "connect the Cloudflare DNS credential first, the plugin uses it for each call"}
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	zones, err := rt.dnsPlugins().DNSListZones(ctx, name, token)
	var de *plugin.DNSError
	switch {
	case errors.As(err, &de):
		return dnsPluginTestResult{Message: de.Detail}
	case err != nil:
		return dnsPluginTestResult{Message: pluginCallMessage(err)}
	}
	return dnsPluginTestResult{OK: true, Zones: zones, Message: fmt.Sprintf("the plugin can see %d zones", len(zones))}
}

func pluginCallMessage(err error) string {
	switch {
	case errors.Is(err, plugin.ErrUnavailable):
		return "the plugin is not running, enable it first"
	case errors.Is(err, plugin.ErrNotApproved):
		return "approve the plugin's abilities first"
	case errors.Is(err, plugin.ErrCircuitOpen):
		return "the plugin was paused after repeated failures, try again shortly"
	case errors.Is(err, plugin.ErrBusy):
		return "the plugin is busy, try again"
	}
	return "the plugin call failed"
}
