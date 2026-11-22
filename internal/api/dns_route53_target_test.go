package api

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/store"
)

// route53ZonesRouter connects a Route53 zone provider whose credentials list
// two zones, the shape a key scoped to one hosted zone plus ListHostedZones
// has.
func route53ZonesRouter(t *testing.T) (*Router, *fakeZoneProvider) {
	t.Helper()
	rt, _ := newTestRouter(t)
	rt.publicHost = "203.0.113.5"
	rt.detectPublicIPs = func(context.Context) []string { return nil }
	p := newFakeZoneProvider()
	p.name = "route53"
	p.zones = []dnszones.Zone{
		{ID: "Z1", Name: "apps.example.com", Provider: "route53"},
		{ID: "Z2", Name: "other.org", Provider: "route53"},
		{ID: "Z3", Name: "internal.example.net", Provider: "route53", Private: true},
	}
	p.sets = map[string][]dnszones.RecordSet{"Z1": {}, "Z2": {}, "Z3": {}}
	rt.dnsZoneProviders = func(context.Context) (map[string]dnszones.Provider, error) {
		return map[string]dnszones.Provider{p.name: p}, nil
	}
	return rt, p
}

func TestRoute53ZoneLookupUsesListedZones(t *testing.T) {
	rt, _ := route53ZonesRouter(t)
	for domain, zone := range map[string]string{
		"app.apps.example.com":     "apps.example.com",
		"app.run.apps.example.com": "apps.example.com",
		"app.other.org":            "other.org",
	} {
		got := rt.zoneLookup(context.Background(), domain)
		if !got.Found || got.Zone != zone || got.Provider != "route53" {
			t.Fatalf("zoneLookup(%q) = %+v, want zone %q", domain, got, zone)
		}
	}
	for _, domain := range []string{"app.unlisted.net", "app.internal.example.net"} {
		if got := rt.zoneLookup(context.Background(), domain); got.Found {
			t.Fatalf("zoneLookup(%q) = %+v, want no zone", domain, got)
		}
	}
}

func TestAppsBaseDomainAcceptedInsideListedRoute53Zone(t *testing.T) {
	rt, _ := route53ZonesRouter(t)
	for _, base := range []string{"apps.example.com", "run.apps.example.com", "apps.other.org"} {
		upd := ingressSettingsUpdate{AppsBaseDomain: &base}
		if msg, _ := rt.validateDNSAutomationFields(context.Background(), upd); msg != "" {
			t.Fatalf("apps_base_domain %q rejected: %s", base, msg)
		}
	}
	base := "apps.unlisted.net"
	if msg, _ := rt.validateDNSAutomationFields(context.Background(), ingressSettingsUpdate{AppsBaseDomain: &base}); msg == "" {
		t.Fatal("a name outside every listed zone must be rejected")
	}
}

func TestAddDomainCreatesRecordThroughRoute53(t *testing.T) {
	rt, p := route53ZonesRouter(t)
	if err := rt.ingressSettings.UpdateIngressSettings(context.Background(), store.IngressSettings{}); err != nil {
		t.Fatal(err)
	}
	res, undo := rt.applyDomainDNS(context.Background(), nil, "web", "app.apps.example.com", dnsOpts{Mode: dnsModeAuto, CanWrite: true})
	if res.DNS != dnsrecords.OutcomeCreated || res.Zone != "apps.example.com." || res.Provider != "route53" {
		t.Fatalf("result = %+v", res)
	}
	sets := p.sets["Z1"]
	if len(sets) != 1 || sets[0].Name != "app" || sets[0].Type != "A" || sets[0].Values[0] != "203.0.113.5" {
		t.Fatalf("record sets in the matched zone = %+v", sets)
	}
	if len(undo) != 1 || undo[0].Kind != undoKindDNSRecord {
		t.Fatalf("undo = %+v", undo)
	}
}

func TestAddDomainWritesToPublicZoneSharingNameWithPrivateZone(t *testing.T) {
	rt, p := route53ZonesRouter(t)
	p.zones = append([]dnszones.Zone{{ID: "Zprivate", Name: "apps.example.com", Provider: "route53", Private: true}}, p.zones...)
	p.sets["Zprivate"] = nil
	res, _ := rt.applyDomainDNS(context.Background(), nil, "web", "app.apps.example.com", dnsOpts{Mode: dnsModeAuto, CanWrite: true})
	if res.DNS != dnsrecords.OutcomeCreated {
		t.Fatalf("result = %+v", res)
	}
	if len(p.sets["Zprivate"]) != 0 || len(p.sets["Z1"]) != 1 {
		t.Fatalf("private = %+v, public = %+v", p.sets["Zprivate"], p.sets["Z1"])
	}
}

func TestRemoveManagedDNSDeletesFromTheZoneItWasWrittenIn(t *testing.T) {
	rt, p := route53ZonesRouter(t)
	ctx := context.Background()
	if res, _ := rt.applyDomainDNS(ctx, nil, "web", "app.apps.example.com", dnsOpts{Mode: dnsModeAuto, CanWrite: true}); res.DNS != dnsrecords.OutcomeCreated {
		t.Fatalf("result = %+v", res)
	}
	// A more specific zone appears later and now matches the domain first.
	p.zones = append(p.zones, dnszones.Zone{ID: "Z4", Name: "app.apps.example.com", Provider: "route53"})
	p.sets["Z4"] = nil
	out := rt.removeManagedDNS(ctx, nil, "web", "app.apps.example.com")
	if len(out) != 1 || out[0].DNS != "deleted" || out[0].Zone != "apps.example.com." {
		t.Fatalf("result = %+v", out)
	}
	if len(p.sets["Z1"]) != 0 {
		t.Fatalf("the record stayed in the original zone: %+v", p.sets["Z1"])
	}
}
