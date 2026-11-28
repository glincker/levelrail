package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/build"
	"github.com/glincker/levelrail/internal/store"
)

const (
	// builderRecentRuns is how many recent builds the read model shows and averages.
	builderRecentRuns         = 10
	envBuilderTestTimeout     = "APP_BUILDER_TEST_TIMEOUT"
	defaultBuilderTestTimeout = 2 * time.Minute
)

// BuilderProber runs a hermetic probe build on one node.
type BuilderProber func(ctx context.Context, nodeID string) (build.ProbeResult, error)

// SetBuilderProber wires the probe behind POST /api/v1/builders/{id}/test.
func (rt *Router) SetBuilderProber(p BuilderProber) { rt.builderProber = p }

// builderResource is one build node as the builders list and the dashboard show it.
type builderResource struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Status        string     `json:"status"`
	Paused        bool       `json:"paused"`
	Tier          string     `json:"tier"`
	OS            string     `json:"os,omitempty"`
	Arch          string     `json:"arch,omitempty"`
	AgentVersion  string     `json:"agent_version,omitempty"`
	CPUCores      int        `json:"cpu_cores,omitempty"`
	Pressure      string     `json:"pressure"`
	SlotsUsed     int        `json:"slots_used"`
	SlotsTotal    int        `json:"slots_total"`
	ActiveBuilds  int        `json:"active_builds"`
	Waiting       int        `json:"waiting"`
	AcceptsApps   bool       `json:"accepts_app_workloads"`
	Labels        []string   `json:"labels"`
	LastSeenAt    *time.Time `json:"last_seen_at,omitempty"`
	LastBuildAt   *time.Time `json:"last_build_at,omitempty"`
	RecentBuilds  int        `json:"recent_builds"`
	RecentFailed  int        `json:"recent_failed"`
	AvgBuildMS    int64      `json:"avg_build_ms"`
	DiskFreeBytes int64      `json:"disk_free_bytes,omitempty"`
}

type builderBuildRow struct {
	App        string    `json:"app"`
	Tag        string    `json:"tag"`
	StartedAt  time.Time `json:"started_at"`
	DurationMS int64     `json:"duration_ms"`
	Result     string    `json:"result"`
}

type builderDetailResource struct {
	builderResource
	Builds []builderBuildRow `json:"builds"`
}

type buildersResponse struct {
	Builders []builderResource `json:"builders"`
}

type builderTestResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	OK            bool   `json:"ok"`
	LatencyMS     int64  `json:"latency_ms"`
	Arch          string `json:"arch,omitempty"`
	OS            string `json:"os,omitempty"`
	AgentVersion  string `json:"agent_version,omitempty"`
	BuildKit      string `json:"buildkit_version,omitempty"`
	DiskFreeBytes int64  `json:"disk_free_bytes,omitempty"`
	Error         string `json:"error,omitempty"`
}

