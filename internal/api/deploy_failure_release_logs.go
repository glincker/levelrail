package api

import (
	"context"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// releaseLogLines is diagnoseRecentLogs narrowed to the attempt's own
// release, so a failure excerpt never quotes an earlier container. It falls
// back to the unfiltered lines when the release's containers are unknown.
func (rt *Router) releaseLogLines(ctx context.Context, a store.DeployAttempt) []string {
	if rt.telemetry == nil || rt.containers == nil {
		return rt.diagnoseRecentLogs(ctx, a.ServiceName)
	}
	now := time.Now()
	entries, err := rt.telemetry.QueryLogs(ctx, resourceIDForApp(a.ServiceName), now.Add(-diagnosticLogWindow), now, "")
	if err != nil {
		return rt.diagnoseRecentLogs(ctx, a.ServiceName)
	}
	list, err := rt.containers.ListByPrefix(ctx, a.ServiceName+"-")
	if err != nil {
		return rt.diagnoseRecentLogs(ctx, a.ServiceName)
	}
	key := application.ReleaseKey(a.ServiceName, application.ContainerName(a.ServiceName, application.NameImage(store.DesiredService{Name: a.ServiceName, Image: a.Image}), ""))
	var ids []string
	for _, c := range list {
		if c.Image == a.Image || (key != "" && application.ReleaseKey(a.ServiceName, c.Name) == key) {
			ids = append(ids, c.ID)
		}
	}
	return filterLinesByContainer(entries, ids)
}

func filterLinesByContainer(entries []telemetry.LogEntry, ids []string) []string {
	owned := func(id string) bool {
		for _, want := range ids {
			if id != "" && (strings.HasPrefix(want, id) || strings.HasPrefix(id, want)) {
				return true
			}
		}
		return false
	}
	var picked, all []string
	for _, e := range entries {
		all = append(all, e.Message)
		if owned(e.ContainerID) {
			picked = append(picked, e.Message)
		}
	}
	if len(ids) == 0 || len(picked) == 0 {
		picked = all
	}
	if len(picked) > diagnosticLogLines {
		picked = picked[len(picked)-diagnosticLogLines:]
	}
	return picked
}
