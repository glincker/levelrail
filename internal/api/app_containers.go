package api

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// shortContainerIDLen matches the abbreviation docker ps shows.
const shortContainerIDLen = 12

// Field names reported in appContainerResource.Unavailable when the
// node's transport cannot supply them (the agent protocol does not carry
// them yet).
const (
	unavailableHealth       = "health"
	unavailableRestartCount = "restart_count"
	unavailableStartedAt    = "started_at"
	unavailableNetworks     = "networks"
	unavailableImageDigest  = "image_digest"
	unavailableStats        = "stats"
	unavailableAliases      = "networks.aliases"
)

type appContainerHealth struct {
	Status        string `json:"status"`
	FailingStreak int    `json:"failing_streak"`
	LastOutput    string `json:"last_output"`
}

type appContainerPort struct {
	Container int    `json:"container"`
	Host      int    `json:"host"`
	Proto     string `json:"proto"`
	Bind      string `json:"bind"`
}

type appContainerNetwork struct {
	Name    string   `json:"name"`
	IP      string   `json:"ip"`
	Aliases []string `json:"aliases"`
}

type appContainerMount struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Target string `json:"target"`
	RO     bool   `json:"ro"`
}

type appContainerLimits struct {
	CPU         float64 `json:"cpu"`
	MemoryBytes int64   `json:"memory_bytes"`
}

type appContainerStats struct {
	CPUPercent       float64 `json:"cpu_percent"`
	MemoryBytes      uint64  `json:"memory_bytes"`
	MemoryLimitBytes uint64  `json:"memory_limit_bytes"`
}

// appContainerResource is one replica of an app. Env values and secrets
// are never part of it. Unavailable names fields the replica's node
// could not report, so an empty value is never mistaken for a real one.
type appContainerResource struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Image        string                `json:"image"`
	ImageDigest  string                `json:"image_digest"`
	State        string                `json:"state"`
	RestartCount *int                  `json:"restart_count,omitempty"`
	StartedAt    *time.Time            `json:"started_at,omitempty"`
	Node         string                `json:"node"`
	Health       *appContainerHealth   `json:"health,omitempty"`
	Ports        []appContainerPort    `json:"ports"`
	Networks     []appContainerNetwork `json:"networks"`
	Mounts       []appContainerMount   `json:"mounts"`
	Limits       appContainerLimits    `json:"limits"`
	Stats        *appContainerStats    `json:"stats,omitempty"`
	Unavailable  []string              `json:"unavailable"`
}

