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

// ControlPlaneProbe wires the health page to the running process.
type ControlPlaneProbe struct {
	DB             ControlPlaneDB
	Reconcile      func() reconcile.Stats
	ResyncInterval time.Duration
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
	CheckedAt  time.Time           `json:"checked_at"`
	Runtime    cpRuntime           `json:"runtime"`
	DB         store.DBMetrics     `json:"db"`
	Reconcile  cpReconcile         `json:"reconcile"`
	Streams    cpStreams           `json:"streams"`
	Thresholds cphealth.Thresholds `json:"thresholds"`
	Findings   []cphealth.Finding  `json:"findings"`
	Status     string              `json:"status"`
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
	var rs reconcile.Stats
	if p.Reconcile != nil {
		rs = p.Reconcile()
		res.Reconcile = cpReconcile{
			Passes: rs.Passes, Controllers: rs.Controllers, LastPassMS: durMS(rs.LastPass),
			P50MS: durMS(rs.P50), P95MS: durMS(rs.P95), Slowest: rs.SlowestName, Pending: rs.Pending,
		}
		if !rs.LastPassAt.IsZero() {
			res.Reconcile.LagSeconds = now.Sub(rs.LastPassAt).Seconds()
		}
	}
	res.Thresholds = cphealth.DefaultThresholds(os.Getenv)
	res.Findings = cphealth.Evaluate(cphealth.Inputs{
		Goroutines: res.Runtime.Goroutines, HeapBytes: res.Runtime.HeapBytes,
		DBP95MS: res.DB.P95MS, DBWaitCount: res.DB.WaitCount, DBInUse: res.DB.InUse,
		WALBytes: res.DB.WALBytes, SlowQueries: res.DB.SlowQueries,
		ReconcilePass: rs.LastPass, LastPassAt: rs.LastPassAt, ResyncInterval: p.ResyncInterval,
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
	if res.DB.RecentSlow == nil {
		res.DB.RecentSlow = []store.SlowQuery{}
	}
	writeJSON(w, http.StatusOK, res)
}

type cpHealthFixRequest struct {
	Fix string `json:"fix"`
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
	switch req.Fix {
	case cphealth.FixWALCheckpoint:
		err = p.DB.CheckpointWAL(r.Context(), true)
	case cphealth.FixDBOptimize:
		err = p.DB.Optimize(r.Context())
	case cphealth.FixResetSlowLog:
		p.DB.ResetSlowQueries()
	default:
		writeError(w, http.StatusBadRequest, "unknown fix")
		return
	}
	if err != nil {
		rt.internalError(w, "api: control plane health fix failed", err)
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	rt.logger.Info("api: control plane health fix applied", slog.String("fix", req.Fix), slog.String("actor", actor))
	res, _ := rt.cpHealthSnapshot()
	if res.Findings == nil {
		res.Findings = []cphealth.Finding{}
	}
	writeJSON(w, http.StatusOK, res)
}
