package demo

import (
	"context"
	"errors"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/dnszones"
)

var errNoRecord = errors.New("demo zone: no such record")

// Zone is an in-memory DNS provider and resolver sharing one record store, so
// a record applied through it is what the check later resolves.
type Zone struct {
	mu   sync.Mutex
	sets map[string][]dnszones.RecordSet // by zone name
}

// NewZone returns an empty demo zone store.
func NewZone() *Zone { return &Zone{sets: map[string][]dnszones.RecordSet{}} }

// Reset forgets every applied record, so the checklist fails again.
func (z *Zone) Reset() {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.sets = map[string][]dnszones.RecordSet{}
}

// ZoneFor is the fake zone object for a domain.
func (z *Zone) ZoneFor(domain string) dnszones.Zone {
	return dnszones.Zone{ID: domain, Name: domain, Provider: ProviderName, Status: "demo", NameServers: []string{"ns1.demo.example", "ns2.demo.example"}}
}

// Name implements dnszones.Provider and the mailrecords resolver.
func (z *Zone) Name() string { return ProviderName }

// Capabilities implements dnszones.Provider and the mailrecords resolver.
func (z *Zone) Capabilities() dnszones.Capabilities { return dnszones.Capabilities{} }

// ListZones implements dnszones.Provider and the mailrecords resolver.
func (z *Zone) ListZones(context.Context) ([]dnszones.Zone, error) {
	z.mu.Lock()
	defer z.mu.Unlock()
	var out []dnszones.Zone
	for name := range z.sets {
		out = append(out, z.ZoneFor(name))
	}
	return out, nil
}

// GetZone implements dnszones.Provider and the mailrecords resolver.
func (z *Zone) GetZone(_ context.Context, id string) (dnszones.Zone, error) {
	return z.ZoneFor(id), nil
}

// CreateZone implements dnszones.Provider and the mailrecords resolver.
func (z *Zone) CreateZone(_ context.Context, name string) (dnszones.Zone, error) {
	return z.ZoneFor(name), nil
}

// DeleteZone implements dnszones.Provider and the mailrecords resolver.
func (z *Zone) DeleteZone(_ context.Context, id string) error {
	z.mu.Lock()
	defer z.mu.Unlock()
	delete(z.sets, id)
	return nil
}

// ListRecordSets implements dnszones.Provider and the mailrecords resolver.
func (z *Zone) ListRecordSets(_ context.Context, zone dnszones.Zone) ([]dnszones.RecordSet, error) {
	z.mu.Lock()
	defer z.mu.Unlock()
	out := slices.Clone(z.sets[zone.Name])
	for i := range out {
		out[i].Values = slices.Clone(out[i].Values)
	}
	return out, nil
}

// UpsertRecordSet implements dnszones.Provider and the mailrecords resolver.
func (z *Zone) UpsertRecordSet(_ context.Context, zone dnszones.Zone, rs dnszones.RecordSet) error {
	z.mu.Lock()
	defer z.mu.Unlock()
	rs.Values = slices.Clone(rs.Values)
	list := z.sets[zone.Name]
	for i, have := range list {
		if have.Key() == rs.Key() {
			list[i] = rs
			return nil
		}
	}
	z.sets[zone.Name] = append(list, rs)
	return nil
}

// DeleteRecordSet implements dnszones.Provider and the mailrecords resolver.
func (z *Zone) DeleteRecordSet(_ context.Context, zone dnszones.Zone, key dnszones.Key) error {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.sets[zone.Name] = slices.DeleteFunc(z.sets[zone.Name], func(rs dnszones.RecordSet) bool { return rs.Key() == key })
	return nil
}

// fqdn makes a stored record name absolute and lower case inside zone.
func fqdn(name, zone string) string {
	n := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
	if n == "" || n == "@" {
		return zone
	}
	if n == zone || strings.HasSuffix(n, "."+zone) {
		return n
	}
	return n + "." + zone
}

// find returns the values of every set of type typ at name, across zones.
func (z *Zone) find(name, typ string) []string {
	z.mu.Lock()
	defer z.mu.Unlock()
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	var out []string
	for zone, list := range z.sets {
		for _, rs := range list {
			if rs.Type == typ && fqdn(rs.Name, zone) == name {
				out = append(out, rs.Values...)
			}
		}
	}
	return out
}

// LookupTXT implements dnszones.Provider and the mailrecords resolver.
func (z *Zone) LookupTXT(_ context.Context, name string) ([]string, error) {
	vals := z.find(name, "TXT")
	if len(vals) == 0 {
		return nil, errNoRecord
	}
	for i, v := range vals {
		vals[i] = strings.ReplaceAll(strings.Trim(v, `"`), `" "`, "")
	}
	return vals, nil
}

// LookupCNAME implements dnszones.Provider and the mailrecords resolver.
func (z *Zone) LookupCNAME(_ context.Context, name string) (string, error) {
	if v := z.find(name, "CNAME"); len(v) > 0 {
		return v[0], nil
	}
	return "", errNoRecord
}

// LookupMX implements dnszones.Provider and the mailrecords resolver.
func (z *Zone) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	var out []*net.MX
	for _, v := range z.find(name, "MX") {
		pref, host, ok := strings.Cut(strings.TrimSpace(v), " ")
		n, err := strconv.Atoi(pref)
		if !ok || err != nil || n < 0 || n > 65535 {
			continue
		}
		out = append(out, &net.MX{Host: strings.TrimSuffix(host, ".") + ".", Pref: uint16(n)})
	}
	if len(out) == 0 {
		return nil, errNoRecord
	}
	return out, nil
}

// LookupHost implements dnszones.Provider and the mailrecords resolver.
func (z *Zone) LookupHost(_ context.Context, host string) ([]string, error) {
	v := append(z.find(host, "A"), z.find(host, "AAAA")...)
	if len(v) == 0 {
		return nil, errNoRecord
	}
	return v, nil
}

// LookupAddr plays the hosting provider's reverse DNS: any address the zone
// maps from a host name reverses to that name.
func (z *Zone) LookupAddr(_ context.Context, addr string) ([]string, error) {
	z.mu.Lock()
	defer z.mu.Unlock()
	for zone, list := range z.sets {
		for _, rs := range list {
			if (rs.Type == "A" || rs.Type == "AAAA") && slices.Contains(rs.Values, addr) {
				return []string{fqdn(rs.Name, zone) + "."}, nil
			}
		}
	}
	return nil, errNoRecord
}

// LookupSRV implements dnszones.Provider and the mailrecords resolver.
func (z *Zone) LookupSRV(context.Context, string, string, string) (string, []*net.SRV, error) {
	return "", nil, errNoRecord
}
