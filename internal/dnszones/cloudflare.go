package dnszones

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CloudflareBaseURL is Cloudflare's v4 API root.
const CloudflareBaseURL = "https://api.cloudflare.com/client/v4"

// Cloudflare implements Provider over Cloudflare's REST API with a scoped API token.
type Cloudflare struct {
	token     string
	BaseURL   string
	HTTP      *http.Client
	AccountID string
}

// NewCloudflare returns a client for token. The token is only ever sent as a bearer header.
func NewCloudflare(token string) *Cloudflare {
	return &Cloudflare{token: token, BaseURL: CloudflareBaseURL, HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// Name implements Provider.
func (c *Cloudflare) Name() string { return ProviderCloudflare }

// Capabilities implements Provider.
func (c *Cloudflare) Capabilities() Capabilities {
	return Capabilities{Proxied: true, ApexCNAME: true}
}

type cfEnvelope struct {
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	ResultInfo *struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

// CloudflareError is a non-success API response.
type CloudflareError struct {
	Status   int
	Messages []string
}

func (e *CloudflareError) Error() string {
	return fmt.Sprintf("cloudflare: http %d: %s", e.Status, strings.Join(e.Messages, "; "))
}

func (c *Cloudflare) call(ctx context.Context, method, path string, body, out any) (*cfEnvelope, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("cloudflare: encode body: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: %s %s: %w", method, strings.SplitN(path, "?", 2)[0], err)
	}
	defer func() { _ = resp.Body.Close() }()
	var env cfEnvelope
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&env); err != nil {
		return nil, fmt.Errorf("cloudflare: decode response (http %d): %w", resp.StatusCode, err)
	}
	if !env.Success || resp.StatusCode >= 300 {
		ce := &CloudflareError{Status: resp.StatusCode}
		for _, e := range env.Errors {
			ce.Messages = append(ce.Messages, fmt.Sprintf("%d %s", e.Code, e.Message))
		}
		return nil, ce
	}
	if out != nil && len(env.Result) > 0 {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return nil, fmt.Errorf("cloudflare: decode result: %w", err)
		}
	}
	return &env, nil
}

type cfZone struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	NameServers []string `json:"name_servers"`
	ModifiedOn  string   `json:"modified_on"`
	Account     struct {
		ID string `json:"id"`
	} `json:"account"`
}

func (z cfZone) toZone() Zone {
	return Zone{ID: z.ID, Name: NormalizeDomain(z.Name), Provider: ProviderCloudflare, Status: z.Status, NameServers: normalizeAll(z.NameServers), ModifiedAt: z.ModifiedOn}
}

// paginate fetches every page of path (which must already carry a query string).
func paginate[T any](ctx context.Context, c *Cloudflare, path string) ([]T, error) {
	var all []T
	for page := 1; ; page++ {
		var batch []T
		env, err := c.call(ctx, http.MethodGet, fmt.Sprintf("%s&page=%d", path, page), nil, &batch)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if env.ResultInfo == nil || page >= env.ResultInfo.TotalPages || len(batch) == 0 {
			return all, nil
		}
	}
}

func (c *Cloudflare) listRawZones(ctx context.Context) ([]cfZone, error) {
	return paginate[cfZone](ctx, c, "/zones?per_page=50")
}

// ListZones implements Provider (GET /zones).
func (c *Cloudflare) ListZones(ctx context.Context) ([]Zone, error) {
	raw, err := c.listRawZones(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Zone, 0, len(raw))
	for _, z := range raw {
		out = append(out, z.toZone())
	}
	return out, nil
}

// GetZone implements Provider (GET /zones/{id}).
func (c *Cloudflare) GetZone(ctx context.Context, id string) (Zone, error) {
	var z cfZone
	if _, err := c.call(ctx, http.MethodGet, "/zones/"+url.PathEscape(id), nil, &z); err != nil {
		return Zone{}, err
	}
	return z.toZone(), nil
}

func (c *Cloudflare) accountID(ctx context.Context) (string, error) {
	if c.AccountID != "" {
		return c.AccountID, nil
	}
	if zones, err := c.listRawZones(ctx); err == nil && len(zones) > 0 && zones[0].Account.ID != "" {
		return zones[0].Account.ID, nil
	}
	var accounts []struct {
		ID string `json:"id"`
	}
	if _, err := c.call(ctx, http.MethodGet, "/accounts?per_page=5", nil, &accounts); err != nil {
		return "", fmt.Errorf("cloudflare: find account for new zone: %w", err)
	}
	if len(accounts) != 1 {
		return "", fmt.Errorf("cloudflare: token sees %d accounts; pass account_id to choose one", len(accounts))
	}
	return accounts[0].ID, nil
}

// CreateZone implements Provider (POST /zones, type full).
func (c *Cloudflare) CreateZone(ctx context.Context, name string) (Zone, error) {
	acct, err := c.accountID(ctx)
	if err != nil {
		return Zone{}, err
	}
	body := map[string]any{"name": NormalizeDomain(name), "type": "full", "account": map[string]string{"id": acct}}
	var z cfZone
	if _, err := c.call(ctx, http.MethodPost, "/zones", body, &z); err != nil {
		return Zone{}, err
	}
	return z.toZone(), nil
}

// DeleteZone implements Provider (DELETE /zones/{id}).
func (c *Cloudflare) DeleteZone(ctx context.Context, id string) error {
	_, err := c.call(ctx, http.MethodDelete, "/zones/"+url.PathEscape(id), nil, nil)
	return err
}
