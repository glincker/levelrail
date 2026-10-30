package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/resourceadvisor"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// advisorDockerTimeout bounds the one Docker call so the advisor never
// blocks on a slow daemon; a timeout only drops the reclaimable-disk rule.
const advisorDockerTimeout = 2 * time.Second

// advisorLocalNodeID names the single server when mesh gives it no node id.
const advisorLocalNodeID = "local"

// advisorCache holds the last instance-wide analysis for a short TTL.
type advisorCache struct {
	mu       sync.Mutex
	at       time.Time
	findings []resourceadvisor.Finding
}

func newAdvisorCache() *advisorCache { return &advisorCache{} }

func (c *advisorCache) invalidate() {
	c.mu.Lock()
	c.at = time.Time{}
	c.mu.Unlock()
}

// advisorThresholds reads APP_RESOURCE_ADVISOR_* on each refresh.
func (rt *Router) advisorThresholds() resourceadvisor.Thresholds {
	th, errs := resourceadvisor.ThresholdsFromEnv(os.LookupEnv)
	for _, err := range errs {
		rt.logger.Warn("resource advisor: ignoring threshold", slog.String("error", err.Error()))
	}
	return th
}

// advisorFindings returns the cached analysis, recomputing it when stale.
// Callers filter by visibility; the cache is never per caller.
func (rt *Router) advisorFindings(ctx context.Context) ([]resourceadvisor.Finding, error) {
	c := rt.advisor
	c.mu.Lock()
	defer c.mu.Unlock()
	th := rt.advisorThresholds()
	now := time.Now()
	if !c.at.IsZero() && now.Sub(c.at) < th.CacheTTL {
		return c.findings, nil
	}
	in, err := rt.advisorInput(ctx, th, now)
	if err != nil {
		return nil, err
	}
	c.findings, c.at = resourceadvisor.Analyze(in), now
	return c.findings, nil
}

func (rt *Router) advisorNodeOf(nodeID string) string {
	if nodeID != "" {
		return nodeID
	}
	if rt.localNodeID != "" {
		return rt.localNodeID
	}
	return advisorLocalNodeID
}

func (rt *Router) advisorInput(ctx context.Context, th resourceadvisor.Thresholds, now time.Time) (resourceadvisor.Input, error) {
	in := resourceadvisor.Input{Now: now, Thresholds: th}
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return in, fmt.Errorf("resource advisor: list apps: %w", err)
	}
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return in, fmt.Errorf("resource advisor: list databases: %w", err)
	}
	dbs = rt.withoutPendingDatabases(ctx, dbs)
	routed := map[string]bool{}
	if owners, err := rt.domainOwners(ctx); err == nil {
		for _, apps := range owners {
			for _, a := range apps {
				routed[a] = true
			}
		}
	} else {
		rt.logger.Warn("resource advisor: domain owners", slog.String("error", err.Error()))
	}
	sleeping := map[string]bool{}
	if rows, err := rt.appSleep.ListAppSleep(ctx); err == nil {
		for _, a := range rows {
			sleeping[a.ServiceName] = a.IdleMinutes > 0
		}
	}
	links := databaseLinks(svcs, dbs)
	from := now.Add(-max(th.Window, th.IdleWindow))
	for _, svc := range svcs {
		r := resourceadvisor.Resource{
			Ref:    resourceadvisor.Ref{Kind: resourceadvisor.ResourceApp, Name: svc.Name},
			NodeID: rt.advisorNodeOf(svc.NodeID), Image: svc.Image, Suspended: svc.Suspended,
			ServesTraffic: routed[svc.Name], SleepEnabled: sleeping[svc.Name], LinkedApps: -1,
		}
		if svc.Resources != nil {
			r.MemoryLimit, r.NanoCPUs = svc.Resources.MemoryBytes, svc.Resources.NanoCPUs
		}
		rt.advisorSeries(ctx, &r, resourceIDForApp(svc.Name), from, now, th)
		in.Resources = append(in.Resources, r)
	}
	for _, d := range dbs {
		r := resourceadvisor.Resource{
			Ref:    resourceadvisor.Ref{Kind: resourceadvisor.ResourceDatabase, Name: d.Name},
			NodeID: rt.advisorNodeOf(d.NodeID), Engine: d.Engine, Version: d.Version, Suspended: d.Suspended,
			LinkedApps: links[d.Name],
		}
		if d.Resources != nil {
			r.MemoryLimit, r.NanoCPUs = d.Resources.MemoryBytes, d.Resources.NanoCPUs
		}
		rt.advisorSeries(ctx, &r, resourceIDForDatabase(d.Name), from, now, th)
		in.Resources = append(in.Resources, r)
	}
	in.Nodes = rt.advisorNodes(ctx, from, now)
	return in, nil
}

func toAdvisorSamples(samples []telemetry.Sample) []resourceadvisor.Sample {
	out := make([]resourceadvisor.Sample, len(samples))
	for i, s := range samples {
		out[i] = resourceadvisor.Sample{At: s.Timestamp, Value: s.Value}
	}
	return out
}

