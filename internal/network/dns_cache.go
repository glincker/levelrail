package network

import (
	"hash/maphash"
	"math"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// cacheShards splits lock contention only; the byte budget is per cache, not per shard count.
const cacheShards = 8

// cacheKey includes view so answers filtered for one audience can never be served to another.
type cacheKey struct {
	name   string
	qtype  uint16
	qclass uint16
	do     bool
	view   string
}

type cacheEntry struct {
	key     cacheKey
	msg     *dns.Msg
	stored  time.Time
	expires time.Time
	size    int
	ref     bool
}

type cacheShard struct {
	mu   sync.Mutex
	m    map[cacheKey]*cacheEntry
	ring []*cacheEntry
	hand int
	used int
}

// dnsCache is a byte-budgeted answer cache with CLOCK eviction. Nothing is allocated until the first insert.
type dnsCache struct {
	budget func() int
	maxTTL func() time.Duration
	negTTL func() time.Duration
	now    func() time.Time
	seed   maphash.Seed
	shards [cacheShards]cacheShard
}

func newDNSCache(budget func() int, maxTTL, negTTL func() time.Duration) *dnsCache {
	return &dnsCache{budget: budget, maxTTL: maxTTL, negTTL: negTTL, now: time.Now, seed: maphash.MakeSeed()}
}

func (c *dnsCache) shard(k cacheKey) *cacheShard {
	var h maphash.Hash
	h.SetSeed(c.seed)
	_, _ = h.WriteString(k.name)
	_, _ = h.WriteString(k.view)
	return &c.shards[h.Sum64()%cacheShards]
}

// get returns a copy with TTLs reduced by the entry's age, or nil on a miss.
func (c *dnsCache) get(k cacheKey) *dns.Msg {
	if c.budget() <= 0 {
		return nil
	}
	s := c.shard(k)
	s.mu.Lock()
	e, ok := s.m[k]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	now := c.now()
	if !now.Before(e.expires) {
		s.removeLocked(e)
		s.mu.Unlock()
		return nil
	}
	e.ref = true
	out := e.msg.Copy()
	age := uint32(min(now.Sub(e.stored)/time.Second, math.MaxUint32)) //nolint:gosec // clamped to the uint32 range on this line
	s.mu.Unlock()
	for _, sec := range [][]dns.RR{out.Answer, out.Ns, out.Extra} {
		for _, rr := range sec {
			h := rr.Header()
			if h.Rrtype == dns.TypeOPT {
				continue
			}
			if h.Ttl > age {
				h.Ttl -= age
			} else {
				h.Ttl = 0
			}
		}
	}
	return out
}

// put stores resp when it is cacheable and reports whether it did.
func (c *dnsCache) put(k cacheKey, resp *dns.Msg) bool {
	budget := c.budget()
	if budget <= 0 {
		return false
	}
	ttl, ok := c.cacheTTL(resp)
	if !ok {
		return false
	}
	stored := resp.Copy()
	stored.Id = 0
	size := stored.Len()
	perShard := budget / cacheShards
	if size > perShard {
		return false
	}
	now := c.now()
	s := c.shard(k)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[cacheKey]*cacheEntry{}
	}
	if old, ok := s.m[k]; ok {
		s.removeLocked(old)
	}
	for s.used+size > perShard && len(s.ring) > 0 {
		s.evictLocked()
	}
	e := &cacheEntry{key: k, msg: stored, stored: now, expires: now.Add(ttl), size: size, ref: true}
	s.m[k] = e
	s.ring = append(s.ring, e)
	s.used += size
	return true
}

// cacheTTL is the clamped lifetime, or false when the response must not be cached.
func (c *dnsCache) cacheTTL(resp *dns.Msg) (time.Duration, bool) {
	if resp.Truncated || len(resp.Question) != 1 {
		return 0, false
	}
	switch resp.Rcode {
	case dns.RcodeSuccess:
		if len(resp.Answer) > 0 {
			return clampTTL(minTTL(resp.Answer), c.maxTTL()), true
		}
	case dns.RcodeNameError:
	default:
		// SERVFAIL, REFUSED and the rest are never remembered as answers.
		return 0, false
	}
	// Negative answer: only cacheable when an SOA says for how long.
	for _, rr := range resp.Ns {
		if soa, ok := rr.(*dns.SOA); ok {
			t := min(soa.Hdr.Ttl, soa.Minttl)
			return clampTTL(t, c.negTTL()), true
		}
	}
	return 0, false
}

func minTTL(rrs []dns.RR) uint32 {
	t := rrs[0].Header().Ttl
	for _, rr := range rrs[1:] {
		t = min(t, rr.Header().Ttl)
	}
	return t
}

func clampTTL(secs uint32, limit time.Duration) time.Duration {
	d := time.Duration(secs) * time.Second
	if d > limit {
		d = limit
	}
	return d
}

func (s *cacheShard) removeLocked(e *cacheEntry) {
	delete(s.m, e.key)
	for i, r := range s.ring {
		if r == e {
			last := len(s.ring) - 1
			s.ring[i] = s.ring[last]
			s.ring[last] = nil
			s.ring = s.ring[:last]
			if s.hand > len(s.ring) {
				s.hand = 0
			}
			break
		}
	}
	s.used -= e.size
}

// evictLocked advances the CLOCK hand, giving referenced entries one more round.
func (s *cacheShard) evictLocked() {
	for {
		if s.hand >= len(s.ring) {
			s.hand = 0
		}
		e := s.ring[s.hand]
		if e.ref {
			e.ref = false
			s.hand++
			continue
		}
		s.removeLocked(e)
		return
	}
}
