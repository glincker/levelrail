package dnsrecords

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/libdns/libdns"

	"github.com/GLINCKER/levelrail/internal/dnszones"
)

// defaultRecordTTLSeconds applies when neither the caller nor an existing set names a TTL.
const defaultRecordTTLSeconds = 300

// ZoneManager adapts a dnszones.Provider to Manager, so providers without a
// libdns module (Google Cloud DNS) serve record automation and DNS-01 TXT writes.
type ZoneManager struct{ P dnszones.Provider }

// NewGoogleCloudManager builds a Manager from a service account JSON key.
func NewGoogleCloudManager(serviceAccountJSON string) (Manager, error) {
	p, err := dnszones.NewGoogleCloud(serviceAccountJSON)
	if err != nil {
		return nil, fmt.Errorf("dnsrecords: google cloud dns: %w", err)
	}
	return ZoneManager{P: p}, nil
}

func (m ZoneManager) zone(ctx context.Context, zone string) (dnszones.Zone, error) {
	zones, err := m.P.ListZones(ctx)
	if err != nil {
		return dnszones.Zone{}, fmt.Errorf("dnsrecords: list zones: %w", err)
	}
	z, err := dnszones.FindZone(zones, zone)
	if err != nil {
		return dnszones.Zone{}, ErrZoneNotFound
	}
	return z, nil
}

type setKey struct{ name, typ string }

func (m ZoneManager) sets(ctx context.Context, z dnszones.Zone) (map[setKey]dnszones.RecordSet, error) {
	all, err := m.P.ListRecordSets(ctx, z)
	if err != nil {
		return nil, fmt.Errorf("dnsrecords: list record sets: %w", err)
	}
	out := make(map[setKey]dnszones.RecordSet, len(all))
	for _, rs := range all {
		out[setKey{rs.Name, rs.Type}] = rs
	}
	return out, nil
}

// GetRecords implements Manager.
func (m ZoneManager) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	z, err := m.zone(ctx, zone)
	if err != nil {
		return nil, err
	}
	sets, err := m.sets(ctx, z)
	if err != nil {
		return nil, err
	}
	var out []libdns.Record
	for _, rs := range sets {
		for _, v := range rs.Values {
			out = append(out, libdns.RR{Name: rs.Name, Type: rs.Type, TTL: time.Duration(rs.TTL) * time.Second, Data: v})
		}
	}
	return out, nil
}

func group(recs []libdns.Record) map[setKey][]libdns.RR {
	g := map[setKey][]libdns.RR{}
	for _, r := range recs {
		rr := r.RR()
		k := setKey{dnszones.RelativeName(rr.Name, ""), rr.Type}
		g[k] = append(g[k], rr)
	}
	return g
}

func (m ZoneManager) write(ctx context.Context, zone string, recs []libdns.Record, merge bool) ([]libdns.Record, error) {
	z, err := m.zone(ctx, zone)
	if err != nil {
		return nil, err
	}
	cur, err := m.sets(ctx, z)
	if err != nil {
		return nil, err
	}
	for k, rrs := range group(recs) {
		rs := dnszones.RecordSet{Name: k.name, Type: k.typ, TTL: int(rrs[0].TTL / time.Second), Routing: dnszones.RoutingSimple}
		if merge {
			rs.Values = slices.Clone(cur[k].Values)
			if rs.TTL == 0 {
				rs.TTL = cur[k].TTL
			}
		}
		for _, rr := range rrs {
			if !slices.Contains(rs.Values, rr.Data) {
				rs.Values = append(rs.Values, rr.Data)
			}
		}
		if rs.TTL == 0 {
			rs.TTL = defaultRecordTTLSeconds
		}
		if err := m.P.UpsertRecordSet(ctx, z, rs); err != nil {
			return nil, fmt.Errorf("dnsrecords: write %s %s: %w", k.name, k.typ, err)
		}
	}
	return recs, nil
}

// AppendRecords implements Manager.
func (m ZoneManager) AppendRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	return m.write(ctx, zone, recs, true)
}

// SetRecords implements Manager.
func (m ZoneManager) SetRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	return m.write(ctx, zone, recs, false)
}

// DeleteRecords implements Manager. An empty Data on the input matches any value of that name and type.
func (m ZoneManager) DeleteRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	z, err := m.zone(ctx, zone)
	if err != nil {
		return nil, err
	}
	cur, err := m.sets(ctx, z)
	if err != nil {
		return nil, err
	}
	var deleted []libdns.Record
	for k, rrs := range group(recs) {
		rs, ok := cur[k]
		if !ok {
			continue
		}
		keep := slices.DeleteFunc(slices.Clone(rs.Values), func(v string) bool {
			for _, rr := range rrs {
				if rr.Data == "" || strings.EqualFold(rr.Data, v) {
					deleted = append(deleted, libdns.RR{Name: k.name, Type: k.typ, TTL: rr.TTL, Data: v})
					return true
				}
			}
			return false
		})
		if len(keep) == len(rs.Values) {
			continue
		}
		if len(keep) == 0 {
			err = m.P.DeleteRecordSet(ctx, z, rs.Key())
		} else {
			rs.Values = keep
			err = m.P.UpsertRecordSet(ctx, z, rs)
		}
		if err != nil {
			return nil, fmt.Errorf("dnsrecords: delete %s %s: %w", k.name, k.typ, err)
		}
	}
	return deleted, nil
}
