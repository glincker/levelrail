package dnszones

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// ProviderHetzner names the Hetzner DNS provider.
const ProviderHetzner = "hetzner"

// HetznerBaseURL is the Hetzner DNS API root.
const HetznerBaseURL = "https://dns.hetzner.com/api/v1"

// Hetzner implements Provider over Hetzner's DNS API with an API token.
type Hetzner struct {
	rc restClient
}

// NewHetzner returns a client for token, sent only in the Auth-API-Token header.
func NewHetzner(token string) *Hetzner {
	h := &Hetzner{}
	h.rc = restClient{provider: ProviderHetzner, base: HetznerBaseURL, header: func(r *http.Request) {
		r.Header.Set("Auth-API-Token", token)
	}}
	return h
}

// SetBaseURL points the client at another API root (tests).
func (h *Hetzner) SetBaseURL(base string, c *http.Client) { h.rc.base, h.rc.http = base, c }

// Name implements Provider.
func (h *Hetzner) Name() string { return ProviderHetzner }

// Capabilities implements Provider.
func (h *Hetzner) Capabilities() Capabilities { return Capabilities{} }

type hzZone struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	NS       []string `json:"ns"`
	Status   string   `json:"status"`
	Records  int      `json:"records_count"`
	Modified string   `json:"modified"`
}

func (z hzZone) toZone() Zone {
	n := z.Records
	return Zone{ID: z.ID, Name: NormalizeDomain(z.Name), Provider: ProviderHetzner, Status: z.Status, RecordCount: &n, NameServers: normalizeAll(z.NS), ModifiedAt: z.Modified}
}

// ListZones implements Provider.
func (h *Hetzner) ListZones(ctx context.Context) ([]Zone, error) {
	var out []Zone
	for page := 1; ; page++ {
		var resp struct {
			Zones []hzZone `json:"zones"`
			Meta  struct {
				Pagination struct {
					LastPage int `json:"last_page"`
				} `json:"pagination"`
			} `json:"meta"`
		}
		if err := h.rc.do(ctx, http.MethodGet, fmt.Sprintf("/zones?per_page=100&page=%d", page), nil, &resp); err != nil {
			return nil, err
		}
		for _, z := range resp.Zones {
			out = append(out, z.toZone())
		}
		if len(resp.Zones) == 0 || page >= resp.Meta.Pagination.LastPage {
			return out, nil
		}
	}
}

// GetZone implements Provider.
func (h *Hetzner) GetZone(ctx context.Context, id string) (Zone, error) {
	var resp struct {
		Zone hzZone `json:"zone"`
	}
	if err := h.rc.do(ctx, http.MethodGet, "/zones/"+url.PathEscape(id), nil, &resp); err != nil {
		if isNotFound(err) {
			return Zone{}, ErrZoneNotFound
		}
		return Zone{}, err
	}
	return resp.Zone.toZone(), nil
}

// CreateZone implements Provider.
func (h *Hetzner) CreateZone(ctx context.Context, name string) (Zone, error) {
	var resp struct {
		Zone hzZone `json:"zone"`
	}
	if err := h.rc.do(ctx, http.MethodPost, "/zones", map[string]any{"name": NormalizeDomain(name), "ttl": 3600}, &resp); err != nil {
		return Zone{}, err
	}
	return resp.Zone.toZone(), nil
}

// DeleteZone implements Provider.
func (h *Hetzner) DeleteZone(ctx context.Context, id string) error {
	return h.rc.do(ctx, http.MethodDelete, "/zones/"+url.PathEscape(id), nil, nil)
}

type hzRecord struct {
	ID     string `json:"id,omitempty"`
	ZoneID string `json:"zone_id,omitempty"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Value  string `json:"value"`
	TTL    int    `json:"ttl,omitempty"`
}

func (r hzRecord) value() string {
	if r.Type == "TXT" {
		return unquoteTXT(r.Value)
	}
	return r.Value
}

func hzBody(zone Zone, rs RecordSet, value string) hzRecord {
	v := value
	if rs.Type == "TXT" {
		v = quoteTXT(value)
	}
	return hzRecord{ZoneID: zone.ID, Type: rs.Type, Name: rs.Name, Value: v, TTL: rs.TTL}
}

func (h *Hetzner) list(ctx context.Context, zone Zone) ([]flatRecord, error) {
	var out []flatRecord
	for page := 1; ; page++ {
		var resp struct {
			Records []hzRecord `json:"records"`
			Meta    struct {
				Pagination struct {
					LastPage int `json:"last_page"`
				} `json:"pagination"`
			} `json:"meta"`
		}
		path := fmt.Sprintf("/records?zone_id=%s&per_page=100&page=%d", url.QueryEscape(zone.ID), page)
		if err := h.rc.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
			return nil, err
		}
		for _, r := range resp.Records {
			out = append(out, flatRecord{ID: r.ID, Name: RelativeName(r.Name, zone.Name), Type: r.Type, TTL: r.TTL, Value: r.value()})
		}
		if len(resp.Records) == 0 || page >= resp.Meta.Pagination.LastPage {
			return out, nil
		}
	}
}

func (h *Hetzner) create(ctx context.Context, zone Zone, rs RecordSet, value string) error {
	return h.rc.do(ctx, http.MethodPost, "/records", hzBody(zone, rs, value), nil)
}

func (h *Hetzner) update(ctx context.Context, zone Zone, id string, rs RecordSet, value string) error {
	return h.rc.do(ctx, http.MethodPut, "/records/"+url.PathEscape(id), hzBody(zone, rs, value), nil)
}

func (h *Hetzner) remove(ctx context.Context, _ Zone, id string) error {
	return h.rc.do(ctx, http.MethodDelete, "/records/"+url.PathEscape(id), nil, nil)
}

// ListRecordSets implements Provider.
func (h *Hetzner) ListRecordSets(ctx context.Context, zone Zone) ([]RecordSet, error) {
	recs, err := h.list(ctx, zone)
	if err != nil {
		return nil, err
	}
	return groupFlat(recs), nil
}

// UpsertRecordSet implements Provider.
func (h *Hetzner) UpsertRecordSet(ctx context.Context, zone Zone, rs RecordSet) error {
	return upsertFlat(ctx, h, zone, rs)
}

// DeleteRecordSet implements Provider.
func (h *Hetzner) DeleteRecordSet(ctx context.Context, zone Zone, k Key) error {
	return deleteFlat(ctx, h, zone, k)
}
