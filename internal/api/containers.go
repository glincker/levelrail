package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// containerPortResource mirrors docker.PortBinding exactly, no invented
// fields, the same "reuse what the runtime already carries" convention
// images.go's imageResource establishes.
type containerPortResource struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port"`
	Protocol      string `json:"protocol"`
}

// containerResource is GET /api/v1/system/containers' wire shape: every
// container Docker knows about on this node, whether or not Levelrail
// manages it. Deliberately read-only (no id, no stop/restart action on
// this route): a container this platform's own reconciler manages will
// simply be recreated to match desired state if stopped out-of-band, so
// any control action belongs on the existing per-app
// stop/start/restart/exec routes, which update desired state correctly,
// not a raw docker-level mutation here that would fight the reconciler.
// This endpoint exists purely for visibility, "what is actually running
// on this box right now, and on what port," including containers this
// platform didn't create.
type containerResource struct {
	Name    string                  `json:"name"`
	Image   string                  `json:"image"`
	Running bool                    `json:"running"`
	Ports   []containerPortResource `json:"ports"`
}

func toContainerResource(c docker.ContainerState) containerResource {
	ports := make([]containerPortResource, 0, len(c.Ports))
	for _, p := range c.Ports {
		ports = append(ports, containerPortResource{ContainerPort: p.ContainerPort, HostPort: p.HostPort, Protocol: p.Protocol})
	}
	return containerResource{Name: c.Name, Image: c.Image, Running: c.Running, Ports: ports}
}

// handleListContainers handles GET /api/v1/system/containers. 501 if no
// ContainerLister is configured, the same "not configured" shape every
// other optional-dependency route in this package uses (see
// ImageLister's own doc comment).
func (rt *Router) handleListContainers(w http.ResponseWriter, r *http.Request) {
	if rt.containers == nil {
		writeError(w, http.StatusNotImplemented, "container listing is not configured on this control plane")
		return
	}

	containers, err := rt.containers.ListByPrefix(r.Context(), "")
	if err != nil {
		rt.internalError(w, "api: list containers failed", err)
		return
	}

	out := make([]containerResource, 0, len(containers))
	for _, c := range containers {
		out = append(out, toContainerResource(c))
	}
	writeJSON(w, http.StatusOK, out)
}
