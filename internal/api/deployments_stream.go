package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	deploymentStreamHeartbeat = 20 * time.Second
	deploymentFetchAttempts   = 10
	deploymentFetchDelay      = 200 * time.Millisecond
)

// deploymentEvent is one SSE message on GET /api/v1/deployments/stream.
// Type is created, step or finished; Step is set only for step events.
type deploymentEvent struct {
	Type       string             `json:"type"`
	Step       *deploymentStepMsg `json:"step,omitempty"`
	Deployment deploymentResource `json:"deployment"`
}

type deploymentStepMsg struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func deploymentsStreamResource(*http.Request) string { return "deployments" }

func deploymentEventType(kind string) string {
	switch kind {
	case deploylog.StateStarted:
		return "created"
	case deploylog.StateFinished:
		return "finished"
	}
	return kind
}

// fetchDeployment loads one deployment with its derived fields. The
// recorder announces an attempt before its row is saved and finishes it
// before the terminal status is persisted, so it retries briefly until the
// row exists and, for a finished event, has left the building state.
func (rt *Router) fetchDeployment(ctx context.Context, ds deploymentStore, id string, wantSettled bool) (store.Deployment, bool) {
	for i := 0; i < deploymentFetchAttempts; i++ {
		rows, _, err := ds.ListDeployments(ctx, store.DeploymentFilter{ID: id, Limit: 1})
		if err != nil {
			rt.logger.Warn("api: deployment stream: load deployment failed", slog.String("deploy_id", id), slog.String("error", err.Error()))
			return store.Deployment{}, false
		}
		if len(rows) == 1 && (!wantSettled || rows[0].Status != store.DeploymentBuilding) {
			return rows[0], true
		}
		select {
		case <-ctx.Done():
			return store.Deployment{}, false
		case <-time.After(deploymentFetchDelay):
		}
	}
	return store.Deployment{}, false
}

// handleDeploymentsStream handles GET /api/v1/deployments/stream: SSE of
// deployment lifecycle changes for the apps the caller can read.
func (rt *Router) handleDeploymentsStream(w http.ResponseWriter, r *http.Request) {
	ds, ok := rt.deployAttempts.(deploymentStore)
	if !ok || rt.deployRecorder == nil {
		writeError(w, http.StatusNotImplemented, "live deployment streaming is not configured on this control plane")
		return
	}
	canRead, _, err := rt.callerAppVisibility(r)
	if err != nil {
		rt.internalError(w, "api: deployment stream: resolve caller", err)
		return
	}
	events, unsubscribe := rt.deployRecorder.SubscribeAll()
	defer unsubscribe()

	flusher, ok := startSSE(w)
	if !ok {
		return
	}
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(deploymentStreamHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev := <-events:
			if fresh, _, err := rt.callerAppVisibility(r); err == nil {
				canRead = fresh
			}
			d, found := rt.fetchDeployment(r.Context(), ds, ev.AttemptID, ev.Kind == deploylog.StateFinished)
			if !found || !canRead(d.Attempt.ServiceName) {
				continue
			}
			out := deploymentEvent{Type: deploymentEventType(ev.Kind), Deployment: rt.toDeploymentResource(d)}
			applyDeploymentWait(&out.Deployment, rt.deploymentWaits(r.Context(), []store.Deployment{d})[d.Attempt.ID])
			one := []deploymentResource{out.Deployment}
			rt.attachPreviewURLs(r.Context(), one)
			out.Deployment = one[0]
			if ev.Kind == deploylog.StateStep {
				out.Step = &deploymentStepMsg{Name: ev.Step, Status: ev.Status}
			}
			data, err := json.Marshal(out)
			if err != nil {
				rt.logger.Error("api: marshal deployment event failed", slog.String("error", err.Error()))
				continue
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
