package pluginkit

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
)

// DNSCall is the config and per-call credential every DNS operation receives.
// The secret must never be stored or logged by the plugin.
type DNSCall struct {
	Config json.RawMessage
	Secret string
	Zone   string
	Record *pluginpb.DnsRecord
}

// DNSOutcome is the plugin's verdict on one DNS operation.
type DNSOutcome struct {
	OK        bool
	Retryable bool
	Detail    string
}

// DNSProvider implements the dns_provider contract for one DNS service.
type DNSProvider struct {
	ListZones   func(ctx context.Context, c DNSCall) ([]string, DNSOutcome)
	ListRecords func(ctx context.Context, c DNSCall) ([]*pluginpb.DnsRecord, DNSOutcome)
	Upsert      func(ctx context.Context, c DNSCall) DNSOutcome
	Delete      func(ctx context.Context, c DNSCall) DNSOutcome
	Verify      func(ctx context.Context, c DNSCall) (bool, DNSOutcome)
}

func (o DNSOutcome) pb() *pluginpb.DnsOutcome {
	return &pluginpb.DnsOutcome{Ok: o.OK, Retryable: o.Retryable, Detail: o.Detail}
}

func dnsConfig(raw string) json.RawMessage {
	if strings.TrimSpace(raw) == "" {
		return json.RawMessage("{}")
	}
	return json.RawMessage(raw)
}

func (s server) ListZones(ctx context.Context, r *pluginpb.ListZonesRequest) (*pluginpb.ListZonesResponse, error) {
	zones, out := s.p.DNS.ListZones(ctx, DNSCall{Config: dnsConfig(r.GetConfigJson()), Secret: r.GetSecret()})
	return &pluginpb.ListZonesResponse{Outcome: out.pb(), Zones: zones}, nil
}

func (s server) ListRecords(ctx context.Context, r *pluginpb.ListRecordsRequest) (*pluginpb.ListRecordsResponse, error) {
	recs, out := s.p.DNS.ListRecords(ctx, DNSCall{Config: dnsConfig(r.GetConfigJson()), Secret: r.GetSecret(), Zone: r.GetZone()})
	return &pluginpb.ListRecordsResponse{Outcome: out.pb(), Records: recs}, nil
}

func (s server) UpsertRecord(ctx context.Context, r *pluginpb.UpsertRecordRequest) (*pluginpb.UpsertRecordResponse, error) {
	out := s.p.DNS.Upsert(ctx, DNSCall{Config: dnsConfig(r.GetConfigJson()), Secret: r.GetSecret(), Zone: r.GetZone(), Record: r.GetRecord()})
	return &pluginpb.UpsertRecordResponse{Outcome: out.pb()}, nil
}

func (s server) DeleteRecord(ctx context.Context, r *pluginpb.DeleteRecordRequest) (*pluginpb.DeleteRecordResponse, error) {
	out := s.p.DNS.Delete(ctx, DNSCall{Config: dnsConfig(r.GetConfigJson()), Secret: r.GetSecret(), Zone: r.GetZone(), Record: r.GetRecord()})
	return &pluginpb.DeleteRecordResponse{Outcome: out.pb()}, nil
}

func (s server) VerifyRecord(ctx context.Context, r *pluginpb.VerifyRecordRequest) (*pluginpb.VerifyRecordResponse, error) {
	present, out := s.p.DNS.Verify(ctx, DNSCall{Config: dnsConfig(r.GetConfigJson()), Secret: r.GetSecret(), Zone: r.GetZone(), Record: r.GetRecord()})
	return &pluginpb.VerifyRecordResponse{Outcome: out.pb(), Present: present}, nil
}
