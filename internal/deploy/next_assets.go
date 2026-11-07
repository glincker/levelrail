package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Setting for how many superseded releases stay held per service. Each held
// generation keeps its containers running, so it costs their memory and CPU for
// the whole grace window; 1 keeps that to one extra copy of the app.
const (
	KeyNextAssetGraceMaxHeld = "deploy.next_asset_grace_max_held"
	EnvNextAssetGraceMaxHeld = "APP_DEPLOY_NEXT_ASSET_GRACE_MAX_HELD"
)

var nextGraceMaxHeld atomic.Pointer[func(context.Context) int]

// SetNextGraceMaxHeld wires the saved setting in at boot; nil restores the env seed.
func SetNextGraceMaxHeld(get func(context.Context) int) {
	if get == nil {
		nextGraceMaxHeld.Store(nil)
		return
	}
	nextGraceMaxHeld.Store(&get)
}

func nextGraceCap(ctx context.Context) int {
	n := 1
	if f := nextGraceMaxHeld.Load(); f != nil {
		n = (*f)(ctx)
	} else if raw, ok := os.LookupEnv(EnvNextAssetGraceMaxHeld); ok {
		if v, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
			n = v
		}
	}
	return max(n, 1)
}

// NextGraceStore is the durable side of the book. *store.DB satisfies it.
type NextGraceStore interface {
	InsertNextAssetGrace(ctx context.Context, g store.NextAssetGrace) error
	ListActiveNextAssetGrace(ctx context.Context, now time.Time) ([]store.NextAssetGrace, error)
	DeleteExpiredNextAssetGrace(ctx context.Context, now time.Time) error
	DeleteNextAssetGraceForService(ctx context.Context, service string) error
}

// deployIDFor is the stable per deploy id: the attempt id, else the commit.
func deployIDFor(req Request) string {
	if req.AttemptID != "" {
		return req.AttemptID
	}
	return req.CommitSHA
}

// NextAssetGrace records that a superseded Next.js release must keep serving
// its /_next/static and /_next/data requests until Until.
type NextAssetGrace = store.NextAssetGrace

// NextGraceBook is the deploy side bookkeeping. Reads are served from memory;
// with a store attached every change is written through and Load restores it.
type NextGraceBook struct {
	mu    sync.Mutex
	items map[string][]NextAssetGrace
	store NextGraceStore
	// changed fires after the set of active windows changes, so ingress can rebuild.
	changed func()
}

var defaultNextGraceBook = NewNextGraceBook()

// DefaultNextGraceBook is the process wide book the deploy tail, the reconciler
// and the ingress controller share.
func DefaultNextGraceBook() *NextGraceBook { return defaultNextGraceBook }

// WithStore attaches durable storage; nil keeps the book in memory only.
func (b *NextGraceBook) WithStore(s NextGraceStore) *NextGraceBook {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.store = s
	return b
}

// OnChange registers a callback run when a window opens or is evicted.
func (b *NextGraceBook) OnChange(fn func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.changed = fn
}

// Load restores the active windows after a restart and drops expired rows.
func (b *NextGraceBook) Load(ctx context.Context, now time.Time) error {
	b.mu.Lock()
	s := b.store
	b.mu.Unlock()
	if s == nil {
		return nil
	}
	if err := s.DeleteExpiredNextAssetGrace(ctx, now); err != nil {
		return err
	}
	rows, err := s.ListActiveNextAssetGrace(ctx, now)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = map[string][]NextAssetGrace{}
	for _, g := range rows {
		b.items[g.Service] = append(b.items[g.Service], g)
	}
	return nil
}

// HeldUntil is when the latest active window of service ends, zero when none.
func (b *NextGraceBook) HeldUntil(service string, now time.Time) time.Time {
	var until time.Time
	for _, g := range b.Active(service, now) {
		if g.Until.After(until) {
			until = g.Until
		}
	}
	return until
}

// NewNextGraceBook returns an empty book.
func NewNextGraceBook() *NextGraceBook {
	return &NextGraceBook{items: map[string][]NextAssetGrace{}}
}

// Begin opens a grace window for the superseded release. It returns false when
// grace is disabled, the old release carried no deployment id, or the same
// release is already recorded.
func (b *NextGraceBook) Begin(ctx context.Context, service, oldDeploymentID, oldImage string, now time.Time) bool {
	window := nextAssetGrace(ctx)
	if window <= 0 || oldDeploymentID == "" {
		return false
	}
	b.mu.Lock()
	b.prune(service, now)
	for _, g := range b.items[service] {
		if g.DeploymentID == oldDeploymentID {
			b.mu.Unlock()
			return false
		}
	}
	g := NextAssetGrace{Service: service, DeploymentID: oldDeploymentID, Image: oldImage, Until: now.Add(window)}
	list := append(b.items[service], g)
	// Only the newest generations stay held: older ones lose their window.
	evicted := false
	if limit := nextGraceCap(ctx); len(list) > limit {
		slices.SortStableFunc(list, func(a, c NextAssetGrace) int { return a.Until.Compare(c.Until) })
		list = list[len(list)-limit:]
		evicted = true
	}
	b.items[service] = list
	s, changed := b.store, b.changed
	b.mu.Unlock()
	if s != nil {
		if evicted {
			if err := s.DeleteNextAssetGraceForService(ctx, service); err != nil {
				slog.Warn("deploy: next grace evict write failed", slog.String("service", service), slog.String("error", err.Error()))
			}
		}
		for _, kept := range list {
			if err := s.InsertNextAssetGrace(ctx, kept); err != nil {
				slog.Warn("deploy: next grace write failed", slog.String("service", service), slog.String("error", err.Error()))
			}
		}
	}
	if changed != nil {
		changed()
	}
	return true
}