// handleListAppContainers handles GET /api/v1/apps/{name}/containers: one
// entry per replica container, read live from the app's own node.
func (rt *Router) handleListAppContainers(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.loadExecApp(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	lister, err := rt.exposureLister(svc.NodeID)
	if err != nil || lister == nil {
		writeError(w, http.StatusNotImplemented, "container inspection is not configured for this app's node")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dockerInspectTimeout)
	defer cancel()
	states, err := lister.ListByPrefix(ctx, svc.Name+"-")
	if err != nil {
		rt.logger.Error("api: list app containers failed", "name", svc.Name, "error", err.Error())
		writeError(w, http.StatusBadGateway, "app's node is not currently reachable")
		return
	}

	owned := make([]docker.ContainerState, 0, len(states))
	for _, s := range states {
		if application.OwnsContainer(svc.Name, s.Name) {
			owned = append(owned, s)
		}
	}
	sort.Slice(owned, func(i, j int) bool { return owned[i].Name < owned[j].Name })
	// Bounded by what the app can legitimately own: current replicas plus
	// a previous release still draining.
	if limit := max(svc.Replicas, 1) * 2; len(owned) > limit {
		owned = owned[:limit]
	}

	node := localNodeLabel
	if !rt.isLocalNode(svc.NodeID) {
		node = svc.NodeID
	}
	out := make([]appContainerResource, 0, len(owned))
	for _, s := range owned {
		out = append(out, buildAppContainer(ctx, lister, svc, s, node))
	}
	writeJSON(w, http.StatusOK, out)
}

func buildAppContainer(ctx context.Context, lister ContainerLister, svc *store.DesiredService, s docker.ContainerState, node string) appContainerResource {
	c := appContainerResource{
		ID: shortContainerID(s.ID), Name: s.Name, Image: s.Image, ImageDigest: s.ImageID,
		State: "stopped", Node: node,
		Ports: []appContainerPort{}, Networks: []appContainerNetwork{}, Mounts: []appContainerMount{},
		Unavailable: []string{unavailableHealth},
	}
	if s.Running {
		c.State = "running"
	}
	if s.ImageID == "" {
		c.Unavailable = append(c.Unavailable, unavailableImageDigest)
	}
	ports := s.Ports
	if !s.Running {
		ports = s.ConfiguredPorts
	}
	for _, p := range ports {
		bind := p.HostIP
		if bind == "" {
			bind = "0.0.0.0"
		}
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		c.Ports = append(c.Ports, appContainerPort{Container: p.ContainerPort, Host: p.HostPort, Proto: proto, Bind: bind})
	}
	for _, n := range s.Networks {
		c.Networks = append(c.Networks, appContainerNetwork{Name: n.Name, IP: n.IPAddress, Aliases: []string{}})
	}
	if len(s.Networks) == 0 {
		c.Unavailable = append(c.Unavailable, unavailableNetworks)
	} else {
		c.Unavailable = append(c.Unavailable, unavailableAliases)
	}
	c.Mounts, c.Limits = declaredMountsAndLimits(svc)

	if ei, ok := lister.(docker.ExitStateInspector); ok {
		if es, err := ei.InspectExitState(ctx, s.Name); err == nil && es != nil && !es.StartedAt.IsZero() {
			rc := es.RestartCount
			c.RestartCount = &rc
			started := es.StartedAt
			c.StartedAt = &started
		}
	}
	if c.StartedAt == nil {
		c.Unavailable = append(c.Unavailable, unavailableRestartCount, unavailableStartedAt)
	}
	c.Stats = containerStats(ctx, lister, s)
	if c.Stats == nil {
		c.Unavailable = append(c.Unavailable, unavailableStats)
	}
	return c
}

// containerStats samples a running container once; nil when it is
// stopped or the transport cannot report usage.
func containerStats(ctx context.Context, lister ContainerLister, s docker.ContainerState) *appContainerStats {
	si, ok := lister.(docker.StatsInspector)
	if !ok || !s.Running {
		return nil
	}
	st, err := si.Stats(ctx, s.ID)
	if err != nil {
		return nil
	}
	return &appContainerStats{CPUPercent: st.CPUPercent, MemoryBytes: st.MemoryUsageBytes, MemoryLimitBytes: st.MemoryLimitBytes}
}

// declaredMountsAndLimits reports the configured mounts and limits: the
// agent protocol does not return live mount or HostConfig data.
func declaredMountsAndLimits(svc *store.DesiredService) ([]appContainerMount, appContainerLimits) {
	mounts := []appContainerMount{}
	for _, v := range svc.Volumes {
		mounts = append(mounts, appContainerMount{Type: "volume", Source: v.Name, Target: v.ContainerPath})
	}
	for _, b := range svc.BindMounts {
		mounts = append(mounts, appContainerMount{Type: "bind", Source: b.HostPath, Target: b.ContainerPath, RO: b.ReadOnly})
	}
	var limits appContainerLimits
	if svc.Resources != nil {
		limits.CPU = float64(svc.Resources.NanoCPUs) / 1e9
		limits.MemoryBytes = svc.Resources.MemoryBytes
	}
	return mounts, limits
}

func shortContainerID(id string) string {
	if len(id) > shortContainerIDLen {
		return id[:shortContainerIDLen]
	}
	return id
}
