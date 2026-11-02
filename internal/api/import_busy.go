package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/store"
)

// writeImportBusy answers 409 with a retry hint while a volume import holds the app.
func writeImportBusy(w http.ResponseWriter) {
	w.Header().Set("Retry-After", strconv.Itoa(wakeRetryAfterSeconds(store.WakeTiming{})))
	writeError(w, http.StatusConflict, "a volume import is running for this app: retry when it finishes")
}

func isImportBusy(err error) bool { return errors.Is(err, backup.ErrImportInProgress) }

// cloneVolumeConflict names another app that already holds one of d's volume names.
func (rt *Router) cloneVolumeConflict(ctx context.Context, d store.DesiredService) (string, error) {
	if len(d.Volumes) == 0 {
		return "", nil
	}
	all, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return "", err
	}
	for _, v := range d.Volumes {
		if other, clash := store.ServiceVolumeOwnerConflict(all, d.Name, v.Name); clash {
			return other, nil
		}
	}
	return "", nil
}