// builderViews joins build routing facts, node records and recent build history.
func (rt *Router) builderViews(ctx context.Context) []builderDetailResource {
	byID := map[string]store.Node{}
	if nodes, err := rt.nodes.ListNodes(ctx); err == nil {
		for _, n := range nodes {
			byID[n.ID] = n
		}
	}
	var runs []store.BuildRun
	if rt.buildPerf != nil {
		if got, err := rt.buildPerf.ListAllBuildRuns(ctx, buildCenterMaxLimit); err == nil {
			runs = got
		}
	}
	out := []builderDetailResource{}
	for _, n := range rt.buildRunnerList(ctx) {
		if n.Local {
			continue
		}
		out = append(out, builderView(n, byID[n.ID], runs))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func builderView(n build.NodeInfo, rec store.Node, runs []store.BuildRun) builderDetailResource {
	status, _ := runnerStatus(n)
	if n.Draining {
		status = "paused"
	}
	tier := n.Trust
	if tier == "" {
		tier = build.TrustServer
	}
	d := builderDetailResource{builderResource: builderResource{
		ID: n.ID, Name: n.Name, Status: status, Paused: n.Draining, Tier: tier,
		OS: rec.AgentOS, Arch: rec.AgentArch, AgentVersion: rec.AgentVersion, CPUCores: n.CPUCores, Pressure: n.Pressure,
		SlotsUsed: n.SlotsUsed, SlotsTotal: n.SlotsTotal, ActiveBuilds: n.SlotsUsed, Waiting: n.Waiting,
		AcceptsApps: rec.AcceptsAppWorkloads, Labels: labelStrings(n.Labels), DiskFreeBytes: n.DiskFreeBytes,
	}, Builds: []builderBuildRow{}}
	if d.Name == "" {
		d.Name = n.ID
	}
	if d.Pressure == "" {
		d.Pressure = build.PressureUnknown
	}
	if d.OS == "" {
		if i := strings.IndexByte(n.Arch, '/'); i > 0 {
			d.OS, d.Arch = n.Arch[:i], n.Arch[i+1:]
		}
	}
	if !n.LastSeen.IsZero() {
		t := n.LastSeen
		d.LastSeenAt = &t
	}
	var total, ok int64
	for _, run := range runs {
		if run.NodeID != n.ID || len(d.Builds) >= builderRecentRuns {
			continue
		}
		res := "success"
		if run.Status == "failed" {
			res = "failed"
			d.RecentFailed++
		} else {
			total += run.DurationMS
			ok++
		}
		d.Builds = append(d.Builds, builderBuildRow{App: run.App, Tag: run.Tag, StartedAt: run.StartedAt, DurationMS: run.DurationMS, Result: res})
		if d.LastBuildAt == nil {
			t := run.StartedAt.Add(time.Duration(run.DurationMS) * time.Millisecond)
			d.LastBuildAt = &t
		}
	}
	d.RecentBuilds = len(d.Builds)
	if ok > 0 {
		d.AvgBuildMS = total / ok
	}
	return d
}

// handleListBuilders handles GET /api/v1/builders.
func (rt *Router) handleListBuilders(w http.ResponseWriter, r *http.Request) {
	views := rt.builderViews(r.Context())
	resp := buildersResponse{Builders: make([]builderResource, 0, len(views))}
	for _, v := range views {
		resp.Builders = append(resp.Builders, v.builderResource)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (rt *Router) findBuilder(w http.ResponseWriter, r *http.Request) (builderDetailResource, bool) {
	ref := r.PathValue("id")
	for _, v := range rt.builderViews(r.Context()) {
		if v.ID == ref || v.Name == ref {
			return v, true
		}
	}
	writeError(w, http.StatusNotFound, "builder not found")
	return builderDetailResource{}, false
}

// handleGetBuilder handles GET /api/v1/builders/{id}; id may also be the builder name.
func (rt *Router) handleGetBuilder(w http.ResponseWriter, r *http.Request) {
	if v, ok := rt.findBuilder(w, r); ok {
		writeJSON(w, http.StatusOK, v)
	}
}

func (rt *Router) handlePauseBuilder(w http.ResponseWriter, r *http.Request) {
	rt.setBuilderPaused(w, r, true)
}
func (rt *Router) handleResumeBuilder(w http.ResponseWriter, r *http.Request) {
	rt.setBuilderPaused(w, r, false)
}

// setBuilderPaused reuses the node cordon: placement skips it for new builds while running ones finish.
func (rt *Router) setBuilderPaused(w http.ResponseWriter, r *http.Request, paused bool) {
	v, ok := rt.findBuilder(w, r)
	if !ok {
		return
	}
	if v.AcceptsApps {
		writeError(w, http.StatusConflict, "this node also runs apps; use nodes cordon to stop new work on it")
		return
	}
	if err := rt.nodes.SetNodeSchedulable(r.Context(), v.ID, !paused); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "builder not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: pause builder failed", err, slog.String("node_id", v.ID))
		return
	}
	rt.changes.Publish(changeNodes, v.ID)
	v.Paused = paused
	v.Status = "ready"
	if paused {
		v.Status = "paused"
	}
	writeJSON(w, http.StatusOK, v.builderResource)
}

// handleTestBuilder handles POST /api/v1/builders/{id}/test with a probe build that carries no app data.
func (rt *Router) handleTestBuilder(w http.ResponseWriter, r *http.Request) {
	v, ok := rt.findBuilder(w, r)
	if !ok {
		return
	}
	if rt.builderProber == nil {
		writeError(w, http.StatusNotImplemented, "builder tests are not available on this control plane")
		return
	}
	res := builderTestResponse{ID: v.ID, Name: v.Name, OS: v.OS, Arch: v.Arch, AgentVersion: v.AgentVersion, DiskFreeBytes: v.DiskFreeBytes}
	if v.Status == "offline" {
		res.Error = "builder is offline; run builder doctor on that machine"
		writeJSON(w, http.StatusOK, res)
		return
	}
	pr, err := rt.builderProber(r.Context(), v.ID)
	if err != nil {
		rt.internalError(w, "api: builder test failed", err, slog.String("node_id", v.ID))
		return
	}
	res.OK, res.LatencyMS, res.BuildKit = pr.OK, pr.Latency.Milliseconds(), pr.BuildKit
	if !pr.OK {
		res.Error = pr.Error
	}
	writeJSON(w, http.StatusOK, res)
}

// builderTestTimeout is the probe deadline, seeded from the environment.
func builderTestTimeout() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(envBuilderTestTimeout)); err == nil && d > 0 {
		return d
	}
	return defaultBuilderTestTimeout
}

// NewBuilderProber probes through remote with the configured timeout.
func NewBuilderProber(remote build.NodeBuilder) BuilderProber {
	return func(ctx context.Context, nodeID string) (build.ProbeResult, error) {
		return build.ProbeNode(ctx, remote, nodeID, builderTestTimeout())
	}
}