// advisorSeries fills one resource's telemetry. Each query failing only
// leaves that series empty, which the rules read as "no data".
func (rt *Router) advisorSeries(ctx context.Context, r *resourceadvisor.Resource, resourceID string, from, now time.Time, th resourceadvisor.Thresholds) {
	if rt.telemetry == nil {
		return
	}
	q := func(metric string) []resourceadvisor.Sample {
		s, err := rt.telemetry.QueryMetrics(ctx, resourceID, metric, from, now)
		if err != nil {
			rt.logger.Warn("resource advisor: query metric", slog.String("resource", resourceID), slog.String("metric", metric), slog.String("error", err.Error()))
			return nil
		}
		return toAdvisorSamples(s)
	}
	r.Memory = q("memory_usage_bytes")
	r.CPUPercent = q("cpu_percent")
	if r.Ref.Kind == resourceadvisor.ResourceApp && r.ServesTraffic {
		r.Requests = q(telemetry.MetricHTTPRequests)
	}
	for _, s := range q(telemetry.MetricContainerRestartCount) {
		if !s.At.Before(now.Add(-th.Window)) {
			r.Restarts += int(s.Value)
		}
	}
	if r.MemoryLimit > 0 {
		if oom := rt.findOOMEvidence(ctx, r.Ref.Name, resourceID, now.Add(-th.Window), now); oom != nil {
			r.OOM = &resourceadvisor.OOM{At: oom.DetectedAt, Excerpt: oom.Excerpt}
		}
	}
}

func (rt *Router) advisorNodes(ctx context.Context, from, now time.Time) []resourceadvisor.Node {
	var out []resourceadvisor.Node
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		rt.logger.Warn("resource advisor: list nodes", slog.String("error", err.Error()))
	}
	for _, n := range nodes {
		out = append(out, rt.advisorNode(ctx, resourceadvisor.Node{ID: n.ID, Name: n.Name}, from, now))
	}
	if len(out) == 0 {
		local := resourceadvisor.Node{ID: rt.advisorNodeOf(""), Name: "this server", NoIdentity: true}
		out = append(out, rt.advisorNode(ctx, local, from, now))
	}
	return out
}

func (rt *Router) advisorNode(ctx context.Context, n resourceadvisor.Node, from, now time.Time) resourceadvisor.Node {
	isLocal := n.NoIdentity || n.ID == rt.localNodeID
	if rt.telemetry != nil && !n.NoIdentity {
		id := nodeResourceID(n.ID)
		q := func(metric string) []telemetry.Sample {
			s, err := rt.telemetry.QueryMetrics(ctx, id, metric, from, now)
			if err != nil {
				rt.logger.Warn("resource advisor: node metric", slog.String("node", n.ID), slog.String("metric", metric), slog.String("error", err.Error()))
			}
			return s
		}
		if total := q(telemetry.MetricMemoryTotalBytes); len(total) > 0 {
			n.MemoryTotal = total[len(total)-1].Value
		}
		n.MemoryAvailable = toAdvisorSamples(q(telemetry.MetricMemoryAvailableBytes))
		n.SwapUsed = toAdvisorSamples(q(telemetry.MetricSwapUsedBytes))
		n.DiskUsed = toAdvisorSamples(q(telemetry.MetricDiskUsedBytes))
		if total := q(telemetry.MetricDiskTotalBytes); len(total) > 0 {
			n.DiskTotal = total[len(total)-1].Value
		}
	}
	if n.NoIdentity {
		readMem := rt.hostMemory
		if readMem == nil {
			readMem = telemetry.HostMemoryBytes
		}
		if total, avail, err := readMem(); err == nil {
			n.MemoryTotal = float64(total)
			n.MemoryAvailable = []resourceadvisor.Sample{{At: now, Value: float64(avail)}}
		}
	}
	if isLocal && rt.dockerDiskUsage != nil {
		dctx, cancel := context.WithTimeout(ctx, advisorDockerTimeout)
		u, err := rt.dockerDiskUsage.DiskUsage(dctx)
		cancel()
		if err == nil {
			n.Reclaimable = &resourceadvisor.Reclaimable{ImagesBytes: u.ImagesReclaimableBytes, BuildCacheBytes: u.BuildCacheReclaimableBytes}
		}
	}
	return n
}

// dbSecretNameRe matches secret env names that may hold a connection string
// the advisor cannot read, which makes an app's database links unknown.
var dbSecretNameRe = regexp.MustCompile(`(?i)(database|postgres|pg|mysql|maria|redis|mongo|db)`)

// databaseLinks counts the apps referencing each database through a managed
// reference, an attachment or a plain env value naming its container. An
// unreadable secret that looks like a connection string makes it unknown (-1).
func databaseLinks(svcs []store.DesiredService, dbs []store.DesiredDatabase) map[string]int {
	out := map[string]int{}
	unknown := false
	for _, svc := range svcs {
		for _, ref := range svc.SecretEnv {
			if dbSecretNameRe.MatchString(ref.Name) {
				unknown = true
			}
		}
	}
	for _, d := range dbs {
		host := databaseContainerName(d.Name)
		for _, svc := range svcs {
			if svc.DatabaseAttachment != nil && svc.DatabaseAttachment.DatabaseName == d.Name {
				out[d.Name]++
				continue
			}
			linked := false
			for _, ref := range svc.DatabaseEnv {
				if ref.Database == d.Name {
					linked = true
				}
			}
			for _, v := range svc.Env {
				if strings.Contains(v, host) {
					linked = true
				}
			}
			if linked {
				out[d.Name]++
			}
		}
		if out[d.Name] == 0 && unknown {
			out[d.Name] = -1
		}
	}
	return out
}
