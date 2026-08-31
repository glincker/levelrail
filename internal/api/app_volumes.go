package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// appVolumeResource is the wire shape for one of an app's named Docker
// volumes (spec.Service.Volumes, store.ServiceVolume): Name is the
// logical name an operator wrote in app.yaml, not the resolved,
// platform-prefixed Docker volume name (store.ServiceVolumeDockerName's
// own doc comment explains why those differ). Response-only, the same
// "shown but not settable through this endpoint" boundary appResource's
// own NodeID/ProjectID fields already establish: volumes are declared
// through app.yaml, not this API.
type appVolumeResource struct {
	Name          string `json:"name"`
	ContainerPath string `json:"container_path"`
}

// toAppVolumeResources derives each of svc's volumes' logical names back
// from their resolved store.ServiceVolume.Name, the same fixed-prefix
// strip store.ServiceVolumeDockerName uses in the opposite direction:
// nil when svc has none, matching appResource's own omitempty on this
// field.
func toAppVolumeResources(svc store.DesiredService) []appVolumeResource {
	if len(svc.Volumes) == 0 {
		return nil
	}
	prefix := "app-" + svc.Name + "-"
	out := make([]appVolumeResource, len(svc.Volumes))
	for i, v := range svc.Volumes {
		out[i] = appVolumeResource{Name: strings.TrimPrefix(v.Name, prefix), ContainerPath: v.ContainerPath}
	}
	return out
}

// loadServiceVolume resolves {name}/{volume} path values to volumeName's
// real Docker volume name, writing the 404/500 response itself on
// failure: the shared shape every volume backup/restore handler in
// app_volume_backups.go/app_volume_restore.go needs before it can do
// anything else, the same role loadDatabaseForRunner (backups.go)
// already plays for the database path.
func (rt *Router) loadServiceVolume(w http.ResponseWriter, r *http.Request, serviceName, volumeName, logContext string) (dockerVolumeName string, ok bool) {
	svc, err := rt.apps.GetDesiredService(r.Context(), serviceName)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return "", false
	}
	if err != nil {
		rt.logger.Error(logContext, slog.String("error", err.Error()), slog.String("name", serviceName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return "", false
	}

	dockerVolumeName, found := store.ServiceVolumeDockerName(*svc, volumeName)
	if !found {
		writeError(w, http.StatusNotFound, "volume not found on this app")
		return "", false
	}
	return dockerVolumeName, true
}
