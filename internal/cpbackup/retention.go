package cpbackup

import (
	"fmt"
	"sort"
	"time"
)

// Retention is how many backups to keep per calendar bucket. A backup is kept
// if it is the newest of any day, ISO week or month still inside its count.
type Retention struct {
	Daily, Weekly, Monthly int
}

// Entry is one complete backup for retention purposes.
type Entry struct {
	Key       string
	CreatedAt time.Time
}

// Select returns the keys to keep. It always keeps the newest entry, so a
// misconfigured policy can never delete the last good backup.
func (r Retention) Select(entries []Entry) map[string]bool {
	sorted := append([]Entry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].CreatedAt.After(sorted[j].CreatedAt) })
	keep := map[string]bool{}
	if len(sorted) > 0 {
		keep[sorted[0].Key] = true
	}
	tier := func(limit int, bucket func(time.Time) string) {
		seen := map[string]bool{}
		for _, e := range sorted {
			b := bucket(e.CreatedAt.UTC())
			if seen[b] {
				continue
			}
			if len(seen) >= limit {
				return
			}
			seen[b] = true
			keep[e.Key] = true
		}
	}
	tier(r.Daily, func(t time.Time) string { return t.Format("2006-01-02") })
	tier(r.Weekly, func(t time.Time) string {
		y, w := t.ISOWeek()
		return fmt.Sprintf("%d-W%02d", y, w)
	})
	tier(r.Monthly, func(t time.Time) string { return t.Format("2006-01") })
	return keep
}
