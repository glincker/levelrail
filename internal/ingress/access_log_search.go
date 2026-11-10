package ingress

import (
	"container/heap"
	"sort"
	"strings"
	"time"
)

// AccessCursor marks the last row of a page; the next page starts strictly older.
type AccessCursor struct {
	Time      time.Time
	RequestID string
}

// AccessSearch is a bounded, filtered read of the in-memory access log.
// All matching is plain string comparison in process; nothing reaches SQL.
type AccessSearch struct {
	From, To   time.Time
	StatusMin  int
	StatusMax  int
	Method     string
	Host       string
	PathPrefix string
	Text       string
	MinMillis  float64
	Limit      int
	MaxScan    int
	Before     *AccessCursor
}

// AccessSearchResult is one page, newest first.
type AccessSearchResult struct {
	Records   []AccessRecord
	Keys      []string
	Next      *AccessCursor
	Truncated bool
	Scanned   int
}

// newerFirst is the page order: time then request id, both descending.
func newerFirst(a, b AccessRecord) bool {
	if a.Time.Equal(b.Time) {
		return a.RequestID > b.RequestID
	}
	return a.Time.After(b.Time)
}

// olderThan reports whether rec sorts strictly after the cursor row.
func olderThan(rec AccessRecord, c AccessCursor) bool {
	return newerFirst(AccessRecord{Time: c.Time, RequestID: c.RequestID}, rec)
}

func (q AccessSearch) match(rec AccessRecord, ret time.Duration, now time.Time, since time.Time) bool {
	if ret > 0 && now.Sub(rec.Time) > ret {
		return false
	}
	if rec.Time.Before(since) || (!q.To.IsZero() && rec.Time.After(q.To)) {
		return false
	}
	if rec.Status < q.StatusMin || (q.StatusMax > 0 && rec.Status > q.StatusMax) || rec.Millis < q.MinMillis {
		return false
	}
	if q.Method != "" && !strings.EqualFold(rec.Method, q.Method) {
		return false
	}
	if q.Host != "" && !strings.EqualFold(rec.Host, q.Host) {
		return false
	}
	if q.PathPrefix != "" && !strings.HasPrefix(rec.Path, q.PathPrefix) {
		return false
	}
	return q.Text == "" || strings.Contains(strings.ToLower(rec.Path), strings.ToLower(q.Text))
}

type ringCursor struct {
	key  string
	recs []AccessRecord
	i    int
}

type ringHeap []*ringCursor

func (h ringHeap) Len() int           { return len(h) }
func (h ringHeap) Less(i, j int) bool { return newerFirst(h[i].recs[h[i].i], h[j].recs[h[j].i]) }
func (h ringHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *ringHeap) Push(x any)        { *h = append(*h, x.(*ringCursor)) }
func (h *ringHeap) Pop() any {
	old := *h
	c := old[len(old)-1]
	*h = old[:len(old)-1]
	return c
}

// Search merges the rings for keys newest first and stops at Limit matches or
// MaxScan examined rows, so its cost and memory are bounded however wide the
// filter is. Rows newer than the cursor are skipped by binary search and do not
// count against MaxScan. Because the merge is global, a truncated page still
// carries a cursor: every row at or above it was examined. Keys in Result are
// index for index with Records. since holds per key owner start times; older rows are hidden.
func (l *AccessLog) Search(keys []string, q AccessSearch, since map[string]time.Time) AccessSearchResult {
	var res AccessSearchResult
	if l == nil || q.Limit <= 0 {
		return res
	}
	ret := l.retentionNow()
	now := time.Now()
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	h := make(ringHeap, 0, len(sorted))
	for _, key := range sorted {
		l.mu.RLock()
		r := l.rings[key]
		l.mu.RUnlock()
		if r == nil {
			continue
		}
		recs := r.snapshot()
		sort.Slice(recs, func(i, j int) bool { return newerFirst(recs[i], recs[j]) })
		start := 0
		if q.Before != nil {
			c := *q.Before
			start = sort.Search(len(recs), func(i int) bool { return olderThan(recs[i], c) })
		}
		if start < len(recs) {
			h = append(h, &ringCursor{key: key, recs: recs, i: start})
		}
	}
	heap.Init(&h)
	var last *AccessRecord
	for h.Len() > 0 {
		c := h[0]
		rec := c.recs[c.i]
		if rec.Time.Before(q.From) {
			heap.Pop(&h)
			continue
		}
		if q.MaxScan > 0 && res.Scanned >= q.MaxScan {
			res.Truncated = true
			if last != nil {
				res.Next = &AccessCursor{Time: last.Time, RequestID: last.RequestID}
			}
			break
		}
		res.Scanned++
		last = &c.recs[c.i]
		if q.match(rec, ret, now, maxTime(q.From, since[c.key])) {
			if len(res.Records) == q.Limit {
				prev := res.Records[len(res.Records)-1]
				res.Next = &AccessCursor{Time: prev.Time, RequestID: prev.RequestID}
				break
			}
			res.Records = append(res.Records, rec)
			res.Keys = append(res.Keys, c.key)
		}
		c.i++
		if c.i == len(c.recs) {
			heap.Pop(&h)
		} else {
			heap.Fix(&h, 0)
		}
	}
	return res
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// Purge drops the ring for key, so a host that changed hands or left the
// routing table keeps no rows and the ring map cannot grow without bound.
func (l *AccessLog) Purge(key string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	delete(l.rings, key)
	l.mu.Unlock()
}

// OwnerSince returns when each route key got its current owner; keys owned
// since the first publication are absent and so not filtered.
func (s *RequestStats) OwnerSince() map[string]time.Time {
	p := s.ownerSince.Load()
	if p == nil {
		return nil
	}
	return *p
}

// OwnedKeys returns a copy of the route key to app map used to scope reads.
func (s *RequestStats) OwnedKeys() map[string]string {
	p := s.owners.Load()
	if p == nil {
		return nil
	}
	out := make(map[string]string, len(*p))
	for k, v := range *p {
		out[k] = v
	}
	return out
}
