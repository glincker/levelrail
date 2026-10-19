package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/placement"
)

type zoneNodeResource struct {
	NodeID    string `json:"node_id"`
	Name      string `json:"name"`
	Zone      string `json:"zone,omitempty"`
	Region    string `json:"region,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Online    bool   `json:"online"`
	Workloads int    `json:"workloads"`
}

type zonesResponse struct {
	Zones []placement.ZoneSummary `json:"zones"`
	Nodes []zoneNodeResource      `json:"nodes"`
	// Labeled is true when at least one node carries a zone label, which is
	// what turns failure-domain spread on.
	Labeled bool `json:"labeled"`
}

// handleListZones handles GET /api/v1/nodes/zones: failure domains with the
// nodes and workloads each one carries.
func (rt *Router) handleListZones(w http.ResponseWriter, r *http.Request) {
	cands, err := rt.placementCandidates(r.Context())
	if err != nil {
		rt.internalError(w, "api: list zones: candidates failed", err)
		return
	}
	out := zonesResponse{Zones: placement.Zones(cands), Nodes: make([]zoneNodeResource, 0, len(cands))}
	for _, c := range cands {
		if placement.ZoneOf(c.Labels) != "" {
			out.Labeled = true
		}
		out.Nodes = append(out.Nodes, zoneNodeResource{
			NodeID: c.ID, Name: c.Name, Zone: placement.ZoneOf(c.Labels), Region: c.Labels[placement.LabelRegion],
			Provider: c.Labels[placement.LabelProvider], Online: c.Online, Workloads: c.Load,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
