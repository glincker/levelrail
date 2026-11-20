package dnsrecords

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/libdns/libdns"

	"github.com/GLINCKER/levelrail/internal/dnszones"
)

// setsProvider serves fixed record sets and records upserts and deletes.
type setsProvider struct {
	dnszones.Provider
	sets    []dnszones.RecordSet
	listErr error
	upserts []dnszones.RecordSet
	deletes []dnszones.Key
}

func (p *setsProvider) ListZones(context.Context) ([]dnszones.Zone, error) {
	return []dnszones.Zone{{ID: "Z1", Name: "apps.example.com"}}, nil
}

func (p *setsProvider) ListRecordSets(context.Context, dnszones.Zone) ([]dnszones.RecordSet, error) {
	return p.sets, p.listErr
}

func (p *setsProvider) UpsertRecordSet(_ context.Context, _ dnszones.Zone, rs dnszones.RecordSet) error {
	p.upserts = append(p.upserts, rs)
	return nil
}

func (p *setsProvider) DeleteRecordSet(_ context.Context, _ dnszones.Zone, k dnszones.Key) error {
	p.deletes = append(p.deletes, k)
	return nil
}

// brokenLibdns fails every listing the way the libdns module does on an odd SRV, and records writes.
type brokenLibdns struct{ appended []libdns.Record }

func (b *brokenLibdns) GetRecords(context.Context, string) ([]libdns.Record, error) {
	return nil, errors.New("failed to parse SRV record: expected format '_service._proto.name'")
}

func (b *brokenLibdns) AppendRecords(_ context.Context, _ string, r []libdns.Record) ([]libdns.Record, error) {
	b.appended = append(b.appended, r...)
	return r, nil
}

func (b *brokenLibdns) SetRecords(context.Context, string, []libdns.Record) ([]libdns.Record, error) {
	return nil, nil
}

func (b *brokenLibdns) DeleteRecords(context.Context, string, []libdns.Record) ([]libdns.Record, error) {
	return nil, nil
}

func TestTolerantListManager(t *testing.T) {
	t.Parallel()
	odd := dnszones.RecordSet{Name: "srv", Type: "SRV", TTL: 300, Values: []string{"1 1 1 x.example.com"}}
	tests := []struct {
		name        string
		sets        []dnszones.RecordSet
		listErr     error
		desired     Desired
		wantErr     string
		wantOutcome string
	}{
		{"odd SRV in zone, target created", []dnszones.RecordSet{odd}, nil, Desired{Name: "run", Type: "A", Value: "203.0.113.5", TTL: 5 * time.Minute}, "", OutcomeCreated},
		{"odd SRV and same A already there", []dnszones.RecordSet{odd, {Name: "run", Type: "A", TTL: 300, Values: []string{"203.0.113.5"}}}, nil, Desired{Name: "run", Type: "A", Value: "203.0.113.5"}, "", OutcomeUnchanged},
		{"odd SRV and a different A conflicts", []dnszones.RecordSet{odd, {Name: "run", Type: "A", TTL: 300, Values: []string{"198.51.100.1"}}}, nil, Desired{Name: "run", Type: "A", Value: "203.0.113.5"}, "", OutcomeConflict},
		{"true provider error surfaces", nil, errors.New("AccessDenied"), Desired{Name: "run", Type: "A", Value: "203.0.113.5"}, "AccessDenied", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := &setsProvider{sets: tc.sets, listErr: tc.listErr}
			inner := &brokenLibdns{}
			m := tolerantListManager{Manager: inner, lister: ZoneManager{P: p}}
			recs, err := m.GetRecords(context.Background(), "apps.example.com.")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("list must survive the odd record: %v", err)
			}
			plan := PlanRecord(tc.desired, recs, false)
			if plan.Outcome != tc.wantOutcome {
				t.Fatalf("outcome = %q, want %q", plan.Outcome, tc.wantOutcome)
			}
			if plan.Outcome == OutcomeCreated {
				rr := libdns.RR{Name: tc.desired.Name, Type: tc.desired.Type, Data: tc.desired.Value, TTL: tc.desired.TTL}
				if _, err := m.AppendRecords(context.Background(), "apps.example.com.", []libdns.Record{rr}); err != nil {
					t.Fatal(err)
				}
				if len(inner.appended) != 1 {
					t.Fatalf("write must go to the wrapped manager, got %d", len(inner.appended))
				}
			}
			if len(p.upserts) != 0 || len(p.deletes) != 0 {
				t.Fatalf("the odd record must be untouched: upserts=%v deletes=%v", p.upserts, p.deletes)
			}
		})
	}
}
