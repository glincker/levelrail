package dnszones

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ProviderDigitalOcean names the DigitalOcean DNS provider.
const ProviderDigitalOcean = "digitalocean"

// DigitalOceanBaseURL is the DigitalOcean v2 API root.
const DigitalOceanBaseURL = "https://api.digitalocean.com/v2"

// DigitalOcean implements Provider over DigitalOcean's domains API with a personal access token.
type DigitalOcean struct {
	rc restClient
}

// NewDigitalOcean returns a client for token, sent only as a bearer header.
func NewDigitalOcean(token string) *DigitalOcean {
	d := &DigitalOcean{}
	d.rc = restClient{provider: ProviderDigitalOcean, base: DigitalOceanBaseURL, header: func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
	}}
	return d
}

// SetBaseURL points the client at another API root (tests).
func (d *DigitalOcean) SetBaseURL(base string, c *http.Client) { d.rc.base, d.rc.http = base, c }

// Name implements Provider.
func (d *DigitalOcean) Name() string { return ProviderDigitalOcean }

// Capabilities implements Provider.
func (d *DigitalOcean) Capabilities() Capabilities { return Capabilities{ApexCNAME: false} }

type doDomain struct {
	Name string `json:"name"`
}

func (z doDomain) toZone() Zone {
	return Zone{ID: z.Name, Name: NormalizeDomain(z.Name), Provider: ProviderDigitalOcean, NameServers: []string{"ns1.digitalocean.com", "ns2.digitalocean.com", "ns3.digitalocean.com"}}
}

// ListZones implements Provider.
func (d *DigitalOcean) ListZones(ctx context.Context) ([]Zone, error) {
	var out []Zone
	for page := 1; ; page++ {
		var resp struct {
			Domains []doDomain `json:"domains"`
		}
		if err := d.rc.do(ctx, http.MethodGet, fmt.Sprintf("/domains?per_page=200&page=%d", page), nil, &resp); err != nil {
			return nil, err
		}
		for _, z := range resp.Domains {
			out = append(out, z.toZone())
		}
		if len(resp.Domains) < 200 {
			return out, nil
		}
	}
}

// GetZone implements Provider.
func (d *DigitalOcean) GetZone(ctx context.Context, id string) (Zone, error) {
	var resp struct {
		Domain doDomain `json:"domain"`
	}
	if err := d.rc.do(ctx, http.MethodGet, "/domains/"+url.PathEscape(id), nil, &resp); err != nil {
		if isNotFound(err) {
			return Zone{}, ErrZoneNotFound
		}
		return Zone{}, err
	}
	return resp.Domain.toZone(), nil
}

// CreateZone implements Provider.
func (d *DigitalOcean) CreateZone(ctx context.Context, name string) (Zone, error) {
	var resp struct {
		Domain doDomain `json:"domain"`
	}
	if err := d.rc.do(ctx, http.MethodPost, "/domains", map[string]string{"name": NormalizeDomain(name)}, &resp); err != nil {
		return Zone{}, err
	}
	return resp.Domain.toZone(), nil
}

// DeleteZone implements Provider.
func (d *DigitalOcean) DeleteZone(ctx context.Context, id string) error {
	return d.rc.do(ctx, http.MethodDelete, "/domains/"+url.PathEscape(id), nil, nil)
}

