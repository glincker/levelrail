package dnszones

import (
	"context"
	"os"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/sync/singleflight"
)

// Delegated values: whether public DNS sends the domain to the zone's assigned name servers.
const (
	DelegatedYes     = "yes"
	DelegatedNo      = "no"
	DelegatedPartial = "partial"
	DelegatedUnknown = "unknown"
)

// Reasons attached to a DelegatedUnknown result.
const (
	DelegationReasonPrivateZone   = "private_zone"
	DelegationReasonLookupFailed  = "lookup_failed"
	DelegationReasonNoAssignedNS  = "no_assigned_nameservers"
	DelegationReasonNoPublicNSSet = "no_public_nameservers"
)

// NSDelegation is the delegation verdict for one zone.
type NSDelegation struct {
	Delegated string   `json:"delegated"`
	Reason    string   `json:"reason,omitempty"`
	Expected  []string `json:"expected_nameservers"`
	Current   []string `json:"current_nameservers"`
}

// ClassifyDelegation compares the provider's assigned name servers with the
// public NS set. Every public server assigned is yes, none is no, a mix is partial.
func ClassifyDelegation(private bool, expected, current []string, lookupErr error) NSDelegation {
	d := NSDelegation{Expected: normalizeAll(expected), Current: normalizeAll(current)}
	switch {
	case private:
		d.Delegated, d.Reason, d.Current = DelegatedUnknown, DelegationReasonPrivateZone, []string{}
		return d
	case len(d.Expected) == 0:
		d.Delegated, d.Reason = DelegatedUnknown, DelegationReasonNoAssignedNS
		return d
	case lookupErr != nil:
		d.Delegated, d.Reason = DelegatedUnknown, DelegationReasonLookupFailed
		return d
	case len(d.Current) == 0:
		d.Delegated, d.Reason = DelegatedUnknown, DelegationReasonNoPublicNSSet
		return d
	}
	in := 0
	for _, ns := range d.Current {
		if slices.Contains(d.Expected, ns) {
			in++
		}
	}
	switch in {
	case len(d.Current):
		d.Delegated = DelegatedYes
	case 0:
		d.Delegated = DelegatedNo
	default:
		d.Delegated = DelegatedPartial
	}
	return d
}

// NSCheckerOptions bound the public NS lookups of an NSChecker.
type NSCheckerOptions struct {
	Servers     []string
	Timeout     time.Duration
	CacheTTL    time.Duration
	Concurrency int
}

// NSCheckerOptionsFromEnv reads APP_DNS_DELEGATION_TIMEOUT_MS, _CACHE_SECONDS and _CONCURRENCY.
func NSCheckerOptionsFromEnv(servers []string) NSCheckerOptions {
	return NSCheckerOptions{
		Servers:     servers,
		Timeout:     time.Duration(envInt("APP_DNS_DELEGATION_TIMEOUT_MS", 2000)) * time.Millisecond,
		CacheTTL:    time.Duration(envInt("APP_DNS_DELEGATION_CACHE_SECONDS", 60)) * time.Second,
		Concurrency: envInt("APP_DNS_DELEGATION_CONCURRENCY", 4),
	}
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

type nsCacheItem struct {
	ns  []string
	err error
	at  time.Time
}

// NSChecker resolves a zone apex's public NS set with a short cache, one
// in-flight lookup per domain and a cap on concurrent lookups. It only sends
// DNS queries to the configured resolvers, never to the discovered hosts.
type NSChecker struct {
	q    Querier
	opts NSCheckerOptions
	sem  chan struct{}
	sf   singleflight.Group
	mu   sync.Mutex
	m    map[string]nsCacheItem
}

// NewNSChecker returns a checker over q.
func NewNSChecker(q Querier, opts NSCheckerOptions) *NSChecker {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 1
	}
	if len(opts.Servers) == 0 {
		opts.Servers = DefaultCheckServers
	}
	return &NSChecker{q: q, opts: opts, sem: make(chan struct{}, opts.Concurrency), m: map[string]nsCacheItem{}}
}

// errNoAnswer marks a lookup where no resolver answered.
type errNoAnswer string

func (e errNoAnswer) Error() string { return string(e) }

// PublicNS returns the domain's NS set from the first resolver that answers.
func (c *NSChecker) PublicNS(ctx context.Context, domain string) ([]string, error) {
	domain = NormalizeDomain(domain)
	c.mu.Lock()
	if it, ok := c.m[domain]; ok && time.Since(it.at) < c.opts.CacheTTL {
		c.mu.Unlock()
		return it.ns, it.err
	}
	c.mu.Unlock()
	v, _, _ := c.sf.Do(domain, func() (any, error) {
		// Detached so a coalesced lookup does not die with its first caller.
		lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.opts.Timeout)
		defer cancel()
		it := nsCacheItem{at: time.Now()}
		select {
		case c.sem <- struct{}{}:
			defer func() { <-c.sem }()
			it.ns, it.err = c.lookup(lctx, domain)
		case <-lctx.Done():
			it.err = lctx.Err()
		}
		c.mu.Lock()
		c.m[domain] = it
		c.mu.Unlock()
		return it, nil
	})
	it := v.(nsCacheItem)
	return it.ns, it.err
}

func (c *NSChecker) lookup(ctx context.Context, domain string) ([]string, error) {
	last := "no resolver answered"
	for _, s := range c.opts.Servers {
		a := c.q.Query(ctx, withPort(s), domain, dns.TypeNS)
		if a.Error == "" && len(a.Values) > 0 {
			return normalizeAll(a.Values), nil
		}
		if a.Error != "" {
			last = a.Error
		}
		if ctx.Err() != nil {
			break
		}
	}
	return nil, errNoAnswer(last)
}

// Check classifies z's delegation. Private zones are never looked up.
func (c *NSChecker) Check(ctx context.Context, z Zone) NSDelegation {
	if z.Private {
		return ClassifyDelegation(true, z.NameServers, nil, nil)
	}
	ns, err := c.PublicNS(ctx, z.Name)
	return ClassifyDelegation(false, z.NameServers, ns, err)
}
