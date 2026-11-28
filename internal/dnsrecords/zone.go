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
	"sort"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/dnszones"
)

// ZoneFinder resolves the zone a domain's records live in.
type ZoneFinder interface {
	// FindZone returns the zone with a trailing dot, or ErrZoneNotFound.
	FindZone(ctx context.Context, domain string) (string, error)
}

// maxListedZoneNames caps how many zone names an error lists.
const maxListedZoneNames = 5

// ZoneMissError is ErrZoneNotFound with the context an operator needs: the
// zone names the provider does list, a pinned zone that cannot hold the
// domain, or a private zone that matched. It carries names only.
type ZoneMissError struct {
	Domain string
	// Listed holds up to maxListedZoneNames public zone names, sorted.
	Listed []string
	// More counts the listed names left out of Listed.
	More int
	// Pinned is the pinned hosted zone name when it was the only zone visible.
	Pinned string
	// Private is a private zone that matched the domain and cannot be used.
	Private string
}

// Error implements error.
func (e *ZoneMissError) Error() string {
	switch {
	case e.Pinned != "":
		return fmt.Sprintf("the pinned hosted zone %q does not contain %q; clear the pinned zone or pick one that does", e.Pinned, e.Domain)
	case e.Private != "":
		return fmt.Sprintf("%q only matches the private hosted zone %q, which cannot publish public records", e.Domain, e.Private)
	case len(e.Listed) > 0:
		more := ""
		if e.More > 0 {
			more = fmt.Sprintf(" and %d more", e.More)
		}
		return fmt.Sprintf("no zone contains %q; the provider lists %s%s", e.Domain, strings.Join(e.Listed, ", "), more)
	}
	return fmt.Sprintf("no zone contains %q", e.Domain)
}

// Unwrap lets errors.Is(err, ErrZoneNotFound) keep working.
func (e *ZoneMissError) Unwrap() error { return ErrZoneNotFound }

// ManagerZoneFinder probes candidate zones through a Manager: a zone exists
// when it lists and its apex SOA is at "@". It is the only option for
// Route53, which libdns cannot enumerate zones for.
type ManagerZoneFinder struct {
	Mgr Manager
	// Debug, when set, receives each probe error that is otherwise skipped.
	Debug func(msg string, err error)
}

// FindZone tries each candidate from longest to shortest.
func (f ManagerZoneFinder) FindZone(ctx context.Context, domain string) (string, error) {
	for _, c := range Candidates(domain) {
		recs, err := f.Mgr.GetRecords(ctx, c+".")
		if err != nil {
			if f.Debug != nil {
				f.Debug("dns zone probe failed for "+c, err)
			}
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

// ListedZoneFinder matches the domain against the provider's own zone list
// first, which works when the credentials can list zones but a record probe
// cannot (a record the library fails to parse, or a pinned hosted zone). It
// falls back to Fallback when the list errors or holds no match. Private
// zones never match, so a public zone of the same name wins.
type ListedZoneFinder struct {
	P        dnszones.Provider
	Fallback ZoneFinder
	// Pinned is true when the operator pinned one hosted zone ID, so a lone
	// listed zone that misses is reported as the pinned zone.
	Pinned bool
	// Debug, when set, receives the list error that is otherwise swallowed.
	Debug func(msg string, err error)
}

// FindZone implements ZoneFinder.
func (f ListedZoneFinder) FindZone(ctx context.Context, domain string) (string, error) {
	zones, err := f.P.ListZones(ctx)
	var public, private []string
	if err != nil {
		if f.Debug != nil {
			f.Debug("dns zone list failed", err)
		}
	} else {
		for _, z := range zones {
			if z.Private {
				private = append(private, z.Name)
			} else {
				public = append(public, z.Name)
			}
		}
		if zone, ok := MatchZone(domain, public); ok {
			return zone, nil
		}
	}
	if f.Fallback != nil {
		zone, ferr := f.Fallback.FindZone(ctx, domain)
		if ferr == nil || !errors.Is(ferr, ErrZoneNotFound) {
			return zone, ferr
		}
	}
	if err != nil {
		return "", ErrZoneNotFound
	}
	miss := &ZoneMissError{Domain: NormalizeName(domain)}
	if p, ok := MatchZone(domain, private); ok {
		miss.Private = strings.TrimSuffix(p, ".")
	}
	if f.Pinned && len(zones) == 1 {
		miss.Pinned = NormalizeName(zones[0].Name)
	}
	miss.Listed, miss.More = ListedNames(public)
	return "", miss
}

// ListedNames returns up to maxListedZoneNames sorted unique names and how many were cut.
func ListedNames(names []string) ([]string, int) {
	seen := make(map[string]bool, len(names))
	var out []string
	for _, n := range names {
		if n = NormalizeName(n); n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	if len(out) > maxListedZoneNames {
		return out[:maxListedZoneNames], len(out) - maxListedZoneNames
	}
	return out, 0
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

type cfDNSRecord struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

// matchingRecords lists the records of recType at fqdn whose content equals
// content; records with other content are never returned.
func (c *CloudflareAPI) matchingRecords(ctx context.Context, zone, fqdn, recType, content string) (cfZone, []cfDNSRecord, error) {
	z, ok, err := c.zoneByName(ctx, strings.TrimSuffix(zone, "."))
	if err != nil {
		return cfZone{}, nil, err
	}
	if !ok {
		return cfZone{}, nil, ErrZoneNotFound
	}
	var recs []cfDNSRecord
	q := url.Values{"type": {recType}, "name": {fqdn}}
	if err := c.do(ctx, http.MethodGet, "/zones/"+z.ID+"/dns_records?"+q.Encode(), nil, &recs); err != nil {
		return cfZone{}, nil, err
	}
	var out []cfDNSRecord
	for _, r := range recs {
		if sameData(r.Content, content, recType) {
			out = append(out, r)
		}
	}
	return z, out, nil
}

// ProxyDrift counts the matching records and how many differ from proxied,
// without changing anything.
func (c *CloudflareAPI) ProxyDrift(ctx context.Context, zone, fqdn, recType, content string, proxied bool) (found, drifted int, err error) {
	_, recs, err := c.matchingRecords(ctx, zone, fqdn, recType, content)
	if err != nil {
		return 0, 0, err
	}
	for _, r := range recs {
		if r.Proxied != proxied {
			drifted++
		}
	}
	return len(recs), drifted, nil
}

// SetProxied sets the proxied flag on the records of recType at fqdn in zone
// whose content equals content, and returns how many it changed.
func (c *CloudflareAPI) SetProxied(ctx context.Context, zone, fqdn, recType, content string, proxied bool) (int, error) {
	z, recs, err := c.matchingRecords(ctx, zone, fqdn, recType, content)
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, r := range recs {
		if r.Proxied == proxied {
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
