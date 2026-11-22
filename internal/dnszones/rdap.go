package dnszones

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// IANABootstrapURL lists the RDAP base URLs of every TLD registry.
const IANABootstrapURL = "https://data.iana.org/rdap/dns.json"

// RegistrarHint is a best-effort registrar from public RDAP data.
type RegistrarHint struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// RDAPOptions bound RDAP lookups.
type RDAPOptions struct {
	Timeout  time.Duration
	CacheTTL time.Duration
	MaxBytes int64
}

// RDAPOptionsFromEnv reads APP_DNS_RDAP_TIMEOUT_MS, APP_DNS_RDAP_CACHE_SECONDS and APP_DNS_RDAP_MAX_BYTES.
func RDAPOptionsFromEnv() RDAPOptions {
	return RDAPOptions{
		Timeout:  time.Duration(envInt("APP_DNS_RDAP_TIMEOUT_MS", 3000)) * time.Millisecond,
		CacheTTL: time.Duration(envInt("APP_DNS_RDAP_CACHE_SECONDS", 3600)) * time.Second,
		MaxBytes: int64(envInt("APP_DNS_RDAP_MAX_BYTES", 1<<20)),
	}
}

type rdapItem struct {
	hint    RegistrarHint
	ok      bool
	expires time.Time
	at      time.Time
}

// RDAP looks up registrars through the IANA bootstrap. The client must be
// the netguard client so a registry URL can never reach a private address.
type RDAP struct {
	Client       *http.Client
	BootstrapURL string
	opts         RDAPOptions
	sf           singleflight.Group
	mu           sync.Mutex
	boot         map[string][]string
	bootAt       time.Time
	m            map[string]rdapItem
}

// NewRDAP returns a lookup over client.
func NewRDAP(client *http.Client, opts RDAPOptions) *RDAP {
	return &RDAP{Client: client, BootstrapURL: IANABootstrapURL, opts: opts, m: map[string]rdapItem{}}
}

// Registrar returns the domain's registrar, ok false on any failure.
func (r *RDAP) Registrar(ctx context.Context, domain string) (RegistrarHint, bool) {
	it, ok := r.item(ctx, domain)
	return it.hint, ok && it.ok
}

// Expiration returns the registrar expiration date, ok false when the lookup
// failed or the registry publishes no expiration event.
func (r *RDAP) Expiration(ctx context.Context, domain string) (time.Time, bool) {
	it, ok := r.item(ctx, domain)
	return it.expires, ok && !it.expires.IsZero()
}

func (r *RDAP) item(ctx context.Context, domain string) (rdapItem, bool) {
	domain = NormalizeDomain(domain)
	if !strings.Contains(domain, ".") {
		return rdapItem{}, false
	}
	r.mu.Lock()
	if it, ok := r.m[domain]; ok && time.Since(it.at) < r.opts.CacheTTL {
		r.mu.Unlock()
		return it, true
	}
	r.mu.Unlock()
	v, _, _ := r.sf.Do(domain, func() (any, error) {
		lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.opts.Timeout)
		defer cancel()
		it := r.lookup(lctx, domain)
		it.at = time.Now()
		r.mu.Lock()
		r.m[domain] = it
		r.mu.Unlock()
		return it, nil
	})
	return v.(rdapItem), true
}

func (r *RDAP) lookup(ctx context.Context, domain string) rdapItem {
	bases, err := r.bases(ctx, domain[strings.LastIndex(domain, ".")+1:])
	if err != nil || len(bases) == 0 {
		return rdapItem{}
	}
	base := strings.TrimSuffix(bases[0], "/")
	body, err := r.get(ctx, base+"/domain/"+url.PathEscape(domain))
	if err != nil {
		return rdapItem{}
	}
	var it rdapItem
	it.hint, it.ok = ParseRDAPRegistrar(body)
	it.expires, _ = ParseRDAPExpiration(body)
	return it
}

func (r *RDAP) bases(ctx context.Context, tld string) ([]string, error) {
	r.mu.Lock()
	if r.boot != nil && time.Since(r.bootAt) < r.opts.CacheTTL {
		b := r.boot[tld]
		r.mu.Unlock()
		return b, nil
	}
	r.mu.Unlock()
	body, err := r.get(ctx, r.BootstrapURL)
	if err != nil {
		return nil, err
	}
	boot, err := ParseRDAPBootstrap(body)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.boot, r.bootAt = boot, time.Now()
	r.mu.Unlock()
	return boot[tld], nil
}

func (r *RDAP) get(ctx context.Context, u string) ([]byte, error) {
	if !strings.HasPrefix(u, "https://") {
		return nil, errors.New("rdap: only https URLs are fetched")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("rdap: http " + resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, r.opts.MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > r.opts.MaxBytes {
		return nil, errors.New("rdap: response too large")
	}
	return body, nil
}

// ParseRDAPBootstrap maps each TLD to its registry's https RDAP base URLs.
func ParseRDAPBootstrap(body []byte) (map[string][]string, error) {
	var doc struct {
		Services [][][]string `json:"services"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, s := range doc.Services {
		if len(s) != 2 {
			continue
		}
		var urls []string
		for _, u := range s[1] {
			if strings.HasPrefix(u, "https://") {
				urls = append(urls, u)
			}
		}
		for _, tld := range s[0] {
			out[strings.ToLower(tld)] = urls
		}
	}
	return out, nil
}

type rdapEntity struct {
	Roles      []string          `json:"roles"`
	VCardArray []json.RawMessage `json:"vcardArray"`
}

// ParseRDAPRegistrar reads the registrar entity's vCard fn and https url.
func ParseRDAPRegistrar(body []byte) (RegistrarHint, bool) {
	var doc struct {
		Entities []rdapEntity `json:"entities"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return RegistrarHint{}, false
	}
	for _, e := range doc.Entities {
		isRegistrar := false
		for _, role := range e.Roles {
			isRegistrar = isRegistrar || role == "registrar"
		}
		if !isRegistrar || len(e.VCardArray) != 2 {
			continue
		}
		var props [][]json.RawMessage
		if err := json.Unmarshal(e.VCardArray[1], &props); err != nil {
			return RegistrarHint{}, false
		}
		var h RegistrarHint
		for _, p := range props {
			if len(p) < 4 {
				continue
			}
			var key, val string
			if json.Unmarshal(p[0], &key) != nil || json.Unmarshal(p[3], &val) != nil {
				continue
			}
			switch key {
			case "fn":
				h.Name = strings.TrimSpace(val)
			case "url":
				if u, err := url.Parse(val); err == nil && u.Scheme == "https" && u.Host != "" {
					h.URL = u.String()
				}
			}
		}
		if h.Name != "" && len(h.Name) <= 200 {
			return h, true
		}
	}
	return RegistrarHint{}, false
}

// ParseRDAPExpiration reads the registration expiration event date.
func ParseRDAPExpiration(body []byte) (time.Time, bool) {
	var doc struct {
		Events []struct {
			Action string `json:"eventAction"`
			Date   string `json:"eventDate"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return time.Time{}, false
	}
	for _, e := range doc.Events {
		if e.Action != "expiration" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, e.Date); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
