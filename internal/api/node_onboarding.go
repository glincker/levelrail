package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/nodepreflight"
	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envOnboardingSyncInterval     = "APP_NODE_ONBOARDING_SYNC_INTERVAL"
	defaultOnboardingSyncInterval = 10 * time.Second
)

// Onboarding stages, in the order a joining node moves through them.
const (
	OnboardingStageWaiting        = "waiting_for_agent"
	OnboardingStageConnected      = "connected"
	OnboardingStageReady          = "ready"
	OnboardingStageNeedsAttention = "needs_attention"
)

// NodeOnboardingSource is the control plane's latest onboarding report per
// node; *agent.OnboardingStore satisfies it.
type NodeOnboardingSource interface {
	Get(nodeID string) (agent.StoredOnboarding, bool)
	Snapshot() map[string]agent.StoredOnboarding
}

type nodeOnboardingState struct {
	src    NodeOnboardingSource
	cfg    nodepreflight.Config
	now    func() time.Time
	mu     sync.Mutex
	synced map[string]string
}

// SetNodeOnboarding wires agent onboarding reports into the preflight and
// failure-domain auto-detect routes. Without it those routes report that no
// agent has sent facts yet.
func (rt *Router) SetNodeOnboarding(src NodeOnboardingSource, cfg nodepreflight.Config) {
	rt.nodeOnb = &nodeOnboardingState{src: src, cfg: cfg, now: time.Now, synced: map[string]string{}}
}

type detectedDomain struct {
	Provider         string `json:"provider,omitempty"`
	Region           string `json:"region,omitempty"`
	Zone             string `json:"zone,omitempty"`
	InstanceID       string `json:"instance_id,omitempty"`
	Source           string `json:"source,omitempty"`
	MetadataDisabled bool   `json:"metadata_disabled,omitempty"`
}

type nodeOnboardingResponse struct {
	NodeID     string                `json:"node_id"`
	Stage      string                `json:"stage"`
	Reported   bool                  `json:"reported"`
	Stale      bool                  `json:"stale"`
	ReportedAt *time.Time            `json:"reported_at,omitempty"`
	Detected   *detectedDomain       `json:"detected,omitempty"`
	Domain     detectedDomain        `json:"domain"`
	Checks     []nodepreflight.Check `json:"checks"`
	Summary    nodepreflight.Summary `json:"summary"`
}

