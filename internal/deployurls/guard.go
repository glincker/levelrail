package deployurls

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// EnvMaxUnready is how many readiness failures in a row a retained deploy
// service may have before it is suspended instead of retried.
const EnvMaxUnready = "APP_DEPLOY_URL_MAX_UNREADY"

// DefaultMaxUnready matches the few restarts the crash backoff allows before it plateaus.
const DefaultMaxUnready = 3

// MaxUnreadyFromEnv reads EnvMaxUnready, falling back to the default.
func MaxUnreadyFromEnv(getenv func(string) string) int {
	if n, err := strconv.Atoi(strings.TrimSpace(getenv(EnvMaxUnready))); err == nil && n > 0 {
		return n
	}
	return DefaultMaxUnready
}

// GuardStore is what Guard needs; *store.DB satisfies it.
type GuardStore interface {
	DeployURLApp(ctx context.Context, serviceName string) (string, bool, error)
	UpdateServiceSuspended(ctx context.Context, name string, suspended bool) error
}

// Guard bounds the retries of retained deploy services and lets them resolve
// their app's secrets. It outlives the per-pass controllers.
type Guard struct {
	Store GuardStore
	Max   int

	mu    sync.Mutex
	fails map[string]int
}

// NoteUnready records a readiness failure of service. For a retained deploy
// service that reaches Max it suspends the service (containers are torn down
// by the normal suspend path) and reports stopped. Other services are untouched.
func (g *Guard) NoteUnready(ctx context.Context, service string) (retained, stopped bool, err error) {
	if _, ok, err := g.Store.DeployURLApp(ctx, service); err != nil || !ok {
		return false, false, err
	}
	g.mu.Lock()
	if g.fails == nil {
		g.fails = map[string]int{}
	}
	g.fails[service]++
	n := g.fails[service]
	g.mu.Unlock()
	if n < g.Max {
		return true, false, nil
	}
	if err := g.Store.UpdateServiceSuspended(ctx, service, true); err != nil {
		return true, false, fmt.Errorf("suspend retained deploy %q: %w", service, err)
	}
	g.NoteReady(service)
	return true, true, nil
}

// NoteReady forgets service's failure count.
func (g *Guard) NoteReady(service string) {
	g.mu.Lock()
	delete(g.fails, service)
	g.mu.Unlock()
}

// IsRetained reports whether service is a retained deploy service.
func (g *Guard) IsRetained(ctx context.Context, service string) bool {
	_, ok, err := g.Store.DeployURLApp(ctx, service)
	return err == nil && ok
}

// SecretReader is the secrets surface the application controller uses.
type SecretReader interface {
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// ParentSecrets resolves a retained deploy service's secrets under its app's
// name, so the clone sees the same secret env without a stored copy of the values.
type ParentSecrets struct {
	Inner SecretReader
	Apps  interface {
		DeployURLApp(ctx context.Context, serviceName string) (string, bool, error)
	}
}

func (p ParentSecrets) owner(ctx context.Context, service string) string {
	if app, ok, err := p.Apps.DeployURLApp(ctx, service); err == nil && ok {
		return app
	}
	return service
}

// Exists implements SecretReader.
func (p ParentSecrets) Exists(ctx context.Context, service, key string) (bool, error) {
	return p.Inner.Exists(ctx, p.owner(ctx, service), key)
}

// Resolve implements SecretReader.
func (p ParentSecrets) Resolve(ctx context.Context, service, key string) (string, error) {
	return p.Inner.Resolve(ctx, p.owner(ctx, service), key)
}
