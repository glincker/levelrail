package deployurls

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
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

// Asleep reports whether service is a retained deploy copy that is marked
// sleeping. Only a wake request may start it, never a bulk or project start.
func (g *Guard) Asleep(ctx context.Context, service string) bool {
	sl, ok := g.Store.(interface {
		GetAppSleep(ctx context.Context, serviceName string) (store.AppSleep, error)
	})
	if !ok || !g.IsRetained(ctx, service) {
		return false
	}
	row, err := sl.GetAppSleep(ctx, service)
	return err == nil && row.Sleeping
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

// ErrParentSecretOwner marks a deploy URL parent whose name is a control plane
// namespace or a database, whose secrets a clone must never read.
var ErrParentSecretOwner = errors.New("deployurls: parent app name is not a valid secret owner")

type databaseLookup interface {
	GetDesiredDatabase(ctx context.Context, name string) (*store.DesiredDatabase, error)
}

// owner fails closed: a store error or a parent that is reserved or named like a database yields an error.
func (p ParentSecrets) owner(ctx context.Context, service string) (string, error) {
	app, ok, err := p.Apps.DeployURLApp(ctx, service)
	if err != nil {
		return "", fmt.Errorf("resolve deploy url parent: %w", err)
	}
	if !ok {
		return service, nil
	}
	if spec.IsReservedName(app) {
		return "", ErrParentSecretOwner
	}
	if dbs, ok := p.Apps.(databaseLookup); ok {
		if _, err := dbs.GetDesiredDatabase(ctx, app); err == nil {
			return "", ErrParentSecretOwner
		} else if !errors.Is(err, store.ErrDatabaseNotFound) {
			return "", fmt.Errorf("check parent database %q: %w", app, err)
		}
	}
	return app, nil
}

// Exists implements SecretReader.
func (p ParentSecrets) Exists(ctx context.Context, service, key string) (bool, error) {
	owner, err := p.owner(ctx, service)
	if err != nil {
		return false, err
	}
	return p.Inner.Exists(ctx, owner, key)
}

// Resolve implements SecretReader.
func (p ParentSecrets) Resolve(ctx context.Context, service, key string) (string, error) {
	owner, err := p.owner(ctx, service)
	if err != nil {
		return "", err
	}
	return p.Inner.Resolve(ctx, owner, key)
}
