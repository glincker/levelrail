package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/GLINCKER/levelrail/internal/store"
)

// volumeNameRe mirrors internal/spec's own unexported nameLike pattern
// for a volume's logical name (lowercase alphanumeric and hyphens,
// starting with a letter): kept as its own copy since this endpoint
// validates a plain API request, not a parsed app.yaml.
var volumeNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// setAppVolumesRequest is PUT /api/v1/apps/{name}/volumes' body: the
// service's whole desired volume list, the same full-replace shape
// setEgressPolicyRequest already uses.
type setAppVolumesRequest struct {
	Volumes []appVolumeResource `json:"volumes"`
}

// handleSetAppVolumes handles PUT /api/v1/apps/{name}/volumes: attach
// (or remove) a named Docker volume outside a redeploy, mirroring
// handleSetAppEgressPolicy/handleSetAppHealth's own dedicated-endpoint
// shape (an ordinary PUT /api/v1/apps/{name} never carries Volumes
// forward, see appVolumeResource's own doc comment). The mount is baked
// in at container create time, same as Env, so a running container
// keeps its old mount set until the next deploy or restart.
func (rt *Router) handleSetAppVolumes(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setAppVolumesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: set app volumes: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	volumes, errMsg := resolveAppVolumes(name, req.Volumes, svc.BindMounts)
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}

	if err := rt.apps.UpdateServiceVolumes(r.Context(), name, volumes); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set app volumes failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.nudgeReconciler()
	svc.Volumes = volumes
	writeJSON(w, http.StatusOK, setAppVolumesResponse{Name: name, Volumes: toAppVolumeResources(*svc)})
}

// setAppVolumesResponse is handleSetAppVolumes' response body: the same
// resource shape GET /api/v1/apps/{name} already embeds under its own
// "volumes" field.
type setAppVolumesResponse struct {
	Name    string              `json:"name"`
	Volumes []appVolumeResource `json:"volumes,omitempty"`
}

// resolveAppVolumes validates req and translates each logical name into
// its platform-prefixed Docker volume name, the same "app-" + service
// name + "-" + logical name convention internal/deploy's volumeName
// uses for an app.yaml-declared volume, so a later redeploy declaring
// the same logical name resolves to this same volume. A non-empty
// errMsg, not an error, since every caller just writes it as a 400.
func resolveAppVolumes(serviceName string, req []appVolumeResource, existingBindMounts []store.ServiceBindMount) (volumes []store.ServiceVolume, errMsg string) {
	seenNames := make(map[string]bool, len(req))
	seenPaths := make(map[string]bool, len(req)+len(existingBindMounts))
	for _, m := range existingBindMounts {
		seenPaths[m.ContainerPath] = true
	}

	prefix := "app-" + serviceName + "-"
	volumes = make([]store.ServiceVolume, len(req))
	for i, v := range req {
		if !volumeNameRe.MatchString(v.Name) {
			return nil, fmt.Sprintf("volume name %q must be lowercase alphanumeric and hyphens, starting with a letter", v.Name)
		}
		if len(v.ContainerPath) < 2 || v.ContainerPath[0] != '/' {
			return nil, fmt.Sprintf("volume %q: mount path %q must be an absolute path", v.Name, v.ContainerPath)
		}
		if seenNames[v.Name] {
			return nil, fmt.Sprintf("duplicate volume name %q", v.Name)
		}
		seenNames[v.Name] = true
		if seenPaths[v.ContainerPath] {
			return nil, fmt.Sprintf("mount path %q is already used by another volume or bind mount", v.ContainerPath)
		}
		seenPaths[v.ContainerPath] = true
		volumes[i] = store.ServiceVolume{Name: prefix + v.Name, ContainerPath: v.ContainerPath}
	}
	return volumes, ""
}
