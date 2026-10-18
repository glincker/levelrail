package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

const (
	buildCacheRecordsShown  = 100
	defaultCachePruneAgeHrs = 168
)

// BuildCacheResolver returns the build cache admin of one node, "" being the control plane's own.
type BuildCacheResolver func(nodeID string) (docker.BuildCacheAdmin, error)

// WithBuildCache enables the /api/v1/builds/cache routes.
func WithBuildCache(r BuildCacheResolver) Option {
	return func(rt *Router) { rt.buildCacheNodes = r }
}

type buildCacheRecordResource struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Description string     `json:"description"`
	SizeBytes   int64      `json:"size_bytes"`
	InUse       bool       `json:"in_use"`
	UsageCount  int        `json:"usage_count"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

type buildCacheNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type buildCacheResponse struct {
	Node             string                     `json:"node"`
	Supported        bool                       `json:"supported"`
	Reason           string                     `json:"reason,omitempty"`
	TotalBytes       int64                      `json:"total_bytes"`
	InUseBytes       int64                      `json:"in_use_bytes"`
	ReclaimableBytes int64                      `json:"reclaimable_bytes"`
	RecordCount      int                        `json:"record_count"`
	Records          []buildCacheRecordResource `json:"records"`
	Nodes            []buildCacheNode           `json:"nodes"`
}

type buildCachePruneRequest struct {
	Node           string `json:"node"`
	OlderThanHours *int   `json:"older_than_hours,omitempty"`
	Apply          bool   `json:"apply"`
}

type buildCachePruneResponse struct {
	Node           string                     `json:"node"`
	Applied        bool                       `json:"applied"`
	OlderThanHours int                        `json:"older_than_hours"`
	Candidates     []buildCacheRecordResource `json:"candidates"`
	CandidateCount int                        `json:"candidate_count"`
	WouldFreeBytes int64                      `json:"would_free_bytes"`
	FreedBytes     int64                      `json:"freed_bytes"`
	RemovedCount   int                        `json:"removed_count"`
}

func toCacheRecordResource(r docker.BuildCacheRecord) buildCacheRecordResource {
	out := buildCacheRecordResource{ID: r.ID, Type: r.Type, Description: r.Description, SizeBytes: r.SizeBytes, InUse: r.InUse, UsageCount: r.UsageCount}
	if !r.LastUsedAt.IsZero() {
		t := r.LastUsedAt
		out.LastUsedAt = &t
	}
	return out
}

func (rt *Router) buildCacheFor(w http.ResponseWriter, nodeID string) (docker.BuildCacheAdmin, bool) {
	if rt.buildCacheNodes == nil {
		writeError(w, http.StatusNotImplemented, "build cache management is not configured on this control plane")
		return nil, false
	}
	admin, err := rt.buildCacheNodes(nodeID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "this node cannot be reached: "+err.Error())
		return nil, false
	}
	return admin, true
}

func (rt *Router) buildCacheNodeList(r *http.Request) []buildCacheNode {
	out := []buildCacheNode{{ID: "", Name: "local"}}
	if rt.nodes == nil {
		return out
	}
	nodes, err := rt.nodes.ListNodes(r.Context())
	if err != nil {
		return out
	}
	for _, n := range nodes {
		out = append(out, buildCacheNode{ID: n.ID, Name: n.Name})
	}
	return out
}

// handleBuildCache handles GET /api/v1/builds/cache?node=: BuildKit cache usage of one node.
func (rt *Router) handleBuildCache(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("node")
	resp := buildCacheResponse{Node: nodeID, Records: []buildCacheRecordResource{}, Nodes: rt.buildCacheNodeList(r)}
	admin, ok := rt.buildCacheFor(w, nodeID)
	if !ok {
		return
	}
	records, err := admin.BuildCacheRecords(r.Context())
	if err != nil {
		resp.Reason = err.Error()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp.Supported = true
	resp.RecordCount = len(records)
	for i, rec := range records {
		resp.TotalBytes += rec.SizeBytes
		if rec.InUse {
			resp.InUseBytes += rec.SizeBytes
		} else {
			resp.ReclaimableBytes += rec.SizeBytes
		}
		if i < buildCacheRecordsShown {
			resp.Records = append(resp.Records, toCacheRecordResource(rec))
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleBuildCachePrune handles POST /api/v1/builds/cache/prune. Without apply it only previews.
func (rt *Router) handleBuildCachePrune(w http.ResponseWriter, r *http.Request) {
	var req buildCachePruneRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	hours := defaultCachePruneAgeHrs
	if req.OlderThanHours != nil {
		hours = *req.OlderThanHours
	}
	if hours < 0 || hours > 24*365 {
		writeError(w, http.StatusBadRequest, "older_than_hours must be between 0 and 8760")
		return
	}
	admin, ok := rt.buildCacheFor(w, req.Node)
	if !ok {
		return
	}
	olderThan := time.Duration(hours) * time.Hour
	records, err := admin.BuildCacheRecords(r.Context())
	if err != nil {
		rt.internalError(w, "api: build cache prune: list records failed", err)
		return
	}
	plan := docker.PlanBuildCachePrune(records, olderThan, time.Now())
	resp := buildCachePruneResponse{Node: req.Node, OlderThanHours: hours, Candidates: []buildCacheRecordResource{}, CandidateCount: len(plan)}
	for i, rec := range plan {
		resp.WouldFreeBytes += rec.SizeBytes
		if i < buildCacheRecordsShown {
			resp.Candidates = append(resp.Candidates, toCacheRecordResource(rec))
		}
	}
	if req.Apply {
		res, err := admin.PruneBuildCacheOlderThan(r.Context(), olderThan)
		if err != nil {
			rt.internalError(w, "api: build cache prune failed", err)
			return
		}
		resp.Applied = true
		resp.FreedBytes = int64(res.ReclaimedBytes) //nolint:gosec // reclaimed bytes fit int64
		resp.RemovedCount = len(res.Removed)
	}
	writeJSON(w, http.StatusOK, resp)
}
