package plugin

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
)

// ErrNotDNSProvider is returned when a DNS call names a plugin of another kind.
var ErrNotDNSProvider = errors.New("plugin: not a dns_provider")

const maxDNSZones = 500

// DNSRecord is one record in a zone, named relative to it ("@" for the apex).
type DNSRecord struct {
	Name    string
	Type    string
	Value   string
	TTL     time.Duration
	Proxied bool
}

// DNSError is a failure the plugin itself reported. Detail has the call's
// secret removed and is safe to show an operator.
type DNSError struct {
	Op        string
	Detail    string
	Retryable bool
}

func (e *DNSError) Error() string { return fmt.Sprintf("plugin: dns %s failed: %s", e.Op, e.Detail) }

func (r DNSRecord) pb() *pluginpb.DnsRecord {
	return &pluginpb.DnsRecord{Name: r.Name, Type: r.Type, Value: r.Value, TtlSeconds: uint32(max(0, int(r.TTL.Seconds()))), Proxied: r.Proxied} //nolint:gosec // TTL is clamped to a non-negative int of seconds
}

func dnsRecordFromPB(r *pluginpb.DnsRecord) DNSRecord {
	return DNSRecord{Name: r.GetName(), Type: r.GetType(), Value: r.GetValue(), TTL: time.Duration(r.GetTtlSeconds()) * time.Second, Proxied: r.GetProxied()}
}

// DNSListZones returns the zone names the credential can manage.
func (m *Manager) DNSListZones(ctx context.Context, name, secret string) ([]string, error) {
	var zones []string
	err := m.dnsCall(ctx, name, secret, "list_zones", "", func(cctx context.Context, c dnsClient, cfg string) (*pluginpb.DnsOutcome, error) {
		r, err := c.ListZones(cctx, &pluginpb.ListZonesRequest{ConfigJson: cfg, Secret: secret})
		if err != nil {
			return nil, err
		}
		zones = slices.Clone(r.GetZones())
		if len(zones) > maxDNSZones {
			zones = zones[:maxDNSZones]
		}
		return r.GetOutcome(), nil
	})
	return zones, err
}

// DNSListRecords returns every record the provider holds in zone.
func (m *Manager) DNSListRecords(ctx context.Context, name, secret, zone string) ([]DNSRecord, error) {
	var out []DNSRecord
	err := m.dnsCall(ctx, name, secret, "list_records", zone, func(cctx context.Context, c dnsClient, cfg string) (*pluginpb.DnsOutcome, error) {
		r, err := c.ListRecords(cctx, &pluginpb.ListRecordsRequest{Zone: zone, ConfigJson: cfg, Secret: secret})
		if err != nil {
			return nil, err
		}
		for _, rec := range r.GetRecords() {
			out = append(out, dnsRecordFromPB(rec))
		}
		return r.GetOutcome(), nil
	})
	return out, err
}

// DNSUpsert creates the record or replaces the one with the same name and type.
func (m *Manager) DNSUpsert(ctx context.Context, name, secret, zone string, rec DNSRecord) error {
	return m.dnsCall(ctx, name, secret, "upsert_record", zone, func(cctx context.Context, c dnsClient, cfg string) (*pluginpb.DnsOutcome, error) {
		r, err := c.UpsertRecord(cctx, &pluginpb.UpsertRecordRequest{Zone: zone, Record: rec.pb(), ConfigJson: cfg, Secret: secret})
		if err != nil {
			return nil, err
		}
		return r.GetOutcome(), nil
	})
}

// DNSDelete removes the record equal to rec in name, type and value.
func (m *Manager) DNSDelete(ctx context.Context, name, secret, zone string, rec DNSRecord) error {
	return m.dnsCall(ctx, name, secret, "delete_record", zone, func(cctx context.Context, c dnsClient, cfg string) (*pluginpb.DnsOutcome, error) {
		r, err := c.DeleteRecord(cctx, &pluginpb.DeleteRecordRequest{Zone: zone, Record: rec.pb(), ConfigJson: cfg, Secret: secret})
		if err != nil {
			return nil, err
		}
		return r.GetOutcome(), nil
	})
}

// DNSVerify reports whether the provider now serves exactly rec.
func (m *Manager) DNSVerify(ctx context.Context, name, secret, zone string, rec DNSRecord) (bool, error) {
	present := false
	err := m.dnsCall(ctx, name, secret, "verify_record", zone, func(cctx context.Context, c dnsClient, cfg string) (*pluginpb.DnsOutcome, error) {
		r, err := c.VerifyRecord(cctx, &pluginpb.VerifyRecordRequest{Zone: zone, Record: rec.pb(), ConfigJson: cfg, Secret: secret})
		if err != nil {
			return nil, err
		}
		present = r.GetPresent()
		return r.GetOutcome(), nil
	})
	return present, err
}

func (m *Manager) dnsCall(ctx context.Context, name, secret, op, zone string, fn func(cctx context.Context, c dnsClient, cfg string) (*pluginpb.DnsOutcome, error)) error {
	rec, err := m.store.GetPlugin(ctx, name)
	if err != nil {
		return err
	}
	if rec.Kind != KindDNSProvider {
		return ErrNotDNSProvider
	}
	if rec.State != StateEnabled {
		return fmt.Errorf("%w: plugin is %s", ErrUnavailable, rec.State)
	}
	if !slices.Contains(rec.ApprovedAbilities, AbilityRead) {
		return ErrNotApproved
	}
	if len(secret) > m.cfg.MaxSecretBytes || len(rec.ConfigJSON) > m.cfg.MaxConfigBytes {
		return ErrTooLarge
	}
	in := m.inst(name)
	client, err := m.admit(in)
	if err != nil {
		return err
	}
	defer func() { <-in.sem }()
	if err := m.audit.Record(ctx, AuditEvent{Action: AuditAbilityUsed, Actor: "plugin:" + name, Plugin: name, Detail: map[string]string{
		"call": op, "abilities": AbilityRead, "zone": truncateUTF8(zone, 253),
	}}); err != nil {
		return fmt.Errorf("plugin: audit failed, call refused: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, m.cfg.CallTimeout)
	defer cancel()
	out, err := fn(cctx, client, rec.ConfigJSON)
	if err != nil {
		m.noteCallResult(in, false)
		return fmt.Errorf("plugin: dns %s via %q failed: %w", op, name, redactSecret(err, secret))
	}
	m.noteCallResult(in, true)
	if !out.GetOk() {
		return &DNSError{Op: op, Detail: m.boundedDetail(out.GetDetail(), secret), Retryable: out.GetRetryable()}
	}
	return nil
}
