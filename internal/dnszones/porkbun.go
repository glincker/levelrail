package dnszones

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ProviderPorkbun names the Porkbun DNS provider.
const ProviderPorkbun = "porkbun"

// PorkbunBaseURL is the Porkbun v3 API root.
const PorkbunBaseURL = "https://api.porkbun.com/api/json/v3"

// Porkbun implements Provider over Porkbun's API. Its keys travel in every JSON body, not a header.
type Porkbun struct {
	rc                restClient
	apiKey, secretKey string
}

// NewPorkbun parses credential, either {"api_key","secret_key"} JSON or "apikey:secretapikey".
func NewPorkbun(credential string) (*Porkbun, error) {
	key, secret := "", ""
	if c := strings.TrimSpace(credential); strings.HasPrefix(c, "{") {
		var j struct {
			APIKey    string `json:"api_key"`
			SecretKey string `json:"secret_key"`
		}
		if err := json.Unmarshal([]byte(c), &j); err != nil {
			return nil, errors.New("porkbun: credential is not valid JSON")
		}
		key, secret = j.APIKey, j.SecretKey
	} else if k, s, ok := strings.Cut(c, ":"); ok {
		key, secret = k, s
	}
	key, secret = strings.TrimSpace(key), strings.TrimSpace(secret)
	if key == "" || secret == "" {
		return nil, errors.New("porkbun: credential needs an API key and a secret key")
	}
	return &Porkbun{rc: restClient{provider: ProviderPorkbun, base: PorkbunBaseURL}, apiKey: key, secretKey: secret}, nil
}

// SetBaseURL points the client at another API root (tests).
func (p *Porkbun) SetBaseURL(base string, c *http.Client) { p.rc.base, p.rc.http = base, c }

// Name implements Provider.
func (p *Porkbun) Name() string { return ProviderPorkbun }

// Capabilities implements Provider.
func (p *Porkbun) Capabilities() Capabilities { return Capabilities{ApexCNAME: true} }

// call posts the keys plus fields and turns a status other than SUCCESS into an error.
func (p *Porkbun) call(ctx context.Context, path string, fields map[string]any, out any) error {
	body := map[string]any{"apikey": p.apiKey, "secretapikey": p.secretKey}
	for k, v := range fields {
		body[k] = v
	}
	var resp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	raw := json.RawMessage{}
	if err := p.rc.do(ctx, http.MethodPost, path, body, &raw); err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &resp); err == nil && resp.Status != "" && resp.Status != "SUCCESS" {
		return &HTTPError{Provider: ProviderPorkbun, Status: http.StatusBadRequest, Message: resp.Message}
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// ListZones implements Provider.
func (p *Porkbun) ListZones(ctx context.Context) ([]Zone, error) {
	var out []Zone
	for start := 0; ; {
		var resp struct {
			Domains []struct {
				Domain string `json:"domain"`
				Status string `json:"status"`
			} `json:"domains"`
		}
		if err := p.call(ctx, "/domain/listAll", map[string]any{"start": strconv.Itoa(start)}, &resp); err != nil {
			return nil, err
		}
		for _, d := range resp.Domains {
			out = append(out, Zone{ID: NormalizeDomain(d.Domain), Name: NormalizeDomain(d.Domain), Provider: ProviderPorkbun, Status: d.Status})
		}
		if len(resp.Domains) == 0 {
			return out, nil
		}
		start += len(resp.Domains)
	}
}

// GetZone implements Provider.
func (p *Porkbun) GetZone(ctx context.Context, id string) (Zone, error) {
	zones, err := p.ListZones(ctx)
	if err != nil {
		return Zone{}, err
	}
	return FindZone(zones, id)
}

// CreateZone implements Provider. Domains are registered at Porkbun, not created through DNS.
func (p *Porkbun) CreateZone(context.Context, string) (Zone, error) {
	return Zone{}, errors.New("porkbun: register the domain at Porkbun; zones cannot be created here")
}

// DeleteZone implements Provider.
func (p *Porkbun) DeleteZone(context.Context, string) error {
	return errors.New("porkbun: a domain's DNS zone cannot be deleted through the API")
}

type pbRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     string `json:"ttl"`
}

func (p *Porkbun) list(ctx context.Context, zone Zone) ([]flatRecord, error) {
	var resp struct {
		Records []pbRecord `json:"records"`
	}
	if err := p.call(ctx, "/dns/retrieve/"+url.PathEscape(zone.Name), nil, &resp); err != nil {
		return nil, err
	}
	var out []flatRecord
	for _, r := range resp.Records {
		if r.Type == "SOA" {
			continue
		}
		ttl, _ := strconv.Atoi(r.TTL)
		out = append(out, flatRecord{ID: r.ID, Name: RelativeName(r.Name, zone.Name), Type: r.Type, TTL: ttl, Value: r.Content})
	}
	return out, nil
}

// pbFields sends the apex as an empty name, which is how the API addresses it.
func pbFields(rs RecordSet, value string) map[string]any {
	name := rs.Name
	if name == Apex {
		name = ""
	}
	return map[string]any{"name": name, "type": rs.Type, "content": value, "ttl": strconv.Itoa(rs.TTL)}
}

func (p *Porkbun) create(ctx context.Context, zone Zone, rs RecordSet, value string) error {
	return p.call(ctx, "/dns/create/"+url.PathEscape(zone.Name), pbFields(rs, value), nil)
}

func (p *Porkbun) update(ctx context.Context, zone Zone, id string, rs RecordSet, value string) error {
	return p.call(ctx, fmt.Sprintf("/dns/edit/%s/%s", url.PathEscape(zone.Name), url.PathEscape(id)), pbFields(rs, value), nil)
}

func (p *Porkbun) remove(ctx context.Context, zone Zone, id string) error {
	return p.call(ctx, fmt.Sprintf("/dns/delete/%s/%s", url.PathEscape(zone.Name), url.PathEscape(id)), nil, nil)
}

// ListRecordSets implements Provider.
func (p *Porkbun) ListRecordSets(ctx context.Context, zone Zone) ([]RecordSet, error) {
	recs, err := p.list(ctx, zone)
	if err != nil {
		return nil, err
	}
	return groupFlat(recs), nil
}

// UpsertRecordSet implements Provider.
func (p *Porkbun) UpsertRecordSet(ctx context.Context, zone Zone, rs RecordSet) error {
	return upsertFlat(ctx, p, zone, rs)
}

// DeleteRecordSet implements Provider.
func (p *Porkbun) DeleteRecordSet(ctx context.Context, zone Zone, k Key) error {
	return deleteFlat(ctx, p, zone, k)
}
