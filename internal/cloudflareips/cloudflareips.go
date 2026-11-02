// Package cloudflareips holds Cloudflare's published proxy ranges: an embedded
// snapshot that is always usable, refreshed from the official endpoint.
package cloudflareips

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"slices"
	"sync"
	"time"
)

// Endpoint is the only URL ever fetched; it is fixed so no setting can turn the refresh into an SSRF.
const Endpoint = "https://api.cloudflare.com/client/v4/ips"

// Source names where the active list came from.
const (
	SourceEmbedded = "embedded"
	SourceFetched  = "fetched"
)

// snapshot is the list shipped in the binary, used until a fetch succeeds.
var snapshot = []string{
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22", "141.101.64.0/18",
	"108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20", "197.234.240.0/22", "198.41.128.0/17",
	"162.158.0.0/15", "104.16.0.0/13", "104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32", "2405:8100::/32",
	"2a06:98c0::/29", "2c0f:f248::/32",
}

// Status is the read-only state shown in settings.
type Status struct {
	Source      string    `json:"source"`
	Ranges      int       `json:"ranges"`
	LastRefresh time.Time `json:"last_refresh,omitempty"`
	LastAttempt time.Time `json:"last_attempt,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
}

// List is a concurrency-safe set of Cloudflare ranges.
type List struct {
	mu       sync.RWMutex
	prefixes []netip.Prefix
	source   string
	lastOK   time.Time
	lastTry  time.Time
	lastErr  string
	// endpoint is a field only so tests can point at a local server.
	endpoint string
	client   *http.Client
}

// New returns a list seeded with the embedded snapshot.
func New() *List {
	l := &List{endpoint: Endpoint}
	p, _ := parseCIDRs(snapshot)
	l.prefixes, l.source = p, SourceEmbedded
	return l
}

var (
	defaultOnce sync.Once
	defaultList *List
)

// Default is the process-wide list shared by the ingress, Shield and the API.
func Default() *List {
	defaultOnce.Do(func() { defaultList = New() })
	return defaultList
}

func parseCIDRs(cidrs []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		p, err := netip.ParsePrefix(c)
		if err != nil {
			return nil, fmt.Errorf("cloudflareips: %q is not a CIDR", c)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

// Contains reports whether ip is inside the current ranges.
func (l *List) Contains(ip netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}
	ip = ip.Unmap()
	l.mu.RLock()
	defer l.mu.RUnlock()
	return slices.ContainsFunc(l.prefixes, func(p netip.Prefix) bool { return p.Contains(ip) })
}

// CIDRs returns the current ranges as strings.
func (l *List) CIDRs() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]string, len(l.prefixes))
	for i, p := range l.prefixes {
		out[i] = p.String()
	}
	return out
}

// Status reports the source, size and last refresh outcome.
func (l *List) Status() Status {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return Status{Source: l.source, Ranges: len(l.prefixes), LastRefresh: l.lastOK, LastAttempt: l.lastTry, LastError: l.lastErr}
}

type ipsResponse struct {
	Success bool `json:"success"`
	Result  struct {
		IPv4 []string `json:"ipv4_cidrs"`
		IPv6 []string `json:"ipv6_cidrs"`
	} `json:"result"`
}

// Parse validates an API body and returns its ranges; fewer than minRanges is
// rejected so a truncated answer can never shrink the trusted set.
func Parse(body []byte, minRanges int) ([]netip.Prefix, error) {
	var r ipsResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("cloudflareips: not valid JSON: %w", err)
	}
	if !r.Success {
		return nil, errors.New("cloudflareips: the endpoint reported failure")
	}
	all := append(slices.Clone(r.Result.IPv4), r.Result.IPv6...)
	if len(all) < minRanges {
		return nil, fmt.Errorf("cloudflareips: %d ranges is below the floor of %d", len(all), minRanges)
	}
	return parseCIDRs(all)
}

// Limits bound one refresh.
type Limits struct {
	Timeout   time.Duration
	MaxBytes  int64
	MinRanges int
}

// Refresh fetches the official list once and keeps the previous list on any
// failure. It reports whether the active ranges changed.
func (l *List) Refresh(ctx context.Context, lim Limits, now time.Time) (bool, error) {
	changed, err := l.fetch(ctx, lim)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lastTry = now
	if err != nil {
		l.lastErr = err.Error()
		return false, err
	}
	l.lastErr = ""
	return changed, nil
}

func (l *List) fetch(ctx context.Context, lim Limits) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, lim.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.endpoint, http.NoBody)
	if err != nil {
		return false, fmt.Errorf("cloudflareips: build request: %w", err)
	}
	client := l.client
	if client == nil {
		client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("cloudflareips: fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("cloudflareips: endpoint answered HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, lim.MaxBytes+1))
	if err != nil {
		return false, fmt.Errorf("cloudflareips: read: %w", err)
	}
	if int64(len(body)) > lim.MaxBytes {
		return false, fmt.Errorf("cloudflareips: answer is larger than %d bytes", lim.MaxBytes)
	}
	prefixes, err := Parse(body, lim.MinRanges)
	if err != nil {
		return false, err
	}
	l.mu.Lock()
	changed := !samePrefixes(l.prefixes, prefixes)
	l.prefixes, l.source, l.lastOK = prefixes, SourceFetched, time.Now()
	l.mu.Unlock()
	return changed, nil
}

func samePrefixes(a, b []netip.Prefix) bool {
	if len(a) != len(b) {
		return false
	}
	as, bs := slices.Clone(a), slices.Clone(b)
	cmp := func(x, y netip.Prefix) int { return x.Addr().Compare(y.Addr()) }
	slices.SortFunc(as, cmp)
	slices.SortFunc(bs, cmp)
	return slices.Equal(as, bs)
}

// Run refreshes at once, then on the interval returned by next (read again
// each round so a stored change applies live) until ctx ends. A failed attempt
// retries after a quarter of the interval so a boot-time network blip does not
// cost a whole day. onChange runs when the active ranges changed.
func (l *List) Run(ctx context.Context, limits func(context.Context) Limits, next func(context.Context) time.Duration, onChange func()) {
	var wait time.Duration
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		changed, err := l.Refresh(ctx, limits(ctx), time.Now())
		wait = next(ctx)
		if err != nil {
			wait /= 4
		} else if changed && onChange != nil {
			onChange()
		}
	}
}
