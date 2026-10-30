package api

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	appBucketRunning   = "running"
	appBucketDeploying = "deploying"
	appBucketFailing   = "failing"
	appBucketStopped   = "stopped"
)

// appStatusBucket mirrors the web list's statusBucket so server filters and client chips agree.
func appStatusBucket(sum appStatusSummary) string {
	switch {
	case sum.Variant == "success":
		return appBucketRunning
	case sum.Variant == "destructive":
		return appBucketFailing
	case sum.Label == "Stopped":
		return appBucketStopped
	default:
		return appBucketDeploying
	}
}

func (rt *Router) appStatusBuckets(ctx context.Context, svcs []store.DesiredService) (map[string]string, error) {
	controllers := make([]string, len(svcs))
	for i, s := range svcs {
		controllers[i] = applicationControllerName(s.Name)
	}
	conds, err := rt.deploys.GetConditionsForControllers(ctx, controllers)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(svcs))
	for _, s := range svcs {
		conditions := conds[applicationControllerName(s.Name)]
		if cond := rt.crossNodeIngressAppCondition(ctx, s); cond != nil {
			conditions = append(conditions, *cond)
		}
		out[s.Name] = appStatusBucket(summarizeAppConditions(conditions))
	}
	return out, nil
}

// restrictToStatus narrows canRead to apps in the given status bucket; the
// result is always reported as filtered because DB paging cannot see status.
func (rt *Router) restrictToStatus(ctx context.Context, fs appFilterStore, f store.AppListFilter, bucket string, canRead func(string) bool) (func(string) bool, bool, error) {
	f.Limit, f.Offset = 0, 0
	svcs, err := listAppHeads(ctx, fs, f)
	if err != nil {
		return nil, false, err
	}
	visible := svcs[:0:0]
	for _, s := range svcs {
		if canRead(s.Name) {
			visible = append(visible, s)
		}
	}
	buckets, err := rt.appStatusBuckets(ctx, visible)
	if err != nil {
		return nil, false, err
	}
	return func(name string) bool { return buckets[name] == bucket }, true, nil
}
