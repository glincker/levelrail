package api

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"runtime"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/nodeinsight"
	"github.com/GLINCKER/levelrail/internal/nodepreflight"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
)

const (
	defaultTimelineWindow = 24 * time.Hour
	maxTimelineWindow     = 30 * 24 * time.Hour
	timelineEventCap      = 500
)

type nodeDoctorResponse struct {
	NodeID   string                `json:"node_id"`
	Reported bool                  `json:"reported"`
	Checks   []nodepreflight.Check `json:"checks"`
	Summary  nodepreflight.Summary `json:"summary"`
}

// handleNodeDoctor handles GET /api/v1/nodes/{id}/doctor: the agent's own
// preflight checks plus certificate, heartbeat and version checks in one list.
func (rt *Router) handleNodeDoctor(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := rt.nodes.GetNode(r.Context(), id)
	if errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: node doctor: load failed", err, slog.String("node_id", id))
		return
	}
	now := time.Now()
	out := nodeDoctorResponse{NodeID: id, Checks: []nodepreflight.Check{}}
	if rt.nodeOnb != nil {
		if st, ok := rt.nodeOnb.src.Get(id); ok {
			out.Reported = true
			out.Checks = append(out.Checks, nodepreflight.Evaluate(st.Report.Facts, st.ReceivedAt, rt.nodeOnb.cfg)...)
		}
	}
	facts := nodeinsight.DoctorFacts{
		Online: n.Status == store.NodeStatusOnline, LastSeen: n.LastSeenAt, AgentVersion: n.AgentVersion,
		ControlPlane: version.Version, Schedulable: n.Schedulable, LocalControlRow: rt.isLocalNodeRecord(n),
	}
	warning, critical := rt.nodeCerts.thresholds()
	state, left := alerting.ClassifyNodeCert(*n, warning, critical, now)
	facts.CertState = string(state)
	if n.CertNotAfter != nil && state != alerting.NodeCertRevoked {
		days := int(math.Floor(left.Hours() / 24))
		facts.CertDaysLeft = &days
	}
	out.Checks = append(out.Checks, nodeinsight.ExtraChecks(facts, now)...)
	out.Summary = nodepreflight.Summarize(out.Checks)
	writeJSON(w, http.StatusOK, out)
}

// handleNodeTimeline handles GET /api/v1/nodes/{id}/timeline?window=24h: the
// node's state over time as contiguous segments plus an uptime figure.
func (rt *Router) handleNodeTimeline(w http.ResponseWriter, r *http.Request) {
	lister, ok := rt.nodes.(nodeStatusEventLister)
	if !ok {
		writeError(w, http.StatusNotImplemented, "node status history is not available on this control plane")
		return
	}
	window := defaultTimelineWindow
	if raw := r.URL.Query().Get("window"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < time.Minute || d > maxTimelineWindow {
			writeError(w, http.StatusBadRequest, "window must be a duration between 1m and 720h")
			return
		}
		window = d
	}
	id := r.PathValue("id")
	n, err := rt.nodes.GetNode(r.Context(), id)
	if errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: node timeline: load failed", err, slog.String("node_id", id))
		return
	}
	events, err := lister.ListNodeStatusEvents(r.Context(), id, timelineEventCap)
	if err != nil {
		rt.internalError(w, "api: node timeline failed", err, slog.String("node_id", id))
		return
	}
	in := make([]nodeinsight.Event, 0, len(events))
	for _, e := range events {
		in = append(in, nodeinsight.Event{From: string(e.FromStatus), To: string(e.ToStatus), At: e.CreatedAt})
	}
	writeJSON(w, http.StatusOK, nodeinsight.BuildTimeline(in, string(n.Status), window, time.Now()))
}

// handleNodeVersionSkew handles GET /api/v1/nodes/version-skew: every agent
// against the control plane version, with the suggested upgrade order.
func (rt *Router) handleNodeVersionSkew(w http.ResponseWriter, r *http.Request) {
	nodes, err := rt.nodes.ListNodes(r.Context())
	if err != nil {
		rt.internalError(w, "api: node version skew: list failed", err)
		return
	}
	in := make([]nodeinsight.NodeVersion, 0, len(nodes))
	for i := range nodes {
		n := nodes[i]
		local := rt.isLocalNodeRecord(&n)
		v, os, arch := n.AgentVersion, n.AgentOS, n.AgentArch
		if local {
			// The control plane's own node has no agent row version, but it runs this build.
			v, os, arch = version.Version, orDefault(os, runtime.GOOS), orDefault(arch, runtime.GOARCH)
		}
		in = append(in, nodeinsight.NodeVersion{
			ID: n.ID, Name: n.Name, Version: v, OS: os, Arch: arch,
			Online: n.Status == store.NodeStatusOnline, Schedulable: n.Schedulable, Local: local,
		})
	}
	report := nodeinsight.BuildSkew(version.Version, in)
	if rt.runtimeSettings != nil {
		report.AutoUpdate = rt.runtimeSettings.Bool(r.Context(), runtimesettings.KeyAgentAutoUpdate)
	}
	writeJSON(w, http.StatusOK, report)
}

// handleNodeAgentUpdate handles GET /api/v1/nodes/{id}/agent-update: what the
// agent should run, following the control plane channel. It names a version
// only; the agent fetches and verifies the release itself.
func (rt *Router) handleNodeAgentUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := rt.nodes.GetNode(r.Context(), id)
	if errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: node agent update: load failed", err, slog.String("node_id", id))
		return
	}
	writeJSON(w, http.StatusOK, rt.DecideAgentUpdate(r.Context(), n))
}

// DecideAgentUpdate is the one update decision for a node, used by the REST
// route and by the agent listener's node-scoped UpdateCheck.
func (rt *Router) DecideAgentUpdate(ctx context.Context, n *store.Node) nodeinsight.UpdateDecision {
	facts := nodeinsight.UpdateFacts{
		Channel: rt.currentUpdateChannel(ctx), ControlPlane: version.Version, AgentVersion: n.AgentVersion,
		AgentOnline: n.Status == store.NodeStatusOnline,
	}
	if rt.isLocalNodeRecord(n) {
		facts.AgentVersion = version.Version
	}
	vh := rt.agentVersions.Observe(n.ID, facts.AgentVersion, time.Now())
	facts.PreviousGood = vh.LastGood
	if lister, ok := rt.nodes.(nodeStatusEventLister); ok {
		if events, err := lister.ListNodeStatusEvents(ctx, n.ID, timelineEventCap); err == nil {
			in := make([]nodeinsight.Event, 0, len(events))
			for _, e := range events {
				in = append(in, nodeinsight.Event{From: string(e.FromStatus), To: string(e.ToStatus), At: e.CreatedAt})
			}
			facts.HealthFailures = nodeinsight.OfflineSince(in, vh.Since)
		}
	}
	auto := false
	if rt.runtimeSettings != nil {
		facts.MaxHealthFailures = rt.runtimeSettings.Int(ctx, runtimesettings.KeyAgentUpdateHealthFailed)
		auto = rt.runtimeSettings.Bool(ctx, runtimesettings.KeyAgentAutoUpdate)
	}
	d := nodeinsight.DecideUpdate(facts)
	d.AutoApply = auto
	return d
}
