package search

import (
	"context"
	"strings"
	"sync"
)

const candCacheSize = 32

// candCache holds raw (pre-authorization) candidate lists per query for one
// index generation. Authorization still runs per request on the cached rows.
type candCache struct {
	mu      sync.Mutex
	gen     uint64
	entries map[string][]candidate
	order   []string
}

func candKey(q Query, kind Kind) string {
	return strings.Join(q.Text, "\x00") + "\x01" + q.App + "\x01" + q.Env + "\x01" + q.Tag + "\x01" + q.Node + "\x01" + string(kind)
}

func (c *candCache) get(gen uint64, key string) ([]candidate, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return nil, false
	}
	v, ok := c.entries[key]
	return v, ok
}

func (c *candCache) put(gen uint64, key string, v []candidate) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen || c.entries == nil {
		c.gen, c.entries, c.order = gen, map[string][]candidate{}, nil
	}
	if _, ok := c.entries[key]; !ok {
		if len(c.order) >= candCacheSize {
			delete(c.entries, c.order[0])
			c.order = c.order[1:]
		}
		c.order = append(c.order, key)
	}
	c.entries[key] = v
}

// candidates returns the strict match, falling back to the loose one when
// the strict match is empty. The generation is read before the query, so a
// reconcile that lands mid-query leaves the result under the old generation.
func (s *Service) candidates(ctx context.Context, q Query, kind Kind) ([]candidate, error) {
	gen := s.indexGen.Load()
	key := candKey(q, kind)
	if v, ok := s.cache.get(gen, key); ok {
		return v, nil
	}
	cands, err := fetchCandidates(ctx, s.db, q, kind, matchStrict)
	if err != nil {
		return nil, err
	}
	if len(cands) == 0 && len(q.Text) > 0 {
		if cands, err = fetchCandidates(ctx, s.db, q, kind, matchLoose); err != nil {
			return nil, err
		}
	}
	s.cache.put(gen, key, cands)
	return cands, nil
}
