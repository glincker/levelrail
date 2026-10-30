package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/cphealth"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/integrity"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envMaxLiveStreams     = "APP_MAX_LIVE_STREAMS"
	defaultMaxLiveStreams = 500
	streamRetryAfterSecs  = "5"
)

// ControlPlaneDB is the database surface the health page reads and fixes.
type ControlPlaneDB interface {
	Metrics() store.DBMetrics
	CheckpointWAL(ctx context.Context, truncate bool) error
	Optimize(ctx context.Context) error
	ResetSlowQueries()
}

// TelemetryStore is the metrics database surface the health page reads and compacts.
type TelemetryStore interface {
	FileBytes(ctx context.Context) (int64, error)
	Compact(ctx context.Context) error
}

// ControlPlaneProbe wires the health page to the running process.
type ControlPlaneProbe struct {
	DB             ControlPlaneDB
	Reconcile      func() reconcile.Stats
	ResyncInterval time.Duration
	Telemetry      TelemetryStore
	MemoryLimit    footprint.MemoryLimit
	Integrity      IntegrityProbe
	Guard          GuardProbe
	AppCount       func(ctx context.Context) int
}

// GuardProbe is the reconcile storm protection the health page reads and resets.
type GuardProbe interface {
	Status() reconcile.GuardStatus
	Reset(name string) int
}

type cpHealthState struct {
	probe   atomic.Pointer[ControlPlaneProbe]
	streams atomic.Int64
	cap     int64
}

// SetControlPlaneProbe enables GET /api/v1/system/control-plane-health.
func (rt *Router) SetControlPlaneProbe(p ControlPlaneProbe) { rt.cpHealth.probe.Store(&p) }

func (s *cpHealthState) streamCap() int64 {
	if s.cap > 0 {
		return s.cap
	}
	if n, err := strconv.Atoi(os.Getenv(envMaxLiveStreams)); err == nil && n > 0 {
		return int64(n)
	}
	return defaultMaxLiveStreams
}

// startLiveSSE is startSSE for long lived streams: it refuses new ones past the cap so a stampede cannot starve the process.
func (rt *Router) startLiveSSE(w http.ResponseWriter, r *http.Request) (http.Flusher, bool) {
	s := &rt.cpHealth
	if s.streams.Add(1) > s.streamCap() {
		s.streams.Add(-1)
		w.Header().Set("Retry-After", streamRetryAfterSecs)
		writeError(w, http.StatusServiceUnavailable, "too many live streams, retry shortly")
		return nil, false
	}
	flusher, ok := startSSE(w)
	if !ok {
		s.streams.Add(-1)
		return nil, false
	}
	go func() {
		<-r.Context().Done()
		s.streams.Add(-1)
	}()
	return flusher, true
}

type cpHealthResource struct {
	CheckedAt  time.Time            `json:"checked_at"`
	Runtime    cpRuntime            `json:"runtime"`
	DB         store.DBMetrics      `json:"db"`
	Reconcile  cpReconcile          `json:"reconcile"`
	Streams    cpStreams            `json:"streams"`
	Footprint  cpFootprint          `json:"footprint"`
	Idle       footprint.Assessment `json:"idle_footprint"`
	Thresholds cphealth.Thresholds  `json:"thresholds"`
	Findings   []cphealth.Finding   `json:"findings"`
	Status     string               `json:"status"`
}

type cpRuntime struct {
	Goroutines int     `json:"goroutines"`
	HeapBytes  uint64  `json:"heap_bytes"`
	GCPauseMS  float64 `json:"gc_pause_ms"`
	CPUs       int     `json:"cpus"`
}

type cpReconcile struct {
	Passes      int64   `json:"passes"`
	Controllers int     `json:"controllers"`
	LastPassMS  float64 `json:"last_pass_ms"`
	P50MS       float64 `json:"p50_ms"`
	P95MS       float64 `json:"p95_ms"`
	Slowest     string  `json:"slowest_controller"`
	LagSeconds  float64 `json:"lag_seconds"`
	Pending     bool    `json:"pending"`

	// Queue is set only while the concurrent worker pool runs.
	Queue *cpQueue              `json:"queue,omitempty"`
	Guard reconcile.GuardStatus `json:"guard"`
}

