package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// nodeThresholdStore is the slice of *store.DB the override endpoints need; a store without it answers 501.
type nodeThresholdStore interface {
	GetNodeThresholdOverride(ctx context.Context, nodeID string) (store.NodeThresholdOverride, error)
	SetNodeThresholdOverride(ctx context.Context, o store.NodeThresholdOverride) error
}

type nodeThresholdValues struct {
	CPUPercent    *int `json:"cpu_percent"`
	MemoryPercent *int `json:"memory_percent"`
	DiskPercent   *int `json:"disk_percent"`
}

type nodeThresholdsResource struct {
	NodeID             string              `json:"node_id"`
	Defaults           nodeThresholdValues `json:"defaults"`
	Override           nodeThresholdValues `json:"override"`
	Effective          nodeThresholdValues `json:"effective"`
	SustainedIntervals int                 `json:"sustained_intervals"`
}

func (rt *Router) nodeThresholdStore(w http.ResponseWriter) (nodeThresholdStore, bool) {
	st, ok := rt.nodes.(nodeThresholdStore)
	if !ok || rt.runtimeSettings == nil {
		writeError(w, http.StatusNotImplemented, "node thresholds are not configured on this control plane")
		return nil, false
	}
	return st, true
}

// loadThresholdNode answers the same 404 for a missing node and for one the caller cannot see.
func (rt *Router) loadThresholdNode(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return "", false
	} else if err != nil {
		rt.internalError(w, "api: node thresholds: load node failed", err, slog.String("node_id", id))
		return "", false
	}
	return id, true
}

func (rt *Router) nodeThresholdsView(ctx context.Context, o store.NodeThresholdOverride) nodeThresholdsResource {
	rs := rt.runtimeSettings
	def := func(key string) *int { n := rs.Int(ctx, key); return &n }
	pick := func(override, fallback *int) *int {
		if override != nil {
			return override
		}
		return fallback
	}
	d := nodeThresholdValues{
		CPUPercent:    def(runtimesettings.KeyNodeCPUThresholdPercent),
		MemoryPercent: def(runtimesettings.KeyNodeMemoryThresholdPercent),
		DiskPercent:   def(runtimesettings.KeyNodeDiskThresholdPercent),
	}
	ov := nodeThresholdValues{CPUPercent: o.CPUPercent, MemoryPercent: o.MemoryPercent, DiskPercent: o.DiskPercent}
	return nodeThresholdsResource{
		NodeID: o.NodeID, Defaults: d, Override: ov,
		Effective: nodeThresholdValues{
			CPUPercent: pick(ov.CPUPercent, d.CPUPercent), MemoryPercent: pick(ov.MemoryPercent, d.MemoryPercent), DiskPercent: pick(ov.DiskPercent, d.DiskPercent),
		},
		SustainedIntervals: rs.Int(ctx, runtimesettings.KeyNodeThresholdSustained),
	}
}

// handleGetNodeThresholds handles GET /api/v1/nodes/{id}/thresholds.
func (rt *Router) handleGetNodeThresholds(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.nodeThresholdStore(w)
	if !ok {
		return
	}
	id, ok := rt.loadThresholdNode(w, r)
	if !ok {
		return
	}
	o, err := st.GetNodeThresholdOverride(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: get node thresholds failed", err, slog.String("node_id", id))
		return
	}
	writeJSON(w, http.StatusOK, rt.nodeThresholdsView(r.Context(), o))
}

func validThresholdPercent(name string, v *int) error {
	if v != nil && (*v < runtimesettings.NodeThresholdMinPercent || *v > runtimesettings.NodeThresholdMaxPercent) {
		return fmt.Errorf("%s must be between %d and %d, or null for the default", name, runtimesettings.NodeThresholdMinPercent, runtimesettings.NodeThresholdMaxPercent)
	}
	return nil
}

// handleSetNodeThresholds handles PUT /api/v1/nodes/{id}/thresholds; the body replaces the override and null fields use the default.
func (rt *Router) handleSetNodeThresholds(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.nodeThresholdStore(w)
	if !ok {
		return
	}
	id, ok := rt.loadThresholdNode(w, r)
	if !ok {
		return
	}
	var body nodeThresholdValues
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	for _, c := range []struct {
		name string
		v    *int
	}{{"cpu_percent", body.CPUPercent}, {"memory_percent", body.MemoryPercent}, {"disk_percent", body.DiskPercent}} {
		if err := validThresholdPercent(c.name, c.v); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	o := store.NodeThresholdOverride{NodeID: id, CPUPercent: body.CPUPercent, MemoryPercent: body.MemoryPercent, DiskPercent: body.DiskPercent}
	if err := st.SetNodeThresholdOverride(r.Context(), o); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set node thresholds failed", err, slog.String("node_id", id))
		return
	}
	writeJSON(w, http.StatusOK, rt.nodeThresholdsView(r.Context(), o))
}
