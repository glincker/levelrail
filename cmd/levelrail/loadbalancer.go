package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// lbNodeUpstreams resolves a remote node's runtime and the host its
// published ports are reachable on (mesh address first, then its own address).
type lbNodeUpstreams struct {
	db          *store.DB
	local       docker.Runtime
	registry    *agent.Registry
	localNodeID string
}

func (r lbNodeUpstreams) UpstreamHost(ctx context.Context, nodeID string) (docker.Runtime, string, error) {
	if r.localNodeID != "" && nodeID == r.localNodeID {
		return r.local, "127.0.0.1", nil
	}
	rt, err := resolveNodeTransport(r.local, r.registry, nodeID)
	if err != nil {
		return nil, "", fmt.Errorf("resolve node %q runtime: %w", nodeID, err)
	}
	node, err := r.db.GetNode(ctx, nodeID)
	if err != nil {
		return nil, "", fmt.Errorf("load node %q: %w", nodeID, err)
	}
	host := node.MeshAddress
	if host == "" {
		host = node.Address
		if h, _, splitErr := net.SplitHostPort(host); splitErr == nil {
			host = h
		}
	}
	if host == "" {
		return nil, "", fmt.Errorf("node %q has no mesh or public address", nodeID)
	}
	return rt, host, nil
}

func lbTelemetrySamples(ctx context.Context, reg *loadbalancer.Registry, stats loadbalancer.StatsSource, prober loadbalancer.Prober) []telemetry.LoadBalancerSample {
	obs := reg.List()
	if len(obs) == 0 {
		return nil
	}
	var counters map[string]loadbalancer.Stats
	if s, err := stats.UpstreamStats(ctx); err == nil {
		counters = s
	}
	out := make([]telemetry.LoadBalancerSample, 0, len(obs))
	for _, o := range obs {
		st := loadbalancer.BuildStatus(ctx, o, counters, prober)
		reg.RecordStatus(&st)
		sample := telemetry.LoadBalancerSample{Service: o.Service, Total: len(st.Upstreams)}
		for _, u := range st.Upstreams {
			if u.Healthy {
				sample.Healthy++
			}
			sample.ActiveRequests += u.ActiveConns
		}
		out = append(out, sample)
	}
	return out
}

// runLoadBalancerTelemetry samples every balancer's health and load into the
// metrics store until ctx is cancelled.
func runLoadBalancerTelemetry(ctx context.Context, reg *loadbalancer.Registry, stats loadbalancer.StatsSource, db *telemetry.DB, every time.Duration, logger *slog.Logger) {
	loop := footprint.Track("load balancer telemetry", every)
	defer loop.Active()()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			start := time.Now()
			err := db.RecordLoadBalancer(ctx, lbTelemetrySamples(ctx, reg, stats, loadbalancer.HTTPProber{}), now)
			loop.Observe(start, err)
			if err != nil && ctx.Err() == nil {
				logger.Warn("load balancer telemetry sample failed", slog.String("error", err.Error()))
			}
		}
	}
}

// lbTrafficSource exposes the ingress per-replica counters to the balancer
// registry, so the status route and telemetry loop share one collection point.
func lbTrafficSource() loadbalancer.TrafficSource {
	return func() map[string]loadbalancer.TrafficTotals {
		totals := ingress.DefaultRequestStats().UpstreamTotals()
		out := make(map[string]loadbalancer.TrafficTotals, len(totals))
		for dial, st := range totals {
			// Connect failures surface as 502/503/504, so the larger count avoids double counting.
			out[dial] = loadbalancer.TrafficTotals{Requests: st.Requests, Errors: max(st.Errors5xx, st.UpstreamErrors), Latency: st.Latency[:]}
		}
		return out
	}
}
