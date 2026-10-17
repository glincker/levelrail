package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/autoscale"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	defaultAutoscaleHistory = 50
	maxAutoscaleHistory     = 500
	noAutoscalePolicy       = "no autoscale policy is set for this app"
)

type autoscaleHistoryResource struct {
	Events []autoscale.Event `json:"events"`
}

func (rt *Router) autoscaleController() *autoscale.Controller {
	st, ok := rt.apps.(autoscale.Store)
	if !ok {
		return nil
	}
	c := &autoscale.Controller{
		Store: st, Signals: autoscaleSignals{rt: rt}, Logger: rt.logger,
		Nudge:         rt.nudgeReconciler,
		Ceiling:       autoscale.CeilingFromEnv(os.Getenv),
		FlapThreshold: autoscale.FlapThresholdFromEnv(os.Getenv),
	}
	if rec, ok := rt.telemetry.(autoscaleGaugeSink); ok {
		c.Recorder = gaugeRecorder{rec}
	}
	return c
}

type autoscaleGaugeSink interface {
	RecordAutoscale(ctx context.Context, samples []telemetry.AutoscaleSample, at time.Time) error
}

type gaugeRecorder struct{ sink autoscaleGaugeSink }

func (g gaugeRecorder) RecordGauges(ctx context.Context, gauges []autoscale.Gauge, at time.Time) error {
	samples := make([]telemetry.AutoscaleSample, len(gauges))
	for i, v := range gauges {
		samples[i] = telemetry.AutoscaleSample{App: v.App, Current: v.Current, Desired: v.Desired, NoCapacity: v.NoCapacity, Flaps: v.Flaps}
	}
	return g.sink.RecordAutoscale(ctx, samples, at)
}

// AutoscaleTick is the scheduler entry point for the autoscale controller.
func (rt *Router) AutoscaleTick(ctx context.Context) {
	c := rt.autoscaleController()
	if c == nil {
		return
	}
	if err := c.Tick(ctx); err != nil {
		rt.logger.Warn("autoscale: reconcile failed", slog.String("error", err.Error()))
	}
}

// keepAutoscaledReplicas stops a manual edit from taking the replica count
// away from an autoscaled app. A request that leaves replicas unset keeps the
// current count; one that changes it is refused with 409.
func (rt *Router) keepAutoscaledReplicas(w http.ResponseWriter, r *http.Request, name string, existing *store.DesiredService, req *appResource) bool {
	st, ok := rt.apps.(autoscale.Store)
	if !ok {
		return true
	}
	if _, err := st.GetAutoscalePolicy(r.Context(), name); errors.Is(err, autoscale.ErrNotFound) {
		return true
	} else if err != nil {
		rt.internalError(w, "api: update app: read autoscale policy", err, slog.String("name", name))
		return false
	}
	current := max(existing.Replicas, 1)
	if req.Replicas == 0 {
		req.Replicas = current
		return true
	}
	if req.Replicas != current {
		writeError(w, http.StatusConflict, "replicas is managed by autoscale; change the autoscale policy, or clear it to set replicas by hand")
		return false
	}
	return true
}

func (rt *Router) handleGetAutoscale(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c := rt.autoscaleController()
	if c == nil {
		writeError(w, http.StatusNotImplemented, "autoscaling is not available")
		return
	}
	res, err := c.Status(r.Context(), name)
	if errors.Is(err, autoscale.ErrNotFound) {
		writeError(w, http.StatusNotFound, noAutoscalePolicy)
		return
	}
	if err != nil {
		rt.internalError(w, "api: get autoscale", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handlePutAutoscale(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c := rt.autoscaleController()
	if c == nil {
		writeError(w, http.StatusNotImplemented, "autoscaling is not available")
		return
	}
	var req autoscale.Policy
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.App = name
	pol := req.Normalize()
	if err := pol.Validate(c.Ceiling); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set autoscale: load app", err, slog.String("name", name))
		return
	}
	now := time.Now()
	ev := autoscale.Event{
		App: name, Kind: autoscale.EventConfigured, At: now,
		Detail: fmt.Sprintf("%s set %d to %d replicas on %s target %g", rt.eventActor(r), pol.MinReplicas, pol.MaxReplicas, pol.Metric, pol.Target),
	}
	if err := c.Store.SaveAutoscalePolicy(r.Context(), pol, ev, now); err != nil {
		rt.internalError(w, "api: set autoscale", err, slog.String("name", name))
		return
	}
	if err := c.Reconcile(r.Context(), pol, now); err != nil {
		rt.logger.Warn("autoscale: reconcile after save failed", slog.String("name", name), slog.String("error", err.Error()))
	}
	res, err := c.Status(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: set autoscale: reload", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handleDeleteAutoscale(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c := rt.autoscaleController()
	if c == nil {
		writeError(w, http.StatusNotImplemented, "autoscaling is not available")
		return
	}
	ev := autoscale.Event{App: name, Kind: autoscale.EventCleared, At: time.Now(), Detail: rt.eventActor(r) + " removed the policy; replicas stay where they are"}
	err := c.Store.DeleteAutoscalePolicy(r.Context(), name, ev)
	if errors.Is(err, autoscale.ErrNotFound) {
		writeError(w, http.StatusNotFound, noAutoscalePolicy)
		return
	}
	if err != nil {
		rt.internalError(w, "api: clear autoscale", err, slog.String("name", name))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) handleAutoscaleHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c := rt.autoscaleController()
	if c == nil {
		writeError(w, http.StatusNotImplemented, "autoscaling is not available")
		return
	}
	limit := defaultAutoscaleHistory
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxAutoscaleHistory {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("limit must be between 1 and %d", maxAutoscaleHistory))
			return
		}
		limit = n
	}
	events, err := c.Store.ListAutoscaleEvents(r.Context(), name, limit)
	if err != nil {
		rt.internalError(w, "api: autoscale history", err, slog.String("name", name))
		return
	}
	if events == nil {
		events = []autoscale.Event{}
	}
	writeJSON(w, http.StatusOK, autoscaleHistoryResource{Events: events})
}
