package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Name is the logical app.yaml name, not the platform-prefixed Docker volume name; response-only on the app resource.
type appVolumeResource struct {
	Name          string `json:"name"`
	ContainerPath string `json:"container_path"`
}

// Strips the store's fixed Docker name prefix to recover logical names; nil when there are no volumes.
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

// Bind mounts are set only via compose import, so this resource is response-only.
type appBindMountResource struct {
	HostPath      string `json:"host_path"`
	ContainerPath string `json:"container_path"`
	ReadOnly      bool   `json:"read_only,omitempty"`
}

// toAppBindMountResources is toAppVolumeResources' bind-mount
// counterpart: nil when svc has none, matching appResource's own
// omitempty on this field.
func toAppBindMountResources(svc store.DesiredService) []appBindMountResource {
	if len(svc.BindMounts) == 0 {
		return nil
	}
	out := make([]appBindMountResource, len(svc.BindMounts))
	for i, m := range svc.BindMounts {
		out[i] = appBindMountResource{HostPath: m.HostPath, ContainerPath: m.ContainerPath, ReadOnly: m.ReadOnly}
	}
	return out
}

// loadServiceVolume resolves {name}/{volume} to a Docker volume name and writes the 404/500 response on failure.
func (rt *Router) loadServiceVolume(w http.ResponseWriter, r *http.Request, serviceName, volumeName, logContext string) (dockerVolumeName string, ok bool) {
	svc, err := rt.apps.GetDesiredService(r.Context(), serviceName)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return "", false
	}
	if err != nil {
		rt.internalError(w, logContext, err, slog.String("name", serviceName))
		return "", false
	}

	dockerVolumeName, found := store.ServiceVolumeDockerName(*svc, volumeName)
	if !found {
		writeError(w, http.StatusNotFound, "volume not found on this app")
		return "", false
	}
	return dockerVolumeName, true
}
