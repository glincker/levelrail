package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TrialKeeper makes a trial deployment permanent. *store.DB satisfies it.
type TrialKeeper interface {
	KeepTrialApp(ctx context.Context, name string) (store.TrialKeep, error)
}

type keepTrialResponse struct {
	Name        string    `json:"name"`
	Kept        []string  `json:"kept"`
	KeptAt      time.Time `json:"kept_at"`
	AlreadyKept bool      `json:"already_kept"`
}

// handleKeepTrialApp handles POST /api/v1/apps/{name}/keep: a one-click
// trial stops being flagged as throwaway, with every sibling service of its
// app. Repeating it answers the same; a never-trial app is 409.
func (rt *Router) handleKeepTrialApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if rt.trialKeeper == nil {
		writeError(w, http.StatusNotImplemented, "keeping trial apps is not supported by this store")
		return
	}
	res, err := rt.trialKeeper.KeepTrialApp(r.Context(), name)
	switch {
	case errors.Is(err, store.ErrServiceNotFound):
		writeError(w, http.StatusNotFound, "app not found")
		return
	case errors.Is(err, store.ErrServiceNotTrial):
		writeError(w, http.StatusConflict, "app is not a trial deployment")
		return
	case err != nil:
		rt.internalError(w, "api: keep trial app failed", err, slog.String("name", name))
		return
	}
	kept := res.Services
	if kept == nil {
		kept = []string{}
	}
	writeJSON(w, http.StatusOK, keepTrialResponse{Name: name, Kept: kept, KeptAt: res.KeptAt, AlreadyKept: res.AlreadyKept})
}
