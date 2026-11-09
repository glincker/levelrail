package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DBSpaceStore is the database surface the space report and compaction use.
type DBSpaceStore interface {
	SpaceReport(ctx context.Context, topN int) (store.DBSpace, error)
	CompactDatabase(ctx context.Context) error
}

type dbCompactRequest struct {
	Force bool `json:"force"`
}

type dbCompactResponse struct {
	Compacted   bool          `json:"compacted"`
	Reason      string        `json:"reason,omitempty"`
	BytesBefore int64         `json:"bytes_before"`
	BytesAfter  int64         `json:"bytes_after"`
	Space       store.DBSpace `json:"space"`
}

func (rt *Router) dbSpaceStore() (DBSpaceStore, bool) {
	p := rt.cpHealth.probe.Load()
	if p == nil || p.DB == nil {
		return nil, false
	}
	s, ok := p.DB.(DBSpaceStore)
	return s, ok
}

func (rt *Router) handleGetDBSpace(w http.ResponseWriter, r *http.Request) {
	s, ok := rt.dbSpaceStore()
	if !ok {
		writeError(w, http.StatusNotImplemented, "database space report is not available")
		return
	}
	rep, err := s.SpaceReport(r.Context(), rt.runtimeSettings.Limit(r.Context(), runtimesettings.KeyDBSpaceRows))
	if err != nil {
		rt.internalError(w, "api: database space report failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// handleCompactDB runs only on an explicit request and declines when too few pages are free.
func (rt *Router) handleCompactDB(w http.ResponseWriter, r *http.Request) {
	s, ok := rt.dbSpaceStore()
	if !ok {
		writeError(w, http.StatusNotImplemented, "database compaction is not available")
		return
	}
	var req dbCompactRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rows := rt.runtimeSettings.Limit(r.Context(), runtimesettings.KeyDBSpaceRows)
	before, err := s.SpaceReport(r.Context(), rows)
	if err != nil {
		rt.internalError(w, "api: database space report failed", err)
		return
	}
	minFree := rt.runtimeSettings.Limit(r.Context(), runtimesettings.KeyDBCompactMinFree)
	if !req.Force && before.FreePercent < minFree {
		writeJSON(w, http.StatusOK, dbCompactResponse{Reason: "not enough free space to reclaim", BytesBefore: before.TotalBytes, BytesAfter: before.TotalBytes, Space: before})
		return
	}
	if err := s.CompactDatabase(r.Context()); err != nil {
		rt.internalError(w, "api: database compaction failed", err)
		return
	}
	after, err := s.SpaceReport(r.Context(), rows)
	if err != nil {
		rt.internalError(w, "api: database space report failed", err)
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	rt.logger.Info("api: database compacted", slog.String("actor", actor), slog.Int64("bytes_before", before.TotalBytes), slog.Int64("bytes_after", after.TotalBytes))
	writeJSON(w, http.StatusOK, dbCompactResponse{Compacted: true, BytesBefore: before.TotalBytes, BytesAfter: after.TotalBytes, Space: after})
}
