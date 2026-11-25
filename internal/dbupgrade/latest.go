package dbupgrade

import (
	"context"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// Where a resolved version came from.
const (
	LatestSourceRegistry = "registry"
	LatestSourceCache    = "cache"
	LatestSourceFallback = "fallback"
)

// LatestResolution is the concrete version chosen for an engine.
type LatestResolution struct {
	Engine     string
	Version    string
	Source     string
	ResolvedAt time.Time
	// SkippedNewer is a stable tag on a major the catalog does not list yet; it is never picked.
	SkippedNewer string
}

type latestEntry struct {
	res LatestResolution
}

// LatestResolver picks the newest stable tag per engine and caches it. The zero value is ready to use.
type LatestResolver struct {
	// Catalog defaults to the embedded one.
	Catalog *Catalog
	// Now is overridable for tests.
	Now func() time.Time

	once  sync.Once
	mu    sync.Mutex
	cache map[string]latestEntry
}

// A plain release tag has digits only: variants (-alpine, -windowsservercore) and
// pre-releases (rc, beta, nightly) all carry a suffix and are rejected by this shape.
var plainTag = regexp.MustCompile(`^(v?)(\d+)(?:\.(\d+))?(?:\.(\d+))?$`)

type tagParts struct {
	nums [3]int
	n    int
}

func parsePlainTag(tag string) (tagParts, bool) {
	m := plainTag.FindStringSubmatch(tag)
	if m == nil {
		return tagParts{}, false
	}
	var p tagParts
	for i := 0; i < 3; i++ {
		if m[i+2] == "" {
			break
		}
		v, err := strconv.Atoi(m[i+2])
		if err != nil {
			return tagParts{}, false
		}
		p.nums[i] = v
		p.n = i + 1
	}
	return p, true
}

func (p tagParts) cmp(o tagParts) int {
	for i := 0; i < 3; i++ {
		switch {
		case p.nums[i] < o.nums[i]:
			return -1
		case p.nums[i] > o.nums[i]:
			return 1
		}
	}
	return p.n - o.n
}

func (p tagParts) hasPrefix(prefix []int) bool {
	if p.n < len(prefix) {
		return false
	}
	for i, v := range prefix {
		if p.nums[i] != v {
			return false
		}
	}
	return true
}

// supportedPrefixes are the release lines a new database may start on: lines not past EOL,
// or the majors of the curated versions for engines that list no lines.
func supportedPrefixes(ec EngineCatalog, now time.Time) [][]int {
	var out [][]int
	for _, l := range ec.Lines {
		if eol, err := time.Parse("2006-01-02", l.EOL); err == nil && !eol.After(now) {
			continue
		}
		if p, ok := parsePlainTag(l.Line); ok {
			out = append(out, p.nums[:p.n])
		}
	}
	if len(out) > 0 || len(ec.Lines) > 0 {
		return out
	}
	seen := map[int]bool{}
	for _, v := range ec.Versions {
		if p, ok := parsePlainTag(v); ok && !seen[p.nums[0]] {
			seen[p.nums[0]] = true
			out = append(out, []int{p.nums[0]})
		}
	}
	return out
}

// PickLatest returns the newest plain tag on a supported line and, separately, a newer
// stable tag on an unknown major (reported, never chosen).
func PickLatest(tags []string, prefixes [][]int) (best, skippedNewer string) {
	var bestP, anyP tagParts
	var anyTag string
	for _, t := range tags {
		p, ok := parsePlainTag(t)
		if !ok {
			continue
		}
		if anyTag == "" || p.cmp(anyP) > 0 {
			anyTag, anyP = t, p
		}
		for _, pre := range prefixes {
			if p.hasPrefix(pre) && (best == "" || p.cmp(bestP) > 0) {
				best, bestP = t, p
				break
			}
		}
	}
	if best != "" && anyTag != "" && anyP.nums[0] > bestP.nums[0] {
		skippedNewer = anyTag
	}
	return best, skippedNewer
}

func (r *LatestResolver) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *LatestResolver) catalog() *Catalog {
	r.once.Do(func() {
		if r.Catalog == nil {
			r.Catalog, _ = LoadCatalog()
		}
	})
	return r.Catalog
}

// Resolve returns a concrete version for engine from repo's registry tags. Any failure (no lister,
// registry error, nothing on a supported line) yields fallback, so a create never fails on the network.
func (r *LatestResolver) Resolve(ctx context.Context, lister TagLister, engine, repo, fallback string, ttl time.Duration) LatestResolution {
	now := r.now()
	r.mu.Lock()
	if e, ok := r.cache[engine]; ok && ttl > 0 && now.Sub(e.res.ResolvedAt) < ttl {
		res := e.res
		r.mu.Unlock()
		res.Source = LatestSourceCache
		return res
	}
	r.mu.Unlock()

	fb := LatestResolution{Engine: engine, Version: fallback, Source: LatestSourceFallback, ResolvedAt: now}
	cat := r.catalog()
	if lister == nil || cat == nil || repo == "" {
		return fb
	}
	ec, ok := cat.Engine(engine)
	if !ok {
		return fb
	}
	prefixes := supportedPrefixes(ec, now)
	if len(prefixes) == 0 {
		return fb
	}
	tags, err := lister.ListTags(ctx, repo)
	if err != nil {
		return fb
	}
	best, skipped := PickLatest(tags, prefixes)
	if best == "" {
		return fb
	}
	res := LatestResolution{Engine: engine, Version: best, Source: LatestSourceRegistry, ResolvedAt: now, SkippedNewer: skipped}
	r.mu.Lock()
	if r.cache == nil {
		r.cache = map[string]latestEntry{}
	}
	r.cache[engine] = latestEntry{res: res}
	r.mu.Unlock()
	return res
}
