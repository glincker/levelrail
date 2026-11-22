package dnszones

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// ProviderDeSEC names the deSEC DNS provider.
const ProviderDeSEC = "desec"

// DeSECBaseURL is the deSEC API root.
const DeSECBaseURL = "https://desec.io/api/v1"

// DeSEC implements Provider over deSEC's rrset API with an API token.
type DeSEC struct {
	rc restClient
}

// NewDeSEC returns a client for token, sent only in the Authorization header.
func NewDeSEC(token string) *DeSEC {
	d := &DeSEC{}
	d.rc = restClient{provider: ProviderDeSEC, base: DeSECBaseURL, header: func(r *http.Request) {
		r.Header.Set("Authorization", "Token "+strings.TrimSpace(token))
	}}
	return d
}

// SetBaseURL points the client at another API root (tests).
func (d *DeSEC) SetBaseURL(base string, c *http.Client) { d.rc.base, d.rc.http = base, c }

// Name implements Provider.
func (d *DeSEC) Name() string { return ProviderDeSEC }

// Capabilities implements Provider.
func (d *DeSEC) Capabilities() Capabilities { return Capabilities{} }

type desecDomain struct {
	Name string `json:"name"`
}

func (z desecDomain) toZone() Zone {
	n := NormalizeDomain(z.Name)
	return Zone{ID: n, Name: n, Provider: ProviderDeSEC}
}

// ListZones implements Provider.
func (d *DeSEC) ListZones(ctx context.Context) ([]Zone, error) {
	var ds []desecDomain
	if err := d.rc.do(ctx, http.MethodGet, "/domains/", nil, &ds); err != nil {
		return nil, err
	}
	out := make([]Zone, 0, len(ds))
	for _, z := range ds {
		out = append(out, z.toZone())
	}
	return out, nil
}

// GetZone implements Provider.
func (d *DeSEC) GetZone(ctx context.Context, id string) (Zone, error) {
	var z desecDomain
	if err := d.rc.do(ctx, http.MethodGet, "/domains/"+url.PathEscape(id)+"/", nil, &z); err != nil {
		if isNotFound(err) {
			return Zone{}, ErrZoneNotFound
		}
		return Zone{}, err
	}
	return z.toZone(), nil
}

// CreateZone implements Provider.
func (d *DeSEC) CreateZone(ctx context.Context, name string) (Zone, error) {
	var z desecDomain
	if err := d.rc.do(ctx, http.MethodPost, "/domains/", map[string]any{"name": NormalizeDomain(name)}, &z); err != nil {
		return Zone{}, err
	}
	return z.toZone(), nil
}

// DeleteZone implements Provider.
func (d *DeSEC) DeleteZone(ctx context.Context, id string) error {
	return d.rc.do(ctx, http.MethodDelete, "/domains/"+url.PathEscape(id)+"/", nil, nil)
}

type desecRRSet struct {
	SubName string   `json:"subname"`
	Type    string   `json:"type"`
	TTL     int      `json:"ttl"`
	Records []string `json:"records"`
}

// subPath addresses one rrset; deSEC spells the apex "@" in the URL and "" in the body.
func (d *DeSEC) subPath(zone Zone, name, typ string) string {
	if name == Apex {
		name = "@"
	}
	return "/domains/" + url.PathEscape(zone.Name) + "/rrsets/" + url.PathEscape(name) + "/" + url.PathEscape(typ) + "/"
}

// ListRecordSets implements Provider.
func (d *DeSEC) ListRecordSets(ctx context.Context, zone Zone) ([]RecordSet, error) {
	var sets []desecRRSet
	if err := d.rc.do(ctx, http.MethodGet, "/domains/"+url.PathEscape(zone.Name)+"/rrsets/", nil, &sets); err != nil {
		return nil, err
	}
	var out []RecordSet
	for _, s := range sets {
		if s.Type == "SOA" {
			continue
		}
		vals := make([]string, 0, len(s.Records))
		for _, v := range s.Records {
			if s.Type == "TXT" {
				v = unquoteTXT(v)
			}
			vals = append(vals, v)
		}
		out = append(out, RecordSet{Name: RelativeName(s.SubName, ""), Type: s.Type, TTL: s.TTL, Values: vals, Routing: RoutingSimple})
	}
	SortSets(out)
	return out, nil
}

// UpsertRecordSet implements Provider. The PUT replaces the whole rrset in one call.
func (d *DeSEC) UpsertRecordSet(ctx context.Context, zone Zone, rs RecordSet) error {
	vals := make([]string, 0, len(rs.Values))
	for _, v := range rs.Values {
		if rs.Type == "TXT" {
			v = quoteTXT(v)
		}
		vals = append(vals, v)
	}
	sub := rs.Name
	if sub == Apex {
		sub = ""
	}
	body := desecRRSet{SubName: sub, Type: rs.Type, TTL: rs.TTL, Records: vals}
	return d.rc.do(ctx, http.MethodPut, d.subPath(zone, rs.Name, rs.Type), body, nil)
}

// DeleteRecordSet implements Provider.
func (d *DeSEC) DeleteRecordSet(ctx context.Context, zone Zone, k Key) error {
	err := d.rc.do(ctx, http.MethodDelete, d.subPath(zone, k.Name, k.Type), nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}
