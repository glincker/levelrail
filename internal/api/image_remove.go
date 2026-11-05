package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/imagelife"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const defaultImageRemoveMax = 200

type imageRemoveRequest struct {
	Node                   string   `json:"node"`
	IDs                    []string `json:"ids"`
	DryRun                 bool     `json:"dry_run"`
	IncludeUnmanagedUnused bool     `json:"include_unmanaged_unused"`
}

type imageRemoveResponse struct {
	DryRun         bool              `json:"dry_run"`
	Candidates     []imagelife.Entry `json:"candidates"`
	Removed        []string          `json:"removed"`
	Failed         []imageGCFailure  `json:"failed"`
	ReclaimedBytes int64             `json:"reclaimed_bytes"`
}

func (rt *Router) imageRemoveMax(r *http.Request) int {
	if n := rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyImagesRemoveMaxPerRequest); n > 0 {
		return n
	}
	return defaultImageRemoveMax
}

// handleImageRemove handles POST /api/v1/images/remove: removes images the user
// chose, or every unmanaged image no container uses. Each image is re-checked
// against a fresh inventory, and one refusal never fails the request.
func (rt *Router) handleImageRemove(w http.ResponseWriter, r *http.Request) {
	var req imageRemoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.IDs) == 0 && !req.IncludeUnmanagedUnused {
		writeError(w, http.StatusBadRequest, "ids or include_unmanaged_unused is required")
		return
	}
	if limit := rt.imageRemoveMax(r); len(req.IDs) > limit {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("at most %d images per request, got %d", limit, len(req.IDs)))
		return
	}
	inv, ok := rt.imageInventoryFor(w, req.Node)
	if !ok {
		return
	}
	entries, err := rt.buildImageEntries(r.Context(), inv, req.Node, rt.imageRetentionDefaults())
	if err != nil {
		rt.internalError(w, "api: image remove plan failed", err)
		return
	}
	byID := make(map[string]imagelife.Entry, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}
	resp := imageRemoveResponse{DryRun: req.DryRun, Candidates: []imagelife.Entry{}, Removed: []string{}, Failed: []imageGCFailure{}}
	picked := map[string]bool{}
	pick := func(e imagelife.Entry) {
		if !picked[e.ID] {
			picked[e.ID] = true
			resp.Candidates = append(resp.Candidates, e)
		}
	}
	for _, id := range req.IDs {
		e, found := resolveImageEntry(entries, byID, id)
		switch {
		case !found:
			resp.Failed = append(resp.Failed, imageGCFailure{ID: id, Error: "image not found on this node"})
		case imagelife.RemovalBlock(e) != "":
			resp.Failed = append(resp.Failed, imageGCFailure{ID: id, Error: imagelife.RemovalBlock(e)})
		default:
			pick(e)
		}
	}
	if req.IncludeUnmanagedUnused {
		limit := rt.imageRemoveMax(r)
		for _, e := range imagelife.UnmanagedUnused(entries) {
			if len(resp.Candidates) >= limit {
				break
			}
			pick(e)
		}
	}
	if req.DryRun {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	for _, e := range resp.Candidates {
		if err := removeImageEntry(r.Context(), inv, e); err != nil {
			resp.Failed = append(resp.Failed, imageGCFailure{ID: e.ID, Error: err.Error()})
			continue
		}
		resp.Removed = append(resp.Removed, e.ID)
		resp.ReclaimedBytes += imagelife.Reclaimable(e)
	}
	rt.logger.Info("api: image remove finished", slog.Int("removed", len(resp.Removed)), slog.Int("failed", len(resp.Failed)))
	writeJSON(w, http.StatusOK, resp)
}

// resolveImageEntry matches a full ID or a unique prefix, with or without
// "sha256:", so the short IDs the CLI prints work.
func resolveImageEntry(entries []imagelife.Entry, byID map[string]imagelife.Entry, id string) (imagelife.Entry, bool) {
	if e, ok := byID[id]; ok {
		return e, true
	}
	want := strings.TrimPrefix(id, "sha256:")
	if want == "" {
		return imagelife.Entry{}, false
	}
	var hit imagelife.Entry
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(strings.TrimPrefix(e.ID, "sha256:"), want) {
			hit = e
			n++
		}
	}
	return hit, n == 1
}
