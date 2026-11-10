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
	// DropIncompatibleParams removes running engine parameters the new
	// version refuses instead of blocking the change with 409.
	DropIncompatibleParams bool `json:"drop_incompatible_params"`
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

	plan, ok := rt.planParamsForVersion(w, r, existing, req.Version, req.DropIncompatibleParams)
	if !ok {
		return
	}
	if plan.drops() {
		// The version change and the drop commit together or not at all.
		audit, ok := rt.dropAudit(r, http.MethodPut, name, plan)
		if !ok {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		dropper, _ := rt.databases.(databaseParamsDropper)
		if err := dropper.SetDatabaseVersionDroppingEngineParams(r.Context(), name, existing.Version, req.Version, plan.applied, plan.keep, audit); err != nil {
			if errors.Is(err, store.ErrEngineParamsConflict) {
				writeError(w, http.StatusConflict, "the database changed meanwhile: reload and try again")
				return
			}
			rt.internalError(w, "api: set database version failed", err, slog.String("name", name))
			return
		}
	} else if dropper, ok := rt.databases.(databaseParamsDropper); ok && plan.checked {
		// Only while the re-validated set still runs, so a concurrent apply cannot slip past.
		if err := dropper.SetDatabaseVersionIfEngineParams(r.Context(), name, existing.Version, req.Version, plan.applied); err != nil {
			if errors.Is(err, store.ErrEngineParamsConflict) {
				writeError(w, http.StatusConflict, "the database changed meanwhile: reload and try again")
				return
			}
			rt.internalError(w, "api: set database version failed", err, slog.String("name", name))
			return
		}
	} else {
		desired := *existing
		desired.Version = req.Version
		if err := rt.databases.SaveDesiredDatabase(r.Context(), desired); err != nil {
			rt.internalError(w, "api: set database version failed", err, slog.String("name", name))
			return
		}
	}
	if plan.pending {
		w.Header().Set(paramsPendingHeader, "staged")
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
