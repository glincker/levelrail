package backup

import (
	"fmt"
	"sort"
	"time"
)

// RetentionPolicy keeps the newest backup of each of the last Daily days,
// Weekly ISO weeks and Monthly months that have one. A zero policy keeps
// everything (it expires nothing).
type RetentionPolicy struct {
	Daily   int
	Weekly  int
	Monthly int
}

// Active reports whether any bucket count is set.
func (p RetentionPolicy) Active() bool {
	return p.Daily > 0 || p.Weekly > 0 || p.Monthly > 0
}

// RetentionItem is one backup candidate for retention.
type RetentionItem struct {
	ID string
	At time.Time
}

// ExpiredByPolicy returns the IDs of items no bucket of p wants to keep. The
// newest item is always kept so a policy can never empty a history. Buckets
// count only periods that actually hold a backup, so a gap in backups does
// not shrink what is kept.
func ExpiredByPolicy(items []RetentionItem, p RetentionPolicy) []string {
	if !p.Active() || len(items) == 0 {
		return nil
	}
	sorted := append([]RetentionItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].At.After(sorted[j].At) })

	keep := map[string]bool{sorted[0].ID: true}
	bucket := func(limit int, key func(time.Time) string) {
		if limit <= 0 {
			return
		}
		seen := map[string]bool{}
		for _, it := range sorted {
			k := key(it.At.UTC())
			if seen[k] {
				continue
			}
			if len(seen) >= limit {
				return
			}
			seen[k] = true
			keep[it.ID] = true
		}
	}
	bucket(p.Daily, func(t time.Time) string { return t.Format("2006-01-02") })
	bucket(p.Weekly, func(t time.Time) string {
		y, w := t.ISOWeek()
		return fmt.Sprintf("%d-W%02d", y, w)
	})
	bucket(p.Monthly, func(t time.Time) string { return t.Format("2006-01") })

	var expired []string
	for _, it := range sorted {
		if !keep[it.ID] {
			expired = append(expired, it.ID)
		}
	}
	return expired
}