// Active lists releases still in grace at now.
func (b *NextGraceBook) Active(service string, now time.Time) []NextAssetGrace {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune(service, now)
	return append([]NextAssetGrace(nil), b.items[service]...)
}

func (b *NextGraceBook) prune(service string, now time.Time) {
	kept := b.items[service][:0]
	for _, g := range b.items[service] {
		if g.Until.After(now) {
			kept = append(kept, g)
		}
	}
	b.items[service] = kept
}

type desiredGetter interface {
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
}

// previousNextRelease reads the service's current desired state before it is
// replaced, so its deployment id and image can open a grace window.
func (p *Pipeline) previousNextRelease(ctx context.Context, req Request) (id, image string) {
	if req.nextDeploymentID == "" || req.Preview || nextAssetGrace(ctx) <= 0 {
		return "", ""
	}
	g, ok := p.store.(desiredGetter)
	if !ok {
		return "", ""
	}
	prev, err := g.GetDesiredService(ctx, req.ServiceName)
	if err != nil || prev == nil {
		return "", ""
	}
	return prev.Env[nextDeploymentIDEnvVar], prev.Image
}

// beginNextGrace opens the grace window once the new release is saved as current.
func (p *Pipeline) beginNextGrace(ctx context.Context, req Request, oldID, oldImage string) {
	if oldID == "" || oldID == req.nextDeploymentID {
		return
	}
	DefaultNextGraceBook().Begin(ctx, req.ServiceName, oldID, oldImage, time.Now())
}

const (
	nextBuildIDEnvVar    = "NEXT_BUILD_ID"
	nextCacheURLEnvVar   = "LEVELRAIL_NEXT_CACHE_URL"
	nextCacheRedisEnvVar = "REDIS_URL"
)

// nextBuildID returns the stable build id for a Next.js app on the shared Redis
// cache, or "" otherwise. It reuses the deployment id source and never replaces a user value.
func nextBuildID(sourceDir, deployID string, svc spec.Service) string {
	if deployID == "" || !svc.Next.UsesRedisCache() {
		return ""
	}
	if _, set := svc.Env[nextBuildIDEnvVar]; set {
		return ""
	}
	if !build.DetectNext(sourceDir).Detected {
		return ""
	}
	return deployID
}

// nextCacheLink finds the Redis connection for the shared cache: an app.yaml env
// var, or a database link saved on the previous release. ok is false when none exists.
func nextCacheLink(env map[string]spec.EnvVar, prev *store.DesiredService) (ref *store.DatabaseEnvRef, ok bool) {
	for _, k := range []string{nextCacheURLEnvVar, nextCacheRedisEnvVar} {
		if _, set := env[k]; set {
			return nil, true
		}
	}
	if prev == nil {
		return nil, false
	}
	for _, k := range []string{nextCacheURLEnvVar, nextCacheRedisEnvVar} {
		if r, set := prev.DatabaseEnv[k]; set {
			return &r, true
		}
	}
	return nil, false
}

// checkNextCache fails a Next.js deploy that asked for the Redis cache without a Redis link.
func (p *Pipeline) checkNextCache(ctx context.Context, sourceDir string, req Request) error {
	if !req.Service.Next.UsesRedisCache() || !build.DetectNext(sourceDir).Detected {
		return nil
	}
	var prev *store.DesiredService
	if g, ok := p.store.(desiredGetter); ok {
		prev, _ = g.GetDesiredService(ctx, req.ServiceName)
	}
	if _, ok := nextCacheLink(req.Service.Env, prev); ok {
		return nil
	}
	return fmt.Errorf("deploy: service %q: next.cache is redis but no Redis database is linked. Run: levelrail-cli databases create --name <name> --engine redis, then levelrail-cli databases link <name> --app %s --env REDIS_URL, or set REDIS_URL from a databases entry in app.yaml", req.ServiceName, req.ServiceName)
}

// applyNextCache sets the runtime env for the shared cache on the desired state.
func applyNextCache(desired *store.DesiredService, req Request, prev *store.DesiredService) {
	if req.nextBuildID != "" {
		if desired.Env == nil {
			desired.Env = map[string]string{}
		}
		desired.Env[nextBuildIDEnvVar] = req.nextBuildID
	}
	if !req.Service.Next.UsesRedisCache() {
		return
	}
	if _, set := req.Service.Env[nextCacheURLEnvVar]; set {
		return
	}
	ref, ok := nextCacheLink(req.Service.Env, prev)
	if !ok {
		return
	}
	if ref == nil {
		if r, set := desired.DatabaseEnv[nextCacheRedisEnvVar]; set {
			ref = &r
		}
	}
	if ref == nil {
		return
	}
	if desired.DatabaseEnv == nil {
		desired.DatabaseEnv = map[string]store.DatabaseEnvRef{}
	}
	desired.DatabaseEnv[nextCacheURLEnvVar] = *ref
}
