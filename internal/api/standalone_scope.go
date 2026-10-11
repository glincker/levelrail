package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/standalone"
	"github.com/GLINCKER/levelrail/internal/store"
)

const standaloneProbeTimeout = 3 * time.Second

// standaloneOwns reports whether domain is a standalone domain, so the
// per-domain endpoints accept {name} = standalone.Scope for it.
func (rt *Router) standaloneOwns(ctx context.Context, domain string) (bool, error) {
	st := rt.standaloneStore()
	if st == nil {
		return false, store.ErrServiceNotFound
	}
	_, err := st.GetStandaloneDomain(ctx, normalizeStandaloneDomain(domain))
	if errors.Is(err, store.ErrStandaloneDomainNotFound) {
		return false, nil
	}
	return err == nil, err
}

// domainScopeExists is GetDesiredService's existence check for the
// per-domain endpoints, also accepting the standalone scope.
func (rt *Router) domainScopeExists(ctx context.Context, name string) error {
	if name == standalone.Scope {
		if rt.standaloneStore() == nil {
			return store.ErrServiceNotFound
		}
		return nil
	}
	_, err := rt.apps.GetDesiredService(ctx, name)
	return err
}

// doctorSubject loads the app the doctor checks, or for the standalone
// scope a stand-in plus the probed target (nil target for an app).
func (rt *Router) doctorSubject(ctx context.Context, name, domain string) (*store.DesiredService, *domaindoctor.TargetFacts, error) {
	if name != standalone.Scope {
		svc, err := rt.apps.GetDesiredService(ctx, name)
		return svc, nil, err
	}
	st := rt.standaloneStore()
	if st == nil {
		return nil, nil, store.ErrServiceNotFound
	}
	d, err := st.GetStandaloneDomain(ctx, normalizeStandaloneDomain(domain))
	if errors.Is(err, store.ErrStandaloneDomainNotFound) {
		return nil, nil, store.ErrServiceNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	return &store.DesiredService{Name: standalone.Scope}, rt.probeStandaloneTarget(ctx, d), nil
}

// probeStandaloneTarget dials the target over TCP from this server. App
// and container targets report the app or container state instead.
func (rt *Router) probeStandaloneTarget(ctx context.Context, d store.StandaloneDomain) *domaindoctor.TargetFacts {
	t, err := standalone.ParseTarget(d.Target)
	if err != nil {
		return &domaindoctor.TargetFacts{Display: d.Target, Detail: err.Error()}
	}
	f := &domaindoctor.TargetFacts{Display: t.Display(d.UpstreamTLS)}
	switch t.Kind {
	case standalone.KindApp:
		conds, err := rt.deploys.GetConditionsForControllers(ctx, []string{applicationControllerName(t.App)})
		if err != nil {
			f.Detail = err.Error()
			return f
		}
		st := doctorAppFacts(conds[applicationControllerName(t.App)])
		f.Reachable, f.Detail = st.Running && st.Ready, st.Detail
		return f
	case standalone.KindContainer:
		if rt.containers == nil {
			f.Reachable, f.Detail = true, "container state is not available on this control plane"
			return f
		}
		list, err := rt.containers.ListByPrefix(ctx, t.Container)
		if err != nil {
			f.Detail = err.Error()
			return f
		}
		for _, c := range list {
			if c.Name == t.Container && c.Running {
				f.Reachable, f.Detail = true, "container "+t.Container+" is running"
				return f
			}
		}
		f.Detail = "container " + t.Container + " is not running"
		return f
	}
	a, err := standalone.Assess(ctx, t, standalone.Env{Resolver: rt.resolverOrDefault()})
	if err != nil || a.Dial == "" {
		f.Detail = "could not resolve the target"
		if err != nil {
			f.Detail = err.Error()
		}
		return f
	}
	conn, err := (&net.Dialer{Timeout: standaloneProbeTimeout}).DialContext(ctx, "tcp", a.Dial)
	if err != nil {
		f.Detail = "connect to " + a.Dial + " failed: " + err.Error()
		return f
	}
	_ = conn.Close()
	f.Reachable, f.Detail = true, "this server connected to "+a.Dial
	return f
}

// standaloneDomainRows are GET /api/v1/domains rows for standalone domains,
// shown only to instance members: they are not team scoped.
func (rt *Router) standaloneDomainRows(r *http.Request, flags func(domain string) domainResource) []domainResource {
	if scope, ok := callerTeamScope(r.Context()); ok && scope != nil && !scope.instance {
		return nil
	}
	st := rt.standaloneStore()
	if st == nil {
		return nil
	}
	rows, err := st.ListStandaloneDomains(r.Context())
	if err != nil {
		rt.logger.Warn("api: list domains: standalone domains skipped", slog.String("error", err.Error()))
		return nil
	}
	out := make([]domainResource, 0, len(rows))
	for _, d := range rows {
		res := flags(d.Domain)
		res.Domain, res.Kind, res.Target = d.Domain, domainKindStandalone, toStandaloneDomainResource(d).TargetURL
		out = append(out, res)
	}
	return out
}

func (rt *Router) resolverOrDefault() standalone.Resolver {
	if rt.standaloneResolver != nil {
		return rt.standaloneResolver
	}
	return net.DefaultResolver
}
