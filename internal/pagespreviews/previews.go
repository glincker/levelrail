// Package pagespreviews groups and prunes the preview sites of a Pages site.
package pagespreviews

import (
	"sort"
	"time"
)

// Entry is one preview site: a branch published at a commit.
type Entry struct {
	Branch     string
	PRNumber   int
	PreviewApp string
	HeadSHA    string
	Status     string
	Domain     string
	UpdatedAt  time.Time
}

func newestFirst(es []Entry) []Entry {
	out := append([]Entry(nil), es...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

// LatestPerBranch keeps the most recently updated preview of each branch, newest first.
func LatestPerBranch(es []Entry) []Entry {
	seen := map[string]bool{}
	var out []Entry
	for _, e := range newestFirst(es) {
		if seen[e.Branch] {
			continue
		}
		seen[e.Branch] = true
		out = append(out, e)
	}
	return out
}

// Prune returns the oldest entries beyond limit, never including keep. limit <= 0 prunes nothing.
func Prune(es []Entry, limit int, keep string) []Entry {
	if limit <= 0 {
		return nil
	}
	var victims []Entry
	// The spared preview holds a slot even when it is not the newest.
	kept := 0
	for _, e := range es {
		if e.PreviewApp == keep {
			kept = 1
		}
	}
	for _, e := range newestFirst(es) {
		if e.PreviewApp == keep {
			continue
		}
		if kept >= limit {
			victims = append(victims, e)
			continue
		}
		kept++
	}
	return victims
}