type cpQueue struct {
	Workers           int     `json:"workers"`
	Depth             int     `json:"queue_depth"`
	InFlight          int     `json:"in_flight"`
	OldestWaitSeconds float64 `json:"oldest_wait_seconds"`
}

type cpFootprint struct {
	RSSBytes       uint64                `json:"rss_bytes"`
	Threads        int                   `json:"threads"`
	OpenFDs        int                   `json:"open_fds"`
	CPUPercent     float64               `json:"cpu_percent"`
	TelemetryBytes int64                 `json:"telemetry_bytes"`
	MemoryLimit    footprint.MemoryLimit `json:"memory_limit"`
	Loops          []footprint.LoopInfo  `json:"loops"`
}

type cpStreams struct {
	Active int64 `json:"active"`
	Cap    int64 `json:"cap"`
}

func durMS(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func (rt *Router) cpHealthSnapshot() (cpHealthResource, bool) {
	p := rt.cpHealth.probe.Load()
	if p == nil || p.DB == nil {
		return cpHealthResource{}, false
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	now := time.Now().UTC()
	res := cpHealthResource{
		CheckedAt: now,
		Runtime: cpRuntime{
			Goroutines: runtime.NumGoroutine(), HeapBytes: ms.HeapAlloc, CPUs: runtime.NumCPU(),
			GCPauseMS: float64(ms.PauseNs[(ms.NumGC+255)%256]) / float64(time.Millisecond),
		},
		DB:      p.DB.Metrics(),
		Streams: cpStreams{Active: rt.cpHealth.streams.Load(), Cap: rt.cpHealth.streamCap()},
	}
	proc := footprint.ReadProc()
	res.Footprint = cpFootprint{
		RSSBytes: proc.RSSBytes, Threads: proc.Threads, OpenFDs: proc.OpenFDs, CPUPercent: proc.CPUPercent,
		MemoryLimit: p.MemoryLimit, Loops: footprint.Loops(),
	}
	if p.Telemetry != nil {
		if n, err := p.Telemetry.FileBytes(context.Background()); err == nil {
			res.Footprint.TelemetryBytes = n
		}
	}
	res.Idle = rt.idleAssessment(p)
	var rs reconcile.Stats
	if p.Reconcile != nil {
		rs = p.Reconcile()
		res.Reconcile = cpReconcile{
			Passes: rs.Passes, Controllers: rs.Controllers, LastPassMS: durMS(rs.LastPass),
			P50MS: durMS(rs.P50), P95MS: durMS(rs.P95), Slowest: rs.SlowestName, Pending: rs.Pending,
		}
		if q := rs.Queue; q != nil {
			res.Reconcile.Queue = &cpQueue{Workers: q.Workers, Depth: q.Depth, InFlight: q.InFlight, OldestWaitSeconds: q.OldestWait.Seconds()}
		}
		if !rs.LastPassAt.IsZero() {
			res.Reconcile.LagSeconds = now.Sub(rs.LastPassAt).Seconds()
		}
	}
	var integrityFailed bool
	if p.Integrity != nil {
		integrityFailed = !p.Integrity.Status().Healthy
	}
	res.Reconcile.Guard = reconcile.GuardStatus{Breakers: []reconcile.BreakerStatus{}}
	if p.Guard != nil {
		res.Reconcile.Guard = p.Guard.Status()
	}
	res.Thresholds = cphealth.DefaultThresholds(os.Getenv)
	res.Findings = cphealth.Evaluate(cphealth.Inputs{
		OpenBreakers: res.Reconcile.Guard.OpenBreakers, RecentLimited: res.Reconcile.Guard.RecentLimited, IntegrityFailed: integrityFailed,
		Goroutines: res.Runtime.Goroutines, HeapBytes: res.Runtime.HeapBytes,
		DBP95MS: res.DB.P95MS, DBWaitCount: res.DB.WaitCount, DBInUse: res.DB.InUse,
		WALBytes: res.DB.WALBytes, SlowQueries: res.DB.SlowQueries,
		ReconcilePass: rs.LastPass, LastPassAt: rs.LastPassAt, ResyncInterval: p.ResyncInterval,
		RSSBytes: proc.RSSBytes, Threads: proc.Threads, TelemetryBytes: res.Footprint.TelemetryBytes,
		Streams: int(res.Streams.Active), StreamCap: int(res.Streams.Cap), Now: now,
	}, res.Thresholds)
	sort.SliceStable(res.Findings, func(i, j int) bool {
		return res.Findings[i].Severity == cphealth.SeverityCrit && res.Findings[j].Severity != cphealth.SeverityCrit
	})
	res.Status = "ok"
	for _, f := range res.Findings {
		if f.Severity == cphealth.SeverityCrit {
			res.Status = "crit"
			break
		}
		res.Status = "warn"
	}
	return res, true
}

func (rt *Router) handleGetCPHealth(w http.ResponseWriter, _ *http.Request) {
	res, ok := rt.cpHealthSnapshot()
	if !ok {
		writeError(w, http.StatusNotImplemented, "control plane health is not available")
		return
	}
	if res.Findings == nil {
		res.Findings = []cphealth.Finding{}
	}
	if res.Footprint.Loops == nil {
		res.Footprint.Loops = []footprint.LoopInfo{}
	}
	if res.DB.RecentSlow == nil {
		res.DB.RecentSlow = []store.SlowQuery{}
	}
	writeJSON(w, http.StatusOK, res)
}

type cpHealthFixRequest struct {
	Fix  string `json:"fix"`
	Name string `json:"name,omitempty"`
}

func (rt *Router) handleCPHealthFix(w http.ResponseWriter, r *http.Request) {
	p := rt.cpHealth.probe.Load()
	if p == nil || p.DB == nil {
		writeError(w, http.StatusNotImplemented, "control plane health is not available")
		return
	}
	var req cpHealthFixRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var err error
	_, _, actor, _ := rt.currentActor(r)
	switch req.Fix {
	case cphealth.FixResetGuard:
		if p.Guard == nil {
			writeError(w, http.StatusNotImplemented, "reconcile protection is not available")
			return
		}
		p.Guard.Reset("")
	case cphealth.FixIntegrityCheck, cphealth.FixIntegrityFix:
		if p.Integrity == nil {
			writeError(w, http.StatusNotImplemented, "database integrity checks are not available")
			return
		}
		if req.Fix == cphealth.FixIntegrityCheck {
			p.Integrity.Check(r.Context(), integrity.ModeFull)
		} else if _, err = p.Integrity.Restore(r.Context(), req.Name, actor); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	case cphealth.FixWALCheckpoint:
		err = p.DB.CheckpointWAL(r.Context(), true)
	case cphealth.FixDBOptimize:
		err = p.DB.Optimize(r.Context())
	case cphealth.FixResetSlowLog:
		p.DB.ResetSlowQueries()
	case cphealth.FixTelemetryPrune:
		if p.Telemetry == nil {
			writeError(w, http.StatusNotImplemented, "metrics database is not available")
			return
		}
		err = p.Telemetry.Compact(r.Context())
	default:
		writeError(w, http.StatusBadRequest, "unknown fix")
		return
	}
	if err != nil {
		rt.internalError(w, "api: control plane health fix failed", err)
		return
	}
	rt.logger.Info("api: control plane health fix applied", slog.String("fix", req.Fix), slog.String("actor", actor))
	res, _ := rt.cpHealthSnapshot()
	if res.Findings == nil {
		res.Findings = []cphealth.Finding{}
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) idleAssessment(p *ControlPlaneProbe) footprint.Assessment {
	apps := 0
	if p.AppCount != nil {
		apps = p.AppCount(context.Background())
	}
	cur := footprint.Snapshot(footprint.CPUWindowFromEnv(os.Getenv), apps)
	tol, err := footprint.ToleranceFromEnv(os.Getenv)
	return footprint.Assess(cur, footprint.Baseline(), runtime.GOOS, runtime.GOARCH, tol, err)
}

func (rt *Router) handleGetIdleFootprint(w http.ResponseWriter, _ *http.Request) {
	p := rt.cpHealth.probe.Load()
	if p == nil {
		writeError(w, http.StatusNotImplemented, "control plane health is not available")
		return
	}
	writeJSON(w, http.StatusOK, rt.idleAssessment(p))
}
