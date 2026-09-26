package ingress

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// LoadBalancerSource lists every service's raw JSON load balancer config.
type LoadBalancerSource interface {
	ListServiceLoadBalancers(ctx context.Context) (map[string]string, error)
}

// NodeUpstreamResolver returns the runtime for a remote node and the host
// the control plane dials to reach its published ports.
type NodeUpstreamResolver interface {
	UpstreamHost(ctx context.Context, nodeID string) (docker.Runtime, string, error)
}

const localUpstreamHost = "127.0.0.1"

// DefaultAdminListen is where this controller binds Caddy's admin API unless
// WithAdminListen overrides it.
const DefaultAdminListen = defaultAdminListen

// WithLoadBalancers turns on per-service load balancing. reg receives every
// applied observation and may be shared with the API.
func WithLoadBalancers(src LoadBalancerSource, reg *loadbalancer.Registry) Option {
	return func(c *Controller) {
		c.lbSource = src
		if reg != nil {
			c.lbRegistry = reg
		}
	}
}

// WithNodeUpstreams lets balancers include replicas that run on remote nodes.
func WithNodeUpstreams(r NodeUpstreamResolver) Option {
	return func(c *Controller) { c.nodeUpstreams = r }
}

func (c *Controller) loadBalancerConfigs(ctx context.Context) (map[string]loadbalancer.Config, error) {
	if c.lbSource == nil {
		return nil, nil
	}
	raw, err := c.lbSource.ListServiceLoadBalancers(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]loadbalancer.Config, len(raw))
	for name, doc := range raw {
		var cfg loadbalancer.Config
		if err := json.Unmarshal([]byte(doc), &cfg); err != nil {
			c.logger.WarnContext(ctx, "ingress: skipping unparseable load balancer config", slog.String("service", name), slog.String("error", err.Error()))
			continue
		}
		if err := cfg.Validate(); err != nil {
			c.logger.WarnContext(ctx, "ingress: skipping invalid load balancer config", slog.String("service", name), slog.String("error", err.Error()))
			continue
		}
		out[name] = cfg
	}
	return out, nil
}

// lbPlan is one service's balancer for this pass.
type lbPlan struct {
	route *ingress.LBRoute
	obs   loadbalancer.Observation
}

// planLoadBalancer discovers upstreams for svc and builds its route. The
// route is nil when no upstream is usable.
func (c *Controller) planLoadBalancer(ctx context.Context, svc store.DesiredService, cfg loadbalancer.Config, now time.Time) lbPlan {
	ups := c.discoverUpstreams(ctx, svc)
	var pool []loadbalancer.Upstream
	draining := false
	for _, u := range ups {
		if u.Running {
			pool = append(pool, u.Upstream)
			draining = draining || u.Draining
		}
	}

	ids := make([]string, len(pool))
	for i, u := range pool {
		ids[i] = u.ID
	}
	firstSeen := c.lbRegistry.FirstSeen(svc.Name, ids, now)
	weights := loadbalancer.EffectiveWeights(cfg, pool, firstSeen, now)
	if cfg.EffectiveAlgorithm() == loadbalancer.AlgoWeighted {
		byID := make(map[string]int, len(pool))
		for i, u := range pool {
			byID[u.ID] = weights[i]
		}
		for i := range ups {
			ups[i].Weight = byID[ups[i].ID]
		}
	}

	want := svc.Replicas
	if want <= 0 {
		want = store.DefaultReplicas
	}
	obs := loadbalancer.Observation{Service: svc.Name, Config: cfg, Upstreams: ups, ObservedAt: now}
	switch {
	case len(pool) == 0:
		obs.Reason, obs.Message = "NoUpstreams", "no running replica with a reachable published port"
	case draining:
		obs.Reason, obs.Message = "ServingPreviousRelease", fmt.Sprintf("serving %d container(s) of the previous release until the new one is running", len(pool))
	case len(pool) < want:
		obs.Reason, obs.Message = "UpstreamsDegraded", fmt.Sprintf("%d of %d replicas are in the pool", len(pool), want)
	default:
		obs.Reason, obs.Message, obs.Ready = "Balancing", fmt.Sprintf("balancing across %d upstream(s)", len(pool)), true
	}
	return lbPlan{route: ingress.NewLBRoute(cfg, pool, weights), obs: obs}
}

