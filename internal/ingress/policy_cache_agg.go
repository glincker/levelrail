package ingress

// MergeCacheStats sums several domains' stats into one view. Series align
// by minute because every domain reports the same trailing window.
func MergeCacheStats(list ...CacheStats) CacheStats {
	var out CacheStats
	reasons := make([][]CacheReason, 0, len(list))
	for _, s := range list {
		reasons = append(reasons, s.Reasons)
		out.Hits += s.Hits
		out.Misses += s.Misses
		out.Bypasses += s.Bypasses
		out.Stale += s.Stale
		out.HitBytes += s.HitBytes
		out.OriginBytes += s.OriginBytes
		out.Entries += s.Entries
		out.Bytes += s.Bytes
		if out.Series == nil {
			out.Series = make([]CacheBucket, len(s.Series))
			for i, b := range s.Series {
				out.Series[i].Minute = b.Minute
			}
		}
		for i, b := range s.Series {
			if i >= len(out.Series) {
				break
			}
			out.Series[i].Hits += b.Hits
			out.Series[i].Misses += b.Misses
			out.Series[i].HitBytes += b.HitBytes
			out.Series[i].MissBytes += b.MissBytes
		}
	}
	out.Reasons = mergeReasons(reasons...)
	return out
}

// HitRate is the share of cacheable requests answered from the cache
// (fresh or stale hits over hits plus misses); bypasses are not counted.
func (s CacheStats) HitRate() float64 {
	served := s.Hits + s.Stale
	if total := served + s.Misses; total > 0 {
		return float64(served) / float64(total)
	}
	return 0
}

// Tail returns the totals over the last minutes of the series.
func (s CacheStats) Tail(minutes int) (hits, misses, hitBytes, missBytes uint64) {
	from := max(0, len(s.Series)-minutes)
	for _, b := range s.Series[from:] {
		hits += b.Hits
		misses += b.Misses
		hitBytes += b.HitBytes
		missBytes += b.MissBytes
	}
	return hits, misses, hitBytes, missBytes
}
