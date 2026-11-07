package build

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Environment overrides and defaults for build limits. Zero disables a limit.
const (
	EnvBuildTimeout       = "APP_BUILD_TIMEOUT"
	EnvBuildMaxConcurrent = "APP_BUILD_MAX_CONCURRENT"
	EnvBuildMemoryMB      = "APP_BUILD_MEMORY_MB"
	EnvBuildCPUMillis     = "APP_BUILD_CPU_MILLIS"
	EnvBuildCgroupRoot    = "APP_BUILD_CGROUP_ROOT"

	DefaultBuildTimeout       = 30 * time.Minute
	DefaultBuildMaxConcurrent = 2

	recordTimeout = 5 * time.Second
)

// ErrBuildTimeout is the cause of a build stopped by its time limit.
var ErrBuildTimeout = errors.New("build timed out")

// ErrBuildMemoryLimit marks a build killed for exceeding its memory limit.
var ErrBuildMemoryLimit = errors.New("build exceeded its memory limit")

// Limits caps builds. A zero field means unlimited. MemoryMB and CPUMillis
// (1000 = one core) are per build; the rest are control-plane wide.
type Limits struct {
	MaxConcurrent int
	Timeout       time.Duration
	MemoryMB      int
	CPUMillis     int
}

// LimitsFromEnv reads the env defaults; a malformed value falls back to the default.
func LimitsFromEnv(lookup func(string) (string, bool)) Limits {
	l := Limits{MaxConcurrent: DefaultBuildMaxConcurrent, Timeout: DefaultBuildTimeout}
	if v, ok := lookup(EnvBuildTimeout); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			l.Timeout = d
		}
	}
	if v, ok := lookup(EnvBuildMaxConcurrent); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			l.MaxConcurrent = n
		}
	}
	if v, ok := lookup(EnvBuildMemoryMB); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			l.MemoryMB = n
		}
	}
	if v, ok := lookup(EnvBuildCPUMillis); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			l.CPUMillis = n
		}
	}
	return l
}

// LimitsSource resolves the limits for the next build of app, so a settings
// change applies without a restart.
type LimitsSource func(ctx context.Context, app string) Limits

// Clock lets tests drive the timeout and queue wait without sleeping.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// RunRecorder persists one finished build.
type RunRecorder interface {
	RecordBuildRun(ctx context.Context, r Run) error
}

// Builder is the build surface the Limiter wraps and itself provides.
type Builder interface {
	Build(ctx context.Context, req Request, progress func(ProgressEvent)) (*Result, error)
	BuildRailpack(ctx context.Context, req RailpackRequest, progress func(ProgressEvent)) (*Result, error)
}

// LimiterOption configures a Limiter.
type LimiterOption func(*Limiter)

// WithRunRecorder records every finished build.
func WithRunRecorder(r RunRecorder) LimiterOption { return func(l *Limiter) { l.rec = r } }

// WithLimiterClock overrides the clock, for tests.
func WithLimiterClock(c Clock) LimiterOption { return func(l *Limiter) { l.clock = c } }

// WithLimiterLogger overrides the logger.
func WithLimiterLogger(lg *slog.Logger) LimiterOption { return func(l *Limiter) { l.log = lg } }

// NodeResolver names the node the next build will run on, "" for the local node.
type NodeResolver func(ctx context.Context) string

// ImageSizer reports a built image's size in bytes, 0 when unknown.
type ImageSizer func(ctx context.Context, tag string) int64

// WithNodeResolver makes the concurrency limit apply per node instead of globally.
func WithNodeResolver(n NodeResolver) LimiterOption { return func(l *Limiter) { l.node = n } }

// WithImageSizer records the size of every built image.
func WithImageSizer(s ImageSizer) LimiterOption { return func(l *Limiter) { l.sizer = s } }

// WithCgroupProvisioner enforces the CPU and memory limits through a cgroup per build.
func WithCgroupProvisioner(c CgroupProvisioner) LimiterOption { return func(l *Limiter) { l.cg = c } }

// WithMemoryGuard sets the node heap share and a host memory reader (MiB) used for warnings and OOM messages.
func WithMemoryGuard(g MemGuard, hostMB func() int) LimiterOption {
	return func(l *Limiter) { l.guard, l.host = g, hostMB }
}

// Limiter queues builds past the per-node concurrency limit, stops builds past the
// time limit, and records each outcome with its cache hit counts.
type Limiter struct {
	inner  Builder
	limits LimitsSource
	rec    RunRecorder
	clock  Clock
	log    *slog.Logger

	node  NodeResolver
	sizer ImageSizer
	cg    CgroupProvisioner
	guard MemGuard
	host  func() int

	mu    sync.Mutex
	slots map[string]*nodeSlots
}

type nodeSlots struct {
	running int
	limit   int
	waiters []chan struct{}
}

// NewLimiter wraps inner. limits is read once per build.
func NewLimiter(inner Builder, limits LimitsSource, opts ...LimiterOption) *Limiter {
	l := &Limiter{inner: inner, limits: limits, guard: MemGuard{NodeHeapPercent: DefaultNodeHeapPercent}, clock: realClock{}, log: slog.Default(), slots: map[string]*nodeSlots{}}
	for _, o := range opts {
		o(l)
	}
	return l
}

