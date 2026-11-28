package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/imageupdate"
	"github.com/glincker/levelrail/internal/reconcile/database"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// latestVersionKeyword is the request value that means "pick the newest stable version".
const latestVersionKeyword = "latest"

// Defaults used only when runtime settings are not wired (tests, bare routers).
const (
	defaultLatestCacheTTL = time.Hour
	defaultLatestLookup   = 8 * time.Second
)

// anonymousTagLister drops registry auth: the engine images are public.
type anonymousTagLister struct{ l imageupdate.TagLister }

func (a anonymousTagLister) ListTags(ctx context.Context, repo string) ([]string, error) {
	return a.l.ListTags(ctx, repo, nil)
}

// versionResolution is response-only: which tag an omitted or "latest" version became, and when.
type versionResolution struct {
	Requested         string    `json:"requested"`
	Version           string    `json:"version"`
	Source            string    `json:"source"`
	ResolvedAt        time.Time `json:"resolved_at"`
	SkippedNewerMajor string    `json:"skipped_newer_major,omitempty"`
}

type latestVersionResponse struct {
	Enabled    bool               `json:"enabled"`
	Resolution *versionResolution `json:"resolution,omitempty"`
}

func (rt *Router) databaseAutoLatest(ctx context.Context) bool {
	if rs := rt.RuntimeSettings(); rs != nil {
		return rs.Bool(ctx, runtimesettings.KeyDatabasesAutoLatest)
	}
	return true
}

// resolveLatestDatabaseVersion asks only the engine's own fixed image repository for tags.
func (rt *Router) resolveLatestDatabaseVersion(ctx context.Context, engine string) (versionResolution, bool) {
	fallback := defaultEngineVersion(engine)
	ttl, lookup := defaultLatestCacheTTL, defaultLatestLookup
	if rs := rt.RuntimeSettings(); rs != nil {
		ttl = time.Duration(rs.Int(ctx, runtimesettings.KeyDatabasesLatestVersionCacheSeconds)) * time.Second
		if n := rs.Int(ctx, runtimesettings.KeyDatabasesLatestLookupSeconds); n > 0 {
			lookup = time.Duration(n) * time.Second
		}
	}
	var inner imageupdate.TagLister = imageupdate.HTTPTagLister{}
	if rt.imageTagLister != nil {
		inner = rt.imageTagLister
	}
	lister := anonymousTagLister{inner}
	repo := strings.TrimSuffix(database.ImageRef(engine, "x"), ":x")
	lctx, cancel := context.WithTimeout(ctx, lookup)
	defer cancel()
	res := rt.databaseLatest.Resolve(lctx, lister, engine, repo, fallback, ttl)
	if res.Version == "" {
		return versionResolution{}, false
	}
	if res.SkippedNewer != "" && res.Source == "registry" {
		rt.logger.Warn("api: registry has a newer major than the upgrade catalog, not picking it",
			slog.String("engine", engine), slog.String("tag", res.SkippedNewer))
	}
	return versionResolution{
		Requested: latestVersionKeyword, Version: res.Version, Source: res.Source,
		ResolvedAt: res.ResolvedAt.UTC(), SkippedNewerMajor: res.SkippedNewer,
	}, true
}

// applyLatestDatabaseVersion turns an omitted or "latest" version into a concrete stored one.
// It returns false after writing an error response.
func (rt *Router) applyLatestDatabaseVersion(w http.ResponseWriter, r *http.Request, req *databaseResource) bool {
	if req.Version != "" && req.Version != latestVersionKeyword {
		return true
	}
	if supported, err := store.IsSupportedEngine(req.Engine); err != nil || !supported {
		return true // validation reports the engine problem
	}
	if !rt.databaseAutoLatest(r.Context()) {
		if req.Version == "" {
			writeError(w, http.StatusBadRequest, "version is required")
		} else {
			writeError(w, http.StatusBadRequest, "automatic latest version is turned off (databases.auto_latest); pass a version")
		}
		return false
	}
	res, ok := rt.resolveLatestDatabaseVersion(r.Context(), req.Engine)
	if !ok {
		writeError(w, http.StatusBadRequest, "no default version is known for this engine; pass a version")
		return false
	}
	req.Version = res.Version
	req.VersionResolution = &res
	rt.matchLeftoverMajor(r.Context(), req)
	return true
}

// handleLatestDatabaseVersion handles GET /api/v1/database-engines/{id}/latest, for the create form.
func (rt *Router) handleLatestDatabaseVersion(w http.ResponseWriter, r *http.Request) {
	engine := r.PathValue("id")
	if supported, err := store.IsSupportedEngine(engine); err != nil || !supported {
		writeError(w, http.StatusNotFound, "unknown engine")
		return
	}
	if !rt.databaseAutoLatest(r.Context()) {
		writeJSON(w, http.StatusOK, latestVersionResponse{})
		return
	}
	res, ok := rt.resolveLatestDatabaseVersion(r.Context(), engine)
	if !ok {
		writeError(w, http.StatusNotFound, "no latest version known")
		return
	}
	writeJSON(w, http.StatusOK, latestVersionResponse{Enabled: true, Resolution: &res})
}
