package alerting

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// BackupSizeSeries holds the sizes of one resource's succeeded backups, newest first.
type BackupSizeSeries struct {
	Resource string
	Sizes    []int64
}

// BackupSizeSource lists the recent succeeded backup sizes per database or volume.
type BackupSizeSource interface {
	RecentBackupSizes(ctx context.Context) ([]BackupSizeSeries, error)
}

// BackupSizePolicy is the live setting pair: Percent 0 turns the check off, Baseline is how many earlier backups form the comparison.
type BackupSizePolicy struct {
	Percent  int
	Baseline int
}

// EvaluateBackupSizeAnomaly runs one KindBackupSizeAnomaly rule. A backup that succeeded but is
// smaller than Percent of the median of the Baseline backups before it is likely empty or truncated.
func EvaluateBackupSizeAnomaly(ctx context.Context, source BackupSizeSource, policy BackupSizePolicy, r Rule, now time.Time) (Rule, string, error) {
	next := r
	next.LastEvaluatedAt = &now
	if policy.Percent <= 0 || policy.Baseline <= 0 {
		next.PendingSince, next.Firing, next.FiringSince = nil, false, nil
		return next, "", nil
	}
	series, err := source.RecentBackupSizes(ctx)
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: read backup sizes: %w", r.ID, err)
	}
	var bad []string
	for _, s := range series {
		if len(s.Sizes) <= policy.Baseline {
			continue
		}
		median := medianInt64(s.Sizes[1 : 1+policy.Baseline])
		if median <= 0 || s.Sizes[0]*100 >= median*int64(policy.Percent) {
			continue
		}
		bad = append(bad, fmt.Sprintf("%s: latest backup is %d bytes, usual size is about %d bytes", s.Resource, s.Sizes[0], median))
	}
	v := float64(len(bad))
	next.LastValue = &v
	var notice string
	if len(bad) > 0 {
		notice = "backup size anomaly: " + strings.Join(bad, "; ")
	}
	return advanceState(next, r, len(bad) > 0, 0, now), notice, nil
}

func medianInt64(in []int64) int64 {
	c := append([]int64(nil), in...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	return c[len(c)/2]
}