// Build implements the deploy pipeline's ImageBuilder.
func (l *Limiter) Build(ctx context.Context, req Request, progress func(ProgressEvent)) (*Result, error) {
	return l.run(ctx, req.Tag, req.ContextDir, progress, func(c context.Context, p func(ProgressEvent), cg string, _ int) (*Result, error) {
		req.CgroupParent = cg
		return l.inner.Build(c, req, p)
	})
}

// BuildRailpack implements the deploy pipeline's ImageBuilder.
func (l *Limiter) BuildRailpack(ctx context.Context, req RailpackRequest, progress func(ProgressEvent)) (*Result, error) {
	return l.run(ctx, req.Tag, req.SourceDir, progress, func(c context.Context, p func(ProgressEvent), cg string, heapMB int) (*Result, error) {
		req.CgroupParent = cg
		req.NodeHeapMB = heapMB
		return l.inner.BuildRailpack(c, req, p)
	})
}

func (l *Limiter) run(ctx context.Context, tag, srcDir string, progress func(ProgressEvent), do func(context.Context, func(ProgressEvent), string, int) (*Result, error)) (*Result, error) {
	lim := l.limits(ctx, AppFromTag(tag))
	queued := l.clock.Now()
	nodeID := ""
	if l.node != nil {
		nodeID = l.node(ctx)
	}
	if err := l.acquire(ctx, nodeID, lim.MaxConcurrent); err != nil {
		return nil, fmt.Errorf("build: waiting for a build slot: %w", err)
	}
	defer l.release(nodeID)

	started := l.clock.Now()
	run := Run{App: AppFromTag(tag), Tag: tag, StartedAt: started, QueueWait: started.Sub(queued), Node: nodeID, Platform: HostPlatform()}

	bctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	if lim.Timeout > 0 {
		timer := l.clock.After(lim.Timeout)
		go func() {
			select {
			case <-timer:
				cancel(ErrBuildTimeout)
			case <-bctx.Done():
			}
		}()
	}

	cgroupParent := ""
	if l.cg != nil && (lim.MemoryMB > 0 || lim.CPUMillis > 0) {
		parent, cleanup, cerr := l.cg.Provision(tag, lim.MemoryMB, lim.CPUMillis)
		if cerr != nil {
			l.log.Warn("build: resource limits not enforced", slog.String("app", run.App), slog.String("error", cerr.Error()))
		} else {
			cgroupParent = parent
			defer cleanup()
		}
	}

	var steps stepCounter
	var smu sync.Mutex
	wrapped := func(ev ProgressEvent) {
		smu.Lock()
		steps.observe(ev)
		smu.Unlock()
		if progress != nil {
			progress(ev)
		}
	}

	hostMB := 0
	if l.host != nil {
		hostMB = l.host()
	}
	if w := LowMemoryWarning(DetectBuildFramework(srcDir), hostMB); w != "" && progress != nil {
		progress(ProgressEvent{Step: "memory guard", Log: w, Stream: "stderr"})
	}
	for _, line := range NextBuildNotes(srcDir, hostMB, l.nextMinFreeMiB(ctx)) {
		if progress != nil {
			progress(ProgressEvent{Step: "next.js", Log: line, Stream: "stderr"})
		}
	}
	heapMB := 0
	if nodeID == "" {
		heapMB = NodeHeapMB(lim.MemoryMB, l.guard.NodeHeapPercent)
	}

	res, err := do(bctx, wrapped, cgroupParent, heapMB)
	run.Duration = l.clock.Now().Sub(started)
	smu.Lock()
	run.CachedSteps, run.TotalSteps = steps.cached, steps.total()
	smu.Unlock()

	switch {
	case err == nil:
		run.Status = RunSuccess
		if l.sizer != nil {
			run.ImageBytes = l.sizer(context.WithoutCancel(ctx), tag)
		}
	case errors.Is(context.Cause(bctx), ErrBuildTimeout):
		run.Status = RunTimeout
		err = fmt.Errorf("%w after %s: %w", ErrBuildTimeout, lim.Timeout, err)
	case ctx.Err() != nil:
		run.Status = RunCanceled
	case lim.MemoryMB > 0 && isMemoryKill(err):
		run.Status = RunMemoryLimit
		err = fmt.Errorf("%w (%d MB): %s: %w", ErrBuildMemoryLimit, lim.MemoryMB, MemoryKillMessage(lim.MemoryMB, hostMB), err)
	default:
		run.Status = RunFailed
	}
	if err != nil {
		run.Error = err.Error()
	}
	l.record(ctx, run)
	return res, err
}

// isMemoryKill matches the SIGKILL and OOM wording BuildKit surfaces for a cgroup memory kill.
func isMemoryKill(err error) bool {
	m := strings.ToLower(err.Error())
	for _, p := range []string{"oomkilled", "out of memory", "signal: killed", "exit code: 137", "exit code 137", "exit status 137"} {
		if strings.Contains(m, p) {
			return true
		}
	}
	return false
}

