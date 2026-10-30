package api

import (
	"context"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// releaseLogLines is diagnoseRecentLogs narrowed to the attempt's own
// release, so a failure excerpt never quotes an earlier container. Lines from before the attempt started never count.
func (rt *Router) releaseLogLines(ctx context.Context, a store.DeployAttempt) []string {
	if rt.telemetry == nil || rt.containers == nil {
		return rt.diagnoseRecentLogs(ctx, a.ServiceName)
	}
	now := time.Now()
	from, to := now.Add(-diagnosticLogWindow), now
	if !a.StartedAt.IsZero() {
		from = a.StartedAt.Add(-releaseLogSlack)
	}
	if a.FinishedAt != nil {
		to = a.FinishedAt.Add(releaseLogTail)
	}
	entries, err := rt.telemetry.QueryLogs(ctx, resourceIDForApp(a.ServiceName), from, to, "")
	if err != nil {
		return rt.diagnoseRecentLogs(ctx, a.ServiceName)
	}
	list, err := rt.containers.ListByPrefix(ctx, a.ServiceName+"-")
	if err != nil {
		return rt.diagnoseRecentLogs(ctx, a.ServiceName)
	}
	return attemptLogLines(entries, list, a)
}

const (
	releaseLogSlack = 5 * time.Second
	releaseLogTail  = time.Minute
)

// attemptLogLines keeps the lines written since the attempt started, from the
// attempt's own containers when they are still known. A container older than
// the attempt is an earlier release even when it shares the image or name.
func attemptLogLines(entries []telemetry.LogEntry, list []docker.ContainerState, a store.DeployAttempt) []string {
	key := application.ReleaseKey(a.ServiceName, application.ContainerName(a.ServiceName, application.NameImage(store.DesiredService{Name: a.ServiceName, Image: a.Image}), ""))
	var ids []string
	for _, c := range list {
		if !a.StartedAt.IsZero() && !c.Created.IsZero() && c.Created.Before(a.StartedAt.Add(-releaseLogSlack)) {
			continue
		}
		if c.Image == a.Image || (key != "" && application.ReleaseKey(a.ServiceName, c.Name) == key) {
			ids = append(ids, c.ID)
		}
	}
	var recent []telemetry.LogEntry
	for _, e := range entries {
		if a.StartedAt.IsZero() || !e.Timestamp.Before(a.StartedAt.Add(-releaseLogSlack)) {
			recent = append(recent, e)
		}
	}
	return filterLinesByContainer(recent, ids)
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
	if len(ids) == 0 {
		picked = all
	}
	if len(picked) > diagnosticLogLines {
		picked = picked[len(picked)-diagnosticLogLines:]
	}
	return picked
}
