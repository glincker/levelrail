package dnsrecords

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ZoneFinder resolves the zone a domain's records live in.
type ZoneFinder interface {
	// FindZone returns the zone with a trailing dot, or ErrZoneNotFound.
	FindZone(ctx context.Context, domain string) (string, error)
}

// ManagerZoneFinder probes candidate zones through a Manager: a zone exists
// when it lists and its apex SOA is at "@". It is the only option for
// Route53, which libdns cannot enumerate zones for.
type ManagerZoneFinder struct{ Mgr Manager }

// FindZone tries each candidate from longest to shortest.
func (f ManagerZoneFinder) FindZone(ctx context.Context, domain string) (string, error) {
	for _, c := range Candidates(domain) {
		recs, err := f.Mgr.GetRecords(ctx, c+".")
		if err != nil {
			continue
		}
		for _, rec := range recs {
			if rr := rec.RR(); rr.Type == "SOA" && rr.Name == "@" {
				return c + ".", nil
			}
		}
	}
	return "", ErrZoneNotFound
}

const cloudflareAPIBase = "https://api.cloudflare.com/client/v4"

const (
	cloudflareTimeout       = 15 * time.Second
	cloudflareMaxErrorBytes = 200
)

// CloudflareAPI is the few direct Cloudflare calls libdns cannot make: an
// exact zone lookup and the proxied flag. The token is only ever sent as a
// bearer header and never appears in an error.
type CloudflareAPI struct {
	Token   string
	BaseURL string
	HTTP    *http.Client
}

type cfEnvelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Result json.RawMessage `json:"result"`
}

func (c *CloudflareAPI) do(ctx context.Context, method, path string, body any, out any) error {
	base := c.BaseURL
	if base == "" {
		base = cloudflareAPIBase
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("dnsrecords: encode cloudflare request: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, rdr)
	if err != nil {
		return fmt.Errorf("dnsrecords: build cloudflare request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: cloudflareTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("dnsrecords: cloudflare request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return errors.New("dnsrecords: read cloudflare response")
	}
	var env cfEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("dnsrecords: cloudflare returned status %d", resp.StatusCode)
	}
	if !env.Success {
		msg := "request rejected"
		if len(env.Errors) > 0 {
			msg = env.Errors[0].Message
		}
		if len(msg) > cloudflareMaxErrorBytes {
			msg = msg[:cloudflareMaxErrorBytes]
		}
		return fmt.Errorf("dnsrecords: cloudflare: %s", msg)
	}
	if out != nil {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return errors.New("dnsrecords: decode cloudflare result")
		}
	}
	return nil
}

type cfZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *CloudflareAPI) zoneByName(ctx context.Context, name string) (cfZone, bool, error) {
	var zones []cfZone
	q := url.Values{"name": {name}}
	if err := c.do(ctx, http.MethodGet, "/zones?"+q.Encode(), nil, &zones); err != nil {
		return cfZone{}, false, err
	}
	for _, z := range zones {
		if strings.EqualFold(z.Name, name) {
			return z, true, nil
		}
	}
	return cfZone{}, false, nil
}

// FindZone asks Cloudflare for each candidate zone name, longest first.
func (c *CloudflareAPI) FindZone(ctx context.Context, domain string) (string, error) {
	for _, cand := range Candidates(domain) {
		z, ok, err := c.zoneByName(ctx, cand)
		if err != nil {
			return "", err
		}
		if ok {
			return z.Name + ".", nil
		}
	}
	return "", ErrZoneNotFound
}

// SetProxied sets the proxied flag on the records of recType at fqdn in zone
// whose content equals content, and returns how many it changed.
func (c *CloudflareAPI) SetProxied(ctx context.Context, zone, fqdn, recType, content string, proxied bool) (int, error) {
	z, ok, err := c.zoneByName(ctx, strings.TrimSuffix(zone, "."))
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ErrZoneNotFound
	}
	var recs []struct {
		ID      string `json:"id"`
		Content string `json:"content"`
		Proxied bool   `json:"proxied"`
	}
	q := url.Values{"type": {recType}, "name": {fqdn}}
	if err := c.do(ctx, http.MethodGet, "/zones/"+z.ID+"/dns_records?"+q.Encode(), nil, &recs); err != nil {
		return 0, err
	}
	changed := 0
	for _, r := range recs {
		if !sameData(r.Content, content, recType) || r.Proxied == proxied {
			continue
		}
		patch := map[string]bool{"proxied": proxied}
		if err := c.do(ctx, http.MethodPatch, "/zones/"+z.ID+"/dns_records/"+r.ID, patch, nil); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}
