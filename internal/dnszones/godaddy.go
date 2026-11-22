package dnszones

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ProviderGoDaddy names the GoDaddy DNS provider.
const ProviderGoDaddy = "godaddy"

// GoDaddyBaseURL is the GoDaddy v1 API root.
const GoDaddyBaseURL = "https://api.godaddy.com/v1"

// goDaddyMinTTL is the lowest TTL the API accepts.
const goDaddyMinTTL = 600

// GoDaddy implements Provider over GoDaddy's domains API. It replaces one record set per call,
// so unrelated records are never touched.
type GoDaddy struct {
	rc restClient
}

// NewGoDaddy parses credential as "key:secret", sent only in the Authorization header.
func NewGoDaddy(credential string) (*GoDaddy, error) {
	key, secret, ok := strings.Cut(strings.TrimSpace(credential), ":")
	key, secret = strings.TrimSpace(key), strings.TrimSpace(secret)
	if !ok || key == "" || secret == "" {
		return nil, errors.New("godaddy: credential must be key:secret")
	}
	g := &GoDaddy{}
	g.rc = restClient{provider: ProviderGoDaddy, base: GoDaddyBaseURL, header: func(r *http.Request) {
		r.Header.Set("Authorization", "sso-key "+key+":"+secret)
	}}
	return g, nil
}

// SetBaseURL points the client at another API root (tests).
func (g *GoDaddy) SetBaseURL(base string, c *http.Client) { g.rc.base, g.rc.http = base, c }

// Name implements Provider.
func (g *GoDaddy) Name() string { return ProviderGoDaddy }

// Capabilities implements Provider.
func (g *GoDaddy) Capabilities() Capabilities { return Capabilities{} }

// ListZones implements Provider.
func (g *GoDaddy) ListZones(ctx context.Context) ([]Zone, error) {
	var ds []struct {
		Domain string `json:"domain"`
		Status string `json:"status"`
	}
	if err := g.rc.do(ctx, http.MethodGet, "/domains?statuses=ACTIVE", nil, &ds); err != nil {
		return nil, err
	}
	out := make([]Zone, 0, len(ds))
	for _, d := range ds {
		n := NormalizeDomain(d.Domain)
		out = append(out, Zone{ID: n, Name: n, Provider: ProviderGoDaddy, Status: d.Status})
	}
	return out, nil
}

// GetZone implements Provider.
func (g *GoDaddy) GetZone(ctx context.Context, id string) (Zone, error) {
	zones, err := g.ListZones(ctx)
	if err != nil {
		return Zone{}, err
	}
	return FindZone(zones, id)
}

// CreateZone implements Provider.
func (g *GoDaddy) CreateZone(context.Context, string) (Zone, error) {
	return Zone{}, errors.New("godaddy: register the domain at GoDaddy; zones cannot be created here")
}

// DeleteZone implements Provider.
func (g *GoDaddy) DeleteZone(context.Context, string) error {
	return errors.New("godaddy: a domain's DNS zone cannot be deleted through the API")
}

type gdRecord struct {
	Type     string `json:"type,omitempty"`
	Name     string `json:"name,omitempty"`
	Data     string `json:"data"`
	TTL      int    `json:"ttl"`
	Priority *int   `json:"priority,omitempty"`
}

func (g *GoDaddy) setPath(zone Zone, name, typ string) string {
	return "/domains/" + url.PathEscape(zone.Name) + "/records/" + url.PathEscape(typ) + "/" + url.PathEscape(name)
}

// ListRecordSets implements Provider.
func (g *GoDaddy) ListRecordSets(ctx context.Context, zone Zone) ([]RecordSet, error) {
	var recs []gdRecord
	if err := g.rc.do(ctx, http.MethodGet, "/domains/"+url.PathEscape(zone.Name)+"/records", nil, &recs); err != nil {
		return nil, err
	}
	var flat []flatRecord
	for _, r := range recs {
		if r.Type == "SOA" || r.Type == "NS" && r.Name == Apex {
			continue
		}
		v := r.Data
		if r.Type == "MX" && r.Priority != nil {
			v = strconv.Itoa(*r.Priority) + " " + r.Data
		}
		flat = append(flat, flatRecord{Name: r.Name, Type: r.Type, TTL: r.TTL, Value: v})
	}
	return groupFlat(flat), nil
}

// UpsertRecordSet implements Provider. The PUT replaces the whole set for the name and type.
func (g *GoDaddy) UpsertRecordSet(ctx context.Context, zone Zone, rs RecordSet) error {
	if rs.Type == "SRV" {
		return fmt.Errorf("godaddy: SRV records are %w", ErrUnsupported)
	}
	ttl := max(rs.TTL, goDaddyMinTTL)
	body := make([]gdRecord, 0, len(rs.Values))
	for _, v := range rs.Values {
		rec := gdRecord{Data: v, TTL: ttl}
		if rs.Type == "MX" {
			prio, host, ok := strings.Cut(strings.TrimSpace(v), " ")
			n, err := strconv.Atoi(prio)
			if !ok || err != nil {
				return fmt.Errorf("godaddy: MX value %q needs a priority and a host", v)
			}
			rec.Data, rec.Priority = strings.TrimSpace(host), &n
		}
		body = append(body, rec)
	}
	return g.rc.do(ctx, http.MethodPut, g.setPath(zone, rs.Name, rs.Type), body, nil)
}

// DeleteRecordSet implements Provider.
func (g *GoDaddy) DeleteRecordSet(ctx context.Context, zone Zone, k Key) error {
	err := g.rc.do(ctx, http.MethodDelete, g.setPath(zone, k.Name, k.Type), nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}