// record never fails a build: a finished build whose stats could not be saved
// is still a finished build.
func (l *Limiter) record(ctx context.Context, r Run) {
	if l.rec == nil {
		return
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()
	if err := l.rec.RecordBuildRun(rctx, r); err != nil {
		l.log.Warn("build: record build run failed", slog.String("app", r.App), slog.String("error", err.Error()))
	}
}

func (l *Limiter) slotsFor(node string) *nodeSlots {
	n := l.slots[node]
	if n == nil {
		n = &nodeSlots{}
		l.slots[node] = n
	}
	return n
}

func (l *Limiter) acquire(ctx context.Context, node string, limit int) error {
	l.mu.Lock()
	n := l.slotsFor(node)
	n.limit = limit
	if limit <= 0 || (n.running < limit && len(n.waiters) == 0) {
		n.running++
		l.mu.Unlock()
		return nil
	}
	ch := make(chan struct{})
	n.waiters = append(n.waiters, ch)
	l.mu.Unlock()

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		l.mu.Lock()
		for i, w := range n.waiters {
			if w == ch {
				n.waiters = append(n.waiters[:i], n.waiters[i+1:]...)
				l.mu.Unlock()
				return ctx.Err()
			}
		}
		l.mu.Unlock()
		// Admitted at the same moment the context ended: hand the slot back.
		l.release(node)
		return ctx.Err()
	}
}

func (l *Limiter) release(node string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := l.slotsFor(node)
	n.running--
	for len(n.waiters) > 0 && (n.limit <= 0 || n.running < n.limit) {
		ch := n.waiters[0]
		n.waiters = n.waiters[1:]
		n.running++
		close(ch)
	}
}

// NodeLoad is one node's build slot usage.
type NodeLoad struct {
	Node    string
	Running int
	Waiting int
	Limit   int
}

// Load reports slot usage per node that has seen a build, sorted by node ID.
func (l *Limiter) Load() []NodeLoad {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]NodeLoad, 0, len(l.slots))
	for id, n := range l.slots {
		out = append(out, NodeLoad{Node: id, Running: n.running, Waiting: len(n.waiters), Limit: n.limit})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out
}

// Queued reports builds waiting for a slot and builds running, across all nodes.
func (l *Limiter) Queued() (waiting, running int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, n := range l.slots {
		waiting += len(n.waiters)
		running += n.running
	}
	return waiting, running
}

// HostPlatform is the OS/arch BuildKit builds for: the daemon's own platform, since no cross-platform target is requested.
func HostPlatform() string { return runtime.GOOS + "/" + runtime.GOARCH }

// EnvBuildNextMinFreeMemoryMiB seeds the Next.js low memory warning threshold.
const EnvBuildNextMinFreeMemoryMiB = "APP_BUILD_NEXT_MIN_FREE_MEMORY_MIB"

// KeyBuildNextMinFreeMemoryMiB is the runtime setting key.
const KeyBuildNextMinFreeMemoryMiB = "build.next_min_free_memory_mib"

// DefaultNextMinFreeMemoryMiB is the seed default when no setting is wired.
const DefaultNextMinFreeMemoryMiB = 2048

var nextMinFreeResolver atomic.Pointer[func(context.Context) int]

// SetNextMinFreeMemoryResolver wires the saved setting in at boot so changes apply live.
func SetNextMinFreeMemoryResolver(fn func(context.Context) int) {
	if fn == nil {
		nextMinFreeResolver.Store(nil)
		return
	}
	nextMinFreeResolver.Store(&fn)
}

func (l *Limiter) nextMinFreeMiB(ctx context.Context) int {
	if fn := nextMinFreeResolver.Load(); fn != nil {
		return (*fn)(ctx)
	}
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(EnvBuildNextMinFreeMemoryMiB))); err == nil {
		return v
	}
	return DefaultNextMinFreeMemoryMiB
}

// NextBuildNotes returns the build log lines for a Next.js source: what was detected and, when
// the host is under minFreeMiB, a memory warning. It never blocks a build.
func NextBuildNotes(srcDir string, hostMiB, minFreeMiB int) []string {
	prof := DetectNext(srcDir)
	if !prof.Detected {
		return nil
	}
	yn := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}
	notes := []string{fmt.Sprintf("next.js %d detected (standalone: %s, app router: %s)", prof.Major, yn(prof.Standalone), yn(prof.UsesAppRouter))}
	if minFreeMiB > 0 && hostMiB > 0 && hostMiB < minFreeMiB {
		notes = append(notes, fmt.Sprintf("next.js builds can run out of memory on this host (%d MiB free). Add swap or build elsewhere.", hostMiB))
	}
	return notes
}

// NextCacheSuggestion returns one log line for a multi replica Next.js app that
// keeps its ISR cache per replica, or "" otherwise. It never blocks a build.
func NextCacheSuggestion(srcDir string, replicas int, redisCache bool) string {
	if replicas <= 1 || redisCache || !DetectNext(srcDir).Detected {
		return ""
	}
	return "next.js runs on several replicas with a per replica cache. Set next: {cache: redis} in app.yaml to share the ISR and data cache."
}
