package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/orphans"
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
// manages it. This route itself stays read-only: a Managed container is
// controlled from its own app page's stop/start/restart/exec routes,
// which update desired state correctly, not a raw docker-level mutation
// here that would fight the reconciler. An orphaned (Managed: false)
// container has no desired state to fight, so it gets its own
// stop/remove/claim routes instead (containers_orphaned.go).
type containerResource struct {
	Name    string                  `json:"name"`
	Image   string                  `json:"image"`
	Running bool                    `json:"running"`
	Ports   []containerPortResource `json:"ports"`
	// Managed reports whether this is a Levelrail-managed container: it
	// carries this platform's own instance label and its name still
	// matches a real app or database record in current desired state
	// (isManagedContainer, containers_orphaned.go). False covers both a
	// container this platform never created and one it created whose
	// owning app/database was since deleted without the container being
	// cleaned up; the orphaned-container routes (containers_orphaned.go)
	// only ever act on the latter kind.
	Managed bool `json:"managed"`
}

func toContainerResource(c docker.ContainerState, managed bool) containerResource {
	ports := make([]containerPortResource, 0, len(c.Ports))
	for _, p := range c.Ports {
		ports = append(ports, containerPortResource{ContainerPort: p.ContainerPort, HostPort: p.HostPort, Protocol: p.Protocol})
	}
	return containerResource{Name: c.Name, Image: c.Image, Running: c.Running, Ports: ports, Managed: managed}
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
	desired, err := rt.loadOrphanDesired(r.Context())
	if err != nil {
		rt.internalError(w, "api: list containers: compute desired container names failed", err)
		return
	}

	hidden, err := rt.hiddenContainerPrefixes(r)
	if err != nil {
		rt.internalError(w, "api: list containers: resolve visibility failed", err)
		return
	}

	out := make([]containerResource, 0, len(containers))
	for _, c := range containers {
		if containerHidden(c.Name, hidden) {
			continue
		}
		out = append(out, toContainerResource(c, orphans.IsManaged(c, desired, "")))
	}
	writeJSON(w, http.StatusOK, out)
}

// hiddenContainerPrefixes lists "<app>-" prefixes of apps the caller's IAM
// policies deny, so a denied app's containers do not show up by name.
func (rt *Router) hiddenContainerPrefixes(r *http.Request) ([]string, error) {
	canRead, filtered, err := rt.callerAppVisibility(r)
	if err != nil {
		return nil, fmt.Errorf("resolve caller visibility: %w", err)
	}
	if !filtered {
		return nil, nil
	}
	services, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	var hidden []string
	for _, s := range services {
		if !canRead(s.Name) {
			hidden = append(hidden, s.Name+"-")
		}
	}
	return hidden, nil
}

func containerHidden(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