func (c *Controller) discoverUpstreams(ctx context.Context, svc store.DesiredService) []loadbalancer.UpstreamObservation {
	rt, host := c.runtime, localUpstreamHost
	if svc.NodeID != "" {
		if c.nodeUpstreams == nil {
			return nil
		}
		remote, h, err := c.nodeUpstreams.UpstreamHost(ctx, svc.NodeID)
		if err != nil {
			c.logger.WarnContext(ctx, "ingress: resolving node for load balancer upstreams failed", slog.String("service", svc.Name), slog.String("node", svc.NodeID), slog.String("error", err.Error()))
			return nil
		}
		rt, host = remote, h
	}

	replicas := svc.Replicas
	if replicas <= 0 {
		replicas = store.DefaultReplicas
	}
	current := make(map[string]bool, replicas)
	var out []loadbalancer.UpstreamObservation
	for i := 0; i < replicas; i++ {
		name := application.ReplicaContainerName(svc.Name, application.NameImage(svc), svc.RestartNonce, i)
		current[name] = true
		u := loadbalancer.UpstreamObservation{Upstream: loadbalancer.Upstream{ID: loadbalancer.UpstreamID(svc.Name, i), Replica: i, NodeID: svc.NodeID}}
		state, err := rt.InspectByName(ctx, name)
		switch {
		case err != nil:
			c.logger.WarnContext(ctx, "ingress: inspecting replica for load balancer failed", slog.String("service", svc.Name), slog.String("container", name), slog.String("error", err.Error()))
			u.Note = "inspect failed"
		case state == nil:
			u.Note = "not created yet"
		default:
			u.Dial, u.Running, u.Note = upstreamDial(state, host, svc.NodeID != "")
		}
		out = append(out, u)
	}

	if anyRunning(out) {
		return out
	}
	return append(out, c.previousReleaseUpstreams(ctx, rt, host, svc, current, len(out))...)
}

func anyRunning(ups []loadbalancer.UpstreamObservation) bool {
	for _, u := range ups {
		if u.Running {
			return true
		}
	}
	return false
}

func upstreamDial(state *docker.ContainerState, host string, remote bool) (dial string, running bool, note string) {
	if !state.Running {
		return "", false, "container not running"
	}
	if len(state.Ports) == 0 {
		return "", false, "no published port"
	}
	p := state.Ports[0]
	if remote && isLoopback(p.HostIP) {
		return "", false, "published on loopback, set bind_address to public to balance across nodes"
	}
	return net.JoinHostPort(host, strconv.Itoa(p.HostPort)), true, ""
}

func isLoopback(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.IsLoopback()
}

func (c *Controller) previousReleaseUpstreams(ctx context.Context, rt docker.Runtime, host string, svc store.DesiredService, current map[string]bool, firstIndex int) []loadbalancer.UpstreamObservation {
	states, err := rt.ListByPrefix(ctx, svc.Name+"-")
	if err != nil {
		c.logger.WarnContext(ctx, "ingress: listing previous release containers failed", slog.String("service", svc.Name), slog.String("error", err.Error()))
		return nil
	}
	sort.Slice(states, func(i, j int) bool { return states[i].Name < states[j].Name })
	var out []loadbalancer.UpstreamObservation
	for _, s := range states {
		st := s
		if current[st.Name] || !application.OwnsContainer(svc.Name, st.Name) {
			continue
		}
		dial, running, _ := upstreamDial(&st, host, svc.NodeID != "")
		if !running {
			continue
		}
		idx := firstIndex + len(out)
		out = append(out, loadbalancer.UpstreamObservation{
			Upstream: loadbalancer.Upstream{ID: loadbalancer.UpstreamID(svc.Name, idx), Dial: dial, Replica: idx, NodeID: svc.NodeID},
			Running:  true, Draining: true, Note: "previous release",
		})
	}
	return out
}

func (p lbPlan) severity() int {
	switch {
	case p.route == nil:
		return 3
	case p.obs.Reason == "UpstreamsDegraded":
		return 2
	case !p.obs.Ready:
		return 1
	}
	return 0
}

// lbCondition summarises every balanced service in one condition, or nil
// when none is configured. The most severe service decides the reason.
func lbCondition(plans []lbPlan) *reconcile.Condition {
	if len(plans) == 0 {
		return nil
	}
	worst := plans[0]
	for _, p := range plans[1:] {
		if p.severity() > worst.severity() {
			worst = p
		}
	}
	if worst.severity() > 0 {
		return &reconcile.Condition{Type: "LoadBalancer", Status: reconcile.ConditionFalse, Reason: worst.obs.Reason, Message: worst.obs.Service + ": " + worst.obs.Message}
	}
	return &reconcile.Condition{Type: "LoadBalancer", Status: reconcile.ConditionTrue, Reason: "Balancing", Message: fmt.Sprintf("%d service(s) balanced", len(plans))}
}

func (c *Controller) recordLoadBalancers(plans []lbPlan, configured map[string]loadbalancer.Config) {
	keep := make(map[string]bool, len(configured))
	for name := range configured {
		keep[name] = true
	}
	c.lbRegistry.Retain(keep)
	for _, p := range plans {
		c.lbRegistry.Record(p.obs)
	}
}
