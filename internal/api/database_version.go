package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

var databaseVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type setDatabaseVersionRequest struct {
	Version string `json:"version"`
}

// handleSetDatabaseVersion handles PUT /api/v1/databases/{name}/version:
// a minor or patch image change, applied by the reconciler as a stop,
// remove and recreate over the same data volume. Major upgrades are
// refused because the on-disk format is not portable; use backup plus
// restore-as-new for those.
func (rt *Router) handleSetDatabaseVersion(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setDatabaseVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	existing, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: set database version: load failed", err, slog.String("name", name))
		return
	}

	if err := checkVersionChange(existing.Engine, existing.Version, req.Version); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	desired := *existing
	desired.Version = req.Version
	if err := rt.databases.SaveDesiredDatabase(r.Context(), desired); err != nil {
		rt.internalError(w, "api: set database version failed", err, slog.String("name", name))
		return
	}
	rt.reloadAndWriteDatabase(w, r, name, "set database version")
}

// checkVersionChange validates the tag shape, then applies the shared
// in-place rule (database.CheckInPlaceVersionChange) the upgrade runner uses too.
func checkVersionChange(engine, from, to string) error {
	if !databaseVersionPattern.MatchString(to) {
		return errors.New("version must be an image tag such as \"16.4\"")
	}
	return database.CheckInPlaceVersionChange(engine, from, to)
}

func majorVersion(v string) (int, bool) {
	return database.LeadingMajor(v)
}