// handleGetNodeOnboarding handles GET /api/v1/nodes/{id}/onboarding: the
// node's detected provider, region and zone and its preflight checks, each
// failure with a concrete fix.
func (rt *Router) handleGetNodeOnboarding(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := rt.nodes.GetNode(r.Context(), id)
	if errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: node onboarding: load failed", err, slog.String("node_id", id))
		return
	}
	out := nodeOnboardingResponse{
		NodeID: id, Checks: []nodepreflight.Check{},
		Domain: detectedDomain{Provider: n.Provider, Region: n.Region, Zone: n.Zone},
	}
	var st agent.StoredOnboarding
	reported := false
	if rt.nodeOnb != nil {
		st, reported = rt.nodeOnb.src.Get(id)
	}
	switch {
	case !reported && n.Status == store.NodeStatusPending:
		out.Stage = OnboardingStageWaiting
	case !reported:
		out.Stage = OnboardingStageConnected
	default:
		out.Reported = true
		at := st.ReceivedAt
		out.ReportedAt = &at
		out.Stale = st.Stale(rt.nodeOnb.now())
		c := st.Report.Cloud
		out.Detected = &detectedDomain{Provider: c.Provider, Region: c.Region, Zone: c.Zone, InstanceID: c.InstanceID, Source: c.Source, MetadataDisabled: st.Report.Disabled}
		out.Checks = nodepreflight.Evaluate(st.Report.Facts, st.ReceivedAt, rt.nodeOnb.cfg)
		out.Summary = nodepreflight.Summarize(out.Checks)
		out.Stage = OnboardingStageReady
		if !out.Summary.Ready {
			out.Stage = OnboardingStageNeedsAttention
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// RunNodeOnboarding applies each node's detected provider, region and zone
// until ctx is done. It only fills values the operator has not set, so a
// manual label always wins, and it re-checks every tick so a node that
// enrolled before its first report is still labeled.
func (rt *Router) RunNodeOnboarding(ctx context.Context) {
	if rt.nodeOnb == nil {
		return
	}
	t := time.NewTicker(envDurationOr(envOnboardingSyncInterval, defaultOnboardingSyncInterval))
	defer t.Stop()
	for {
		rt.syncDetectedDomains(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (rt *Router) syncDetectedDomains(ctx context.Context) {
	o := rt.nodeOnb
	for id, st := range o.src.Snapshot() {
		if st.Stale(o.now()) {
			continue
		}
		c := st.Report.Cloud
		sig := c.Provider + "|" + c.Region + "|" + c.Zone
		o.mu.Lock()
		same := o.synced[id] == sig
		o.mu.Unlock()
		if same || !c.Found() {
			continue
		}
		if rt.fillDetectedDomain(ctx, id, c.Provider, c.Region, c.Zone) {
			o.mu.Lock()
			o.synced[id] = sig
			o.mu.Unlock()
		}
	}
}

// fillDetectedDomain returns true when the node row is settled: updated, or
// already holding every value it would receive, or no longer registered.
func (rt *Router) fillDetectedDomain(ctx context.Context, nodeID, provider, region, zone string) bool {
	n, err := rt.nodes.GetNode(ctx, nodeID)
	if errors.Is(err, store.ErrNodeNotFound) {
		return true
	} else if err != nil {
		rt.logger.Warn("api: node onboarding: load node failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		return false
	}
	p, r, z := firstSet(n.Provider, provider), firstSet(n.Region, region), firstSet(n.Zone, zone)
	if p == n.Provider && r == n.Region && z == n.Zone {
		return true
	}
	if err := rt.nodes.UpdateNodeFailureDomain(ctx, nodeID, p, r, z); err != nil {
		rt.logger.Warn("api: node onboarding: fill failure domain failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		return false
	}
	rt.logger.Info("api: node onboarding: applied detected failure domain", slog.String("node_id", nodeID), slog.String("provider", p), slog.String("region", r), slog.String("zone", z))
	return true
}

type nodeDomainsResponse struct {
	placement.DomainOverview
	Risks []placement.DomainRisk `json:"risks"`
}

// handleNodeDomains handles GET /api/v1/nodes/domains: reported capacity per
// provider, region and zone, plus apps whose replicas all share one domain.
func (rt *Router) handleNodeDomains(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cands, err := rt.placementCandidates(ctx)
	if err != nil {
		rt.internalError(w, "api: node domains: candidates failed", err)
		return
	}
	inputs, err := rt.riskInputs(ctx)
	if err != nil {
		rt.internalError(w, "api: node domains: workloads failed", err)
		return
	}
	risks := placement.DomainRisks(cands, inputs)
	if risks == nil {
		risks = []placement.DomainRisk{}
	}
	writeJSON(w, http.StatusOK, nodeDomainsResponse{DomainOverview: placement.Domains(cands), Risks: risks})
}

func (rt *Router) riskInputs(ctx context.Context) ([]placement.RiskInput, error) {
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]placement.RiskInput, 0, len(services))
	for _, s := range services {
		sel, err := rt.selectorFor(ctx, store.NodeMoveKindApp, s.Name)
		if err != nil {
			return nil, err
		}
		node := s.NodeID
		if node == "" {
			node = rt.localNodeID
		}
		out = append(out, placement.RiskInput{Name: s.Name, NodeID: node, Replicas: s.Replicas, Selector: sel})
	}
	return out, nil
}

// appDomainRisks is the failure-domain warnings for one app.
func (rt *Router) appDomainRisks(ctx context.Context, name string, cands []placement.Candidate) ([]placement.DomainRisk, error) {
	inputs, err := rt.riskInputs(ctx)
	if err != nil {
		return nil, err
	}
	for _, in := range inputs {
		if in.Name == name {
			return placement.DomainRisks(cands, []placement.RiskInput{in}), nil
		}
	}
	return nil, nil
}
