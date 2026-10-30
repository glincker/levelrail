package dnsstatus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"sync"
	"time"
)

// Env knobs for public DNS checks and the result cache.
const (
	EnvLookupTimeout   = "APP_DNS_STATUS_LOOKUP_TIMEOUT"
	EnvProviderTimeout = "APP_DNS_STATUS_PROVIDER_TIMEOUT"
	EnvConcurrency     = "APP_DNS_STATUS_CONCURRENCY"
	EnvCacheSeconds    = "APP_DNS_STATUS_CACHE_SECONDS"
)

const (
	defaultLookupTimeout   = 3 * time.Second
	defaultProviderTimeout = 10 * time.Second
	defaultConcurrency     = 4
	defaultCacheSeconds    = 3
)

// Resolver is the slice of net.Resolver a public DNS check needs.
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupCNAME(ctx context.Context, name string) (string, error)
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
	LookupSRV(ctx context.Context, service, proto, name string) (string, []*net.SRV, error)
}

// Options bounds public DNS checks.
type Options struct {
	Timeout         time.Duration
	ProviderTimeout time.Duration
	Concurrency     int
}

func envDuration(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return def
}

// OptionsFromEnv reads the lookup timeouts and concurrency from env.
func OptionsFromEnv() Options {
	o := Options{Timeout: envDuration(EnvLookupTimeout, defaultLookupTimeout), ProviderTimeout: envDuration(EnvProviderTimeout, defaultProviderTimeout), Concurrency: defaultConcurrency}
	if n, err := strconv.Atoi(os.Getenv(EnvConcurrency)); err == nil && n > 0 {
		o.Concurrency = n
	}
	return o
}

// CacheTTL is how long a result is reused; 0 disables reuse.
func CacheTTL() time.Duration {
	if n, err := strconv.Atoi(os.Getenv(EnvCacheSeconds)); err == nil && n >= 0 {
		return time.Duration(n) * time.Second
	}
	return defaultCacheSeconds * time.Second
}

// unknownErr is true for timeouts and server failures, false for "no such record".
func unknownErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var de *net.DNSError
	if errors.As(err, &de) {
		return !de.IsNotFound
	}
	return false
}

func errFor(err error) error {
	if unknownErr(err) {
		return err
	}
	return nil
}

func lookup(ctx context.Context, r Resolver, w Want) ([]Found, error) {
	var out []Found
	add := func(t, v string) { out = append(out, Found{Type: t, Name: w.Name, Value: v}) }
	addHosts := func(hs []string, only string) {
		for _, h := range hs {
			a, err := netip.ParseAddr(h)
			if err != nil {
				continue
			}
			t := "AAAA"
			if a.Unmap().Is4() {
				t = "A"
			}
			if only == "" || t == only {
				add(t, a.Unmap().String())
			}
		}
	}
	cname := func() (string, error) {
		c, err := r.LookupCNAME(ctx, w.Name)
		if err != nil {
			return "", errFor(err)
		}
		if c = host(c); c != "" && c != w.Name {
			return c, nil
		}
		return "", nil
	}
	switch w.Type {
	case "TXT":
		vs, err := r.LookupTXT(ctx, w.Name)
		if err != nil {
			return nil, errFor(err)
		}
		for _, v := range vs {
			add("TXT", v)
		}
	case "MX":
		mx, err := r.LookupMX(ctx, w.Name)
		if err != nil {
			return nil, errFor(err)
		}
		for _, m := range mx {
			add("MX", fmt.Sprintf("%d %s", m.Pref, host(m.Host)))
		}
	case "A", "AAAA":
		hs, err := r.LookupHost(ctx, w.Name)
		if err != nil {
			return nil, errFor(err)
		}
		addHosts(hs, w.Type)
		if c, _ := cname(); c != "" {
			add("CNAME", c)
		}
	case "CNAME":
		c, err := cname()
		if err != nil {
			return nil, err
		}
		if c != "" {
			add("CNAME", c)
			break
		}
		hs, err := r.LookupHost(ctx, w.Name)
		if err != nil {
			return nil, errFor(err)
		}
		addHosts(hs, "")
	case "SRV":
		_, srvs, err := r.LookupSRV(ctx, "", "", w.Name)
		if err != nil {
			return nil, errFor(err)
		}
		for _, s := range srvs {
			add("SRV", fmt.Sprintf("%d %d %d %s", s.Priority, s.Weight, s.Port, host(s.Target)))
		}
	default:
		return nil, fmt.Errorf("%s records are not checked over public DNS", w.Type)
	}
	return out, nil
}

// FromResolver grades wants against public DNS, a bounded number of lookups at a time.
func FromResolver(ctx context.Context, r Resolver, wants []Want, o Options, now time.Time) Status {
	recs := make([]Record, len(wants))
	sem := make(chan struct{}, max(1, o.Concurrency))
	var wg sync.WaitGroup
	for i, w := range wants {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			lctx, cancel := context.WithTimeout(ctx, o.Timeout)
			defer cancel()
			at, err := lookup(lctx, r, w)
			if err != nil {
				recs[i] = base(w)
				recs[i].Status = Unknown
				recs[i].Reason = "The DNS lookup timed out or the server failed, so this record could not be checked. Check again."
				return
			}
			recs[i] = classify(w, at)
		}()
	}
	wg.Wait()
	return newStatus(SourcePublicDNS, recs, now)
}

// Key identifies one status request for the cache.
func Key(wants []Want, parts ...string) string {
	raw, _ := json.Marshal(struct {
		P []string
		W []Want
	}{parts, wants})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:16])
}

type cached struct {
	s  Status
	at time.Time
}

type call struct {
	done chan struct{}
	s    Status
}

// Cache reuses a result for a few seconds and coalesces identical concurrent
// requests, so a retry burst does one round of lookups. The zero value is ready.
type Cache struct {
	mu      sync.Mutex
	entries map[string]cached
	calls   map[string]*call
}

func cloneStatus(s Status) Status {
	s.Records = slices.Clone(s.Records)
	return s
}

// Do returns a result younger than ttl, waits for an identical call in flight, or runs fn.
func (c *Cache) Do(key string, ttl time.Duration, fn func() Status) Status {
	c.mu.Lock()
	if c.entries == nil {
		c.entries, c.calls = map[string]cached{}, map[string]*call{}
	}
	if e, ok := c.entries[key]; ok && ttl > 0 && time.Since(e.at) < ttl {
		c.mu.Unlock()
		return cloneStatus(e.s)
	}
	if cl, ok := c.calls[key]; ok {
		c.mu.Unlock()
		<-cl.done
		return cloneStatus(cl.s)
	}
	cl := &call{done: make(chan struct{})}
	c.calls[key] = cl
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.calls, key)
		for k, e := range c.entries {
			if time.Since(e.at) >= ttl {
				delete(c.entries, k)
			}
		}
		if ttl > 0 {
			c.entries[key] = cached{s: cl.s, at: time.Now()}
		}
		c.mu.Unlock()
		close(cl.done)
	}()
	cl.s = fn()
	return cloneStatus(cl.s)
}