type doRecord struct {
	ID       int    `json:"id,omitempty"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Data     string `json:"data"`
	Priority *int   `json:"priority,omitempty"`
	Port     *int   `json:"port,omitempty"`
	Weight   *int   `json:"weight,omitempty"`
	TTL      int    `json:"ttl,omitempty"`
	Flags    *int   `json:"flags,omitempty"`
	Tag      string `json:"tag,omitempty"`
}

func (r doRecord) value() string {
	n := func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	}
	switch r.Type {
	case "MX":
		return fmt.Sprintf("%d %s", n(r.Priority), NormalizeDomain(r.Data))
	case "SRV":
		return fmt.Sprintf("%d %d %d %s", n(r.Priority), n(r.Weight), n(r.Port), NormalizeDomain(r.Data))
	case "CAA":
		return fmt.Sprintf("%d %s %s", n(r.Flags), r.Tag, strconv.Quote(r.Data))
	case "CNAME", "NS":
		return NormalizeDomain(r.Data)
	}
	return r.Data
}

func toDORecord(rs RecordSet, value string) (doRecord, error) {
	rec := doRecord{Type: rs.Type, Name: rs.Name, TTL: rs.TTL}
	f := strings.Fields(value)
	atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }
	switch rs.Type {
	case "MX":
		if len(f) != 2 {
			return rec, fmt.Errorf("digitalocean: bad MX value %q", value)
		}
		rec.Priority, rec.Data = intp(atoi(f[0])), absolute(f[1])
	case "SRV":
		if len(f) != 4 {
			return rec, fmt.Errorf("digitalocean: bad SRV value %q", value)
		}
		rec.Priority, rec.Weight, rec.Port, rec.Data = intp(atoi(f[0])), intp(atoi(f[1])), intp(atoi(f[2])), absolute(f[3])
	case "CAA":
		if len(f) < 3 {
			return rec, fmt.Errorf("digitalocean: bad CAA value %q", value)
		}
		v := strings.TrimSpace(strings.SplitN(value, f[1], 2)[1])
		if uq, err := strconv.Unquote(v); err == nil {
			v = uq
		}
		rec.Flags, rec.Tag, rec.Data = intp(atoi(f[0])), f[1], v
	case "CNAME", "NS":
		rec.Data = absolute(value)
	default:
		rec.Data = value
	}
	return rec, nil
}

// absolute adds the trailing dot DigitalOcean needs to treat a target as a full name.
func absolute(s string) string {
	if s == Apex || strings.HasSuffix(s, ".") {
		return s
	}
	return s + "."
}

func (d *DigitalOcean) list(ctx context.Context, zone Zone) ([]flatRecord, error) {
	var out []flatRecord
	for page := 1; ; page++ {
		var resp struct {
			Records []doRecord `json:"domain_records"`
		}
		path := fmt.Sprintf("/domains/%s/records?per_page=200&page=%d", url.PathEscape(zone.ID), page)
		if err := d.rc.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
			return nil, err
		}
		for _, r := range resp.Records {
			out = append(out, flatRecord{ID: strconv.Itoa(r.ID), Name: RelativeName(r.Name, zone.Name), Type: r.Type, TTL: r.TTL, Value: r.value()})
		}
		if len(resp.Records) < 200 {
			return out, nil
		}
	}
}

func (d *DigitalOcean) create(ctx context.Context, zone Zone, rs RecordSet, value string) error {
	body, err := toDORecord(rs, value)
	if err != nil {
		return err
	}
	return d.rc.do(ctx, http.MethodPost, "/domains/"+url.PathEscape(zone.ID)+"/records", body, nil)
}

func (d *DigitalOcean) update(ctx context.Context, zone Zone, id string, rs RecordSet, value string) error {
	body, err := toDORecord(rs, value)
	if err != nil {
		return err
	}
	return d.rc.do(ctx, http.MethodPut, "/domains/"+url.PathEscape(zone.ID)+"/records/"+url.PathEscape(id), body, nil)
}

func (d *DigitalOcean) remove(ctx context.Context, zone Zone, id string) error {
	return d.rc.do(ctx, http.MethodDelete, "/domains/"+url.PathEscape(zone.ID)+"/records/"+url.PathEscape(id), nil, nil)
}

// ListRecordSets implements Provider.
func (d *DigitalOcean) ListRecordSets(ctx context.Context, zone Zone) ([]RecordSet, error) {
	recs, err := d.list(ctx, zone)
	if err != nil {
		return nil, err
	}
	return groupFlat(recs), nil
}

// UpsertRecordSet implements Provider.
func (d *DigitalOcean) UpsertRecordSet(ctx context.Context, zone Zone, rs RecordSet) error {
	return upsertFlat(ctx, d, zone, rs)
}

// DeleteRecordSet implements Provider.
func (d *DigitalOcean) DeleteRecordSet(ctx context.Context, zone Zone, k Key) error {
	return deleteFlat(ctx, d, zone, k)
}
