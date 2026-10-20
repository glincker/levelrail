package preview

import (
	"sort"
	"time"
)

// Eviction reasons.
const (
	EvictKeepLimit = "keep_limit"
	EvictTTL       = "ttl"
	EvictBudget    = "budget"
)

// Eviction is one record PlanEviction wants deleted and why.
type Eviction struct {
	Record Record
	Reason string
}

// RetentionPolicy bounds how many previews are kept.
type RetentionPolicy struct {
	KeepPerApp    int
	TTL           time.Duration
	MaxTotalBytes int64
}

// PlanEviction picks the records to delete. Protected deployments (the
// current production release of each app) are never evicted and do not count
// against KeepPerApp. Order of rules: per-app keep limit, TTL, then the
// global byte budget by least recently used.
func PlanEviction(records []Record, protected map[string]bool, now time.Time, p RetentionPolicy) []Eviction {
	byApp := map[string][]Record{}
	for _, r := range records {
		byApp[r.App] = append(byApp[r.App], r)
	}
	evicted := map[string]string{}
	for _, recs := range byApp {
		sort.Slice(recs, func(i, j int) bool { return recs[i].CapturedAt.After(recs[j].CapturedAt) })
		kept := 0
		for _, r := range recs {
			if protected[r.DeploymentID] {
				continue
			}
			if p.TTL > 0 && now.Sub(r.CapturedAt) > p.TTL {
				evicted[r.DeploymentID] = EvictTTL
				continue
			}
			kept++
			if p.KeepPerApp > 0 && kept > p.KeepPerApp {
				evicted[r.DeploymentID] = EvictKeepLimit
			}
		}
	}

	var total int64
	var candidates []Record
	for _, r := range records {
		if _, gone := evicted[r.DeploymentID]; gone {
			continue
		}
		total += r.Bytes
		if !protected[r.DeploymentID] {
			candidates = append(candidates, r)
		}
	}
	if p.MaxTotalBytes > 0 && total > p.MaxTotalBytes {
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].LastUsed().Before(candidates[j].LastUsed()) })
		for _, r := range candidates {
			if total <= p.MaxTotalBytes {
				break
			}
			evicted[r.DeploymentID] = EvictBudget
			total -= r.Bytes
		}
	}

	var out []Eviction
	for _, r := range records {
		if reason, ok := evicted[r.DeploymentID]; ok {
			out = append(out, Eviction{Record: r, Reason: reason})
		}
	}
	return out
}
