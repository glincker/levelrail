package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"

	"github.com/GLINCKER/levelrail/internal/store"
)

// lifecycleSide is one domain set a park, move or attach touches, with its
// lists as they were before the change.
type lifecycleSide struct {
	target  store.DomainTarget
	envName string
	set     []string
	// def is an app's default list, which the env re-render diffs against.
	def []string
}

// normalizeDomainKind maps an empty kind to app and rejects unknown ones.
func normalizeDomainKind(kind string) (string, bool) {
	switch kind {
	case "", store.DomainKindApp:
		return store.DomainKindApp, true
	case store.DomainKindStaticSite:
		return kind, true
	}
	return "", false
}

// domainTargetRefs are the refs a side needs: a static site needs both its
// own ref and the same-named app's, so a deny on either blocks it.
func domainTargetRefs(t store.DomainTarget) ([]resourceRef, string) {
	if t.Kind == store.DomainKindStaticSite {
		return []resourceRef{staticSiteRef(t.Name), appRef(t.Name)}, "static site not found"
	}
	return []resourceRef{appRef(t.Name)}, "app not found"
}

// authorizeDomainTargets checks a write_sensitive grant on each park, move or
// attach side; that can only narrow access.
func (rt *Router) authorizeDomainTargets(w http.ResponseWriter, r *http.Request, sides ...store.DomainTarget) bool {
	for _, t := range sides {
		refs, msg := domainTargetRefs(t)
		if !rt.authorizeRefs(w, r, AbilityWriteSensitive, notFoundAs(http.StatusNotFound, msg), refs...) {
			return false
		}
	}
	return true
}

// authorizeParkedOrigin gates attach and delete of a parked row on the
// target it left. A row with no recorded origin is not gated.
func (rt *Router) authorizeParkedOrigin(w http.ResponseWriter, r *http.Request, st parkedDomainStore, domain string) bool {
	p, err := st.GetParkedDomain(r.Context(), domain)
	if err != nil || p.ParkedFrom == "" {
		return true
	}
	return rt.authorizeDomainTargets(w, r, store.DomainTarget{Kind: kindOrAppName(p.ParkedFromKind), Name: p.ParkedFrom})
}

// resolveDomainTarget validates kind and name and resolves the environment
// of an app target. A static site has no environment sets.
func (rt *Router) resolveDomainTarget(w http.ResponseWriter, r *http.Request, kind, name, env string) (lifecycleSide, bool) {
	kind, ok := normalizeDomainKind(kind)
	if !ok {
		writeError(w, http.StatusBadRequest, "kind must be app or static_site")
		return lifecycleSide{}, false
	}
	if kind == store.DomainKindApp {
		envID, envName, ok := rt.resolveLifecycleEnv(w, r, name, env)
		if !ok {
			return lifecycleSide{}, false
		}
		side := lifecycleSide{target: store.DomainTarget{Kind: kind, Name: name, EnvID: envID}, envName: envName}
		side.set, side.def = rt.domainSetList(r.Context(), side.target), rt.appDomainList(r.Context(), name)
		return side, true
	}
	if env != "" {
		writeError(w, http.StatusBadRequest, "a static site has no environment domain sets")
		return lifecycleSide{}, false
	}
	getter, ok := rt.staticSites.(staticSiteGetter)
	if !ok {
		writeError(w, http.StatusNotImplemented, "static sites are not supported by this store")
		return lifecycleSide{}, false
	}
	site, err := getter.GetStaticSite(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "static site not found")
		return lifecycleSide{}, false
	}
	if err != nil {
		rt.internalError(w, "api: get static site", err, slog.String("name", name))
		return lifecycleSide{}, false
	}
	return lifecycleSide{target: store.DomainTarget{Kind: kind, Name: name}, set: slices.Clone(site.Domains)}, true
}

// domainSetList is the live domain list of a target, nil when unreadable.
func (rt *Router) domainSetList(ctx context.Context, t store.DomainTarget) []string {
	if t.Kind == store.DomainKindStaticSite {
		getter, ok := rt.staticSites.(staticSiteGetter)
		if !ok {
			return nil
		}
		site, err := getter.GetStaticSite(ctx, t.Name)
		if err != nil || site == nil {
			return nil
		}
		return slices.Clone(site.Domains)
	}
	svc, err := rt.apps.GetDesiredService(ctx, t.Name)
	if err != nil || svc == nil {
		return nil
	}
	return slices.Clone(rt.domainSetOf(ctx, svc, t.EnvID))
}

// describeDomainTarget names a side for messages.
func describeDomainTarget(s lifecycleSide) string {
	if s.target.Kind == store.DomainKindStaticSite {
		return "static site " + s.target.Name
	}
	return s.target.Name
}
