package proxyroutes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// RouterTLS is what Traefik reports about one router's TLS block.
type RouterTLS struct {
	Found        bool
	HasTLS       bool
	CertResolver string
	Domains      []string
}

type apiRouter struct {
	Name string `json:"name"`
	TLS  *struct {
		CertResolver string `json:"certResolver"`
		Domains      []struct {
			Main string   `json:"main"`
			Sans []string `json:"sans"`
		} `json:"domains"`
	} `json:"tls"`
}

func (r apiRouter) info() RouterTLS {
	out := RouterTLS{Found: true}
	if r.TLS == nil {
		return out
	}
	out.HasTLS, out.CertResolver = true, r.TLS.CertResolver
	for _, d := range r.TLS.Domains {
		out.Domains = append(out.Domains, d.Main)
		out.Domains = append(out.Domains, d.Sans...)
	}
	return out
}

func traefikGet(ctx context.Context, base, path string, timeout time.Duration, into any) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(base, "/")+path, nil)
	if err != nil {
		return 0, err
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req) //nolint:gosec // base is a fixed loopback address
	if err != nil {
		return 0, fmt.Errorf("traefik api: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(into); err != nil {
		return resp.StatusCode, fmt.Errorf("traefik api: %w", err)
	}
	return resp.StatusCode, nil
}

// RouterTLSInfo reads one router's TLS block from Traefik's API.
func RouterTLSInfo(ctx context.Context, base, router string, timeout time.Duration) (RouterTLS, error) {
	var r apiRouter
	code, err := traefikGet(ctx, base, "/api/http/routers/"+url.PathEscape(router), timeout, &r)
	if err != nil {
		return RouterTLS{}, err
	}
	switch code {
	case http.StatusOK:
		return r.info(), nil
	case http.StatusNotFound:
		return RouterTLS{}, nil
	}
	return RouterTLS{}, fmt.Errorf("traefik api answered %d", code)
}

// KnownResolvers lists the certResolver names other routers on this Traefik
// use, most used first, so a hint can name a resolver that exists.
func KnownResolvers(ctx context.Context, base string, timeout time.Duration) ([]string, error) {
	var all []apiRouter
	code, err := traefikGet(ctx, base, "/api/http/routers", timeout, &all)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("traefik api answered %d", code)
	}
	counts := map[string]int{}
	for _, r := range all {
		if r.TLS != nil && r.TLS.CertResolver != "" {
			counts[r.TLS.CertResolver]++
		}
	}
	names := make([]string, 0, len(counts))
	for n := range counts {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	return names, nil
}
