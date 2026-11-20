package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/libdns/libdns"
)

// errDNSManager fails the chosen operations with err and otherwise behaves like fakeDNSManager.
type errDNSManager struct {
	fakeDNSManager
	listErr, writeErr error
}

func (m *errDNSManager) GetRecords(ctx context.Context, z string) ([]libdns.Record, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.fakeDNSManager.GetRecords(ctx, z)
}

func (m *errDNSManager) AppendRecords(ctx context.Context, z string, recs []libdns.Record) ([]libdns.Record, error) {
	if m.writeErr != nil {
		return nil, m.writeErr
	}
	return m.fakeDNSManager.AppendRecords(ctx, z, recs)
}

func TestApplyDomainDNSSurfacesProviderErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mgr     *errDNSManager
		wantMsg string
	}{
		{"list fails", &errDNSManager{listErr: errors.New("AccessDenied: not authorized")}, "could not read the zone's records: AccessDenied"},
		{"write fails", &errDNSManager{writeErr: errors.New("InvalidChangeBatch: bad value")}, "the DNS provider rejected the record: InvalidChangeBatch"},
		{"unparseable record fails whole list", &errDNSManager{listErr: errors.New("failed to parse SRV record: expected format '_service._proto.name'")}, "failed to parse SRV record"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newGoLiveFixture(t, "203.0.113.5")
			target := &dnsTarget{Manager: tc.mgr, Provider: "route53", Zone: "example.com."}
			f.rt.domainAuto.resolveTarget = func(context.Context, string) (*dnsTarget, error) { return target, nil }
			res, _ := f.rt.applyDomainDNS(context.Background(), nil, "web", "app.example.com", dnsOpts{Mode: dnsModeAuto, CanWrite: true})
			if res.DNS != dnsResultError || !strings.Contains(res.Message, tc.wantMsg) {
				t.Fatalf("result = %+v, want message containing %q", res, tc.wantMsg)
			}
		})
	}
}
