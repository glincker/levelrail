package api

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// diagnosticTTL is set by wireDiagnosticSettings; it stays nil until runtime
// settings attach, in which case the default applies.
var diagnosticTTL atomic.Pointer[func(context.Context) time.Duration]

// diagnosticHandler is nil until StartDiagnostic runs; the route answers 404 then.
var diagnosticHandler atomic.Pointer[http.Handler]

func (rt *Router) handleInternalDiagnostic(w http.ResponseWriter, r *http.Request) {
	if h := diagnosticHandler.Load(); h != nil {
		(*h).ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

const defaultDiagnosticTTL = 5 * time.Second

func wireDiagnosticSettings(svc *runtimesettings.Service) {
	f := DiagnosticCacheTTL(svc)
	diagnosticTTL.Store(&f)
}

func liveDiagnosticTTL(ctx context.Context) time.Duration {
	if f := diagnosticTTL.Load(); f != nil {
		return (*f)(ctx)
	}
	return defaultDiagnosticTTL
}

// DiagnosticStore is the read side the snapshot needs.
type DiagnosticStore interface {
	ListDesiredServices(ctx context.Context) ([]store.DesiredService, error)
	ListQueuedDeployAttempts(ctx context.Context) ([]store.DeployAttempt, error)
	ListRunningDeployAttempts(ctx context.Context) ([]store.DeployAttempt, error)
}

// DiagnosticContainers lists containers by name prefix.
type DiagnosticContainers interface {
	ListByPrefix(ctx context.Context, prefix string) ([]docker.ContainerState, error)
}

func isReplicaOf(container, service string) bool {
	if container == service {
		return true
	}
	rest, ok := strings.CutPrefix(container, service)
	return ok && replicaSuffix.MatchString(rest)
}

var replicaSuffix = regexp.MustCompile(`^-\d+$`)

// DiagnosticFacts gathers per service facts; freshWindow is how long a
// running container counts as still starting.
func DiagnosticFacts(st DiagnosticStore, cs DiagnosticContainers, freshWindow time.Duration, now func() time.Time) ingress.FactsSource {
	return func(ctx context.Context) ([]ingress.ServiceFacts, error) {
		svcs, err := st.ListDesiredServices(ctx)
		if err != nil {
			return nil, err
		}
		building := map[string]bool{}
		for _, list := range []func(context.Context) ([]store.DeployAttempt, error){st.ListQueuedDeployAttempts, st.ListRunningDeployAttempts} {
			attempts, err := list(ctx)
			if err != nil {
				return nil, err
			}
			for _, a := range attempts {
				building[a.ServiceName] = true
			}
		}
		var containers []docker.ContainerState
		if cs != nil {
			if containers, err = cs.ListByPrefix(ctx, ""); err != nil {
				return nil, err
			}
		}
		out := make([]ingress.ServiceFacts, 0, len(svcs))
		for _, s := range svcs {
			f := ingress.ServiceFacts{
				Name: s.Name, Domains: s.Domains, Building: building[s.Name],
				Stopped: s.Suspended || s.Replicas == 0,
			}
			for _, c := range containers {
				if !isReplicaOf(c.Name, s.Name) {
					continue
				}
				f.Containers++
				if c.Running {
					f.Running++
					if !c.Created.IsZero() && now().Sub(c.Created) < freshWindow {
						f.Fresh = true
					}
				}
			}
			out = append(out, f)
		}
		return out, nil
	}
}

// StartDiagnostic runs the snapshot refresher and returns the handler for the
// ingress catch-all hop. Only requests carrying token are served. It returns
// nil when the stores needed are not available.
func (rt *Router) StartDiagnostic(ctx context.Context, token, brandName, docsURL string, freshWindow time.Duration) http.Handler {
	st, ok := any(rt.apps).(DiagnosticStore)
	if !ok || token == "" {
		return nil
	}
	var cs DiagnosticContainers
	if rt.containers != nil {
		cs, _ = any(rt.containers).(DiagnosticContainers)
	}
	snap := ingress.NewDiagnosticSnapshot(DiagnosticFacts(st, cs, freshWindow, time.Now))
	go snap.Run(ctx, liveDiagnosticTTL, func(err error) {
		rt.logger.Warn("api: diagnostic snapshot refresh failed", slog.String("error", err.Error()))
	})
	// The snapshot is already an in-memory cache, so no per host cache layer.
	h := ingress.TrustedDiagnosticHandler(token, ingress.DiagnosticHandler(snap.Lookup, brandName, docsURL))
	diagnosticHandler.Store(&h)
	return h
}
