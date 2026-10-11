package api

import (
	"net/http"
	"os"
	"sort"

	"github.com/GLINCKER/levelrail/internal/diagnose"
)

// procNetTables are read for the "what is listening" hint; absent off Linux.
var procNetTables = []string{"/proc/net/tcp", "/proc/net/tcp6"}

type hostPortCandidate struct {
	Port int `json:"port"`
	// Label names this platform's own listener on the port, if any.
	Label   string `json:"label,omitempty"`
	Blocked bool   `json:"blocked,omitempty"`
}

type containerCandidate struct {
	Name  string `json:"name"`
	Ports []int  `json:"ports,omitempty"`
}

type appCandidate struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

// standaloneCandidatesResource is GET /api/v1/domains/standalone/candidates.
type standaloneCandidatesResource struct {
	HostPorts  []hostPortCandidate  `json:"host_ports"`
	Containers []containerCandidate `json:"containers"`
	Apps       []appCandidate       `json:"apps"`
}

// handleStandaloneCandidates handles GET /api/v1/domains/standalone/candidates:
// listening host ports, running containers and apps a standalone domain
// could target. Best effort: a source that cannot be read is left empty.
func (rt *Router) handleStandaloneCandidates(w http.ResponseWriter, r *http.Request) {
	out := standaloneCandidatesResource{HostPorts: []hostPortCandidate{}, Containers: []containerCandidate{}, Apps: []appCandidate{}}
	var tables []string
	for _, p := range procNetTables {
		if b, err := os.ReadFile(p); err == nil { //nolint:gosec // fixed /proc paths
			tables = append(tables, string(b))
		}
	}
	control := map[int]hostPortCandidate{}
	for _, cp := range rt.standaloneSafety.ControlPorts {
		control[cp.Port] = hostPortCandidate{Port: cp.Port, Label: cp.Label, Blocked: cp.AlwaysBlock}
	}
	for _, p := range diagnose.ParseListeningPorts(tables...) {
		c, ok := control[p]
		if !ok {
			c = hostPortCandidate{Port: p}
		}
		out.HostPorts = append(out.HostPorts, c)
	}
	sort.Slice(out.HostPorts, func(i, j int) bool { return out.HostPorts[i].Port < out.HostPorts[j].Port })
	if rt.containers != nil {
		if list, err := rt.containers.ListByPrefix(r.Context(), ""); err == nil {
			for _, c := range list {
				if !c.Running {
					continue
				}
				cc := containerCandidate{Name: c.Name}
				for _, p := range c.Ports {
					cc.Ports = append(cc.Ports, p.ContainerPort)
				}
				out.Containers = append(out.Containers, cc)
			}
			sort.Slice(out.Containers, func(i, j int) bool { return out.Containers[i].Name < out.Containers[j].Name })
		}
	}
	if svcs, err := rt.apps.ListDesiredServices(r.Context()); err == nil {
		for _, s := range svcs {
			out.Apps = append(out.Apps, appCandidate{Name: s.Name, Port: s.Port})
		}
	}
	writeJSON(w, http.StatusOK, out)
}
