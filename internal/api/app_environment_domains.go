package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// serviceEnvironmentDomainStore is the per-environment domain surface
// *store.DB provides.
type serviceEnvironmentDomainStore interface {
	EditServiceEnvironmentDomains(ctx context.Context, name, envID string, edit func(current []string) ([]string, error)) ([]string, bool, error)
	ListServiceEnvironmentDomains(ctx context.Context, name string) (map[string][]string, error)
}

// environmentDomainSetResource is one environment's domain set for an app.
type environmentDomainSetResource struct {
	EnvironmentID string   `json:"environment_id"`
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	Active        bool     `json:"active"`
	Domains       []string `json:"domains"`
}

// appEnvironmentDomainsResource is GET /api/v1/apps/{name}/environment-domains.
type appEnvironmentDomainsResource struct {
	App string `json:"app"`
	// ActiveEnvironmentID is the environment the app is tagged with, whose
	// set (or else the default set) is what the ingress routes right now.
	ActiveEnvironmentID string                         `json:"active_environment_id,omitempty"`
	DefaultDomains      []string                       `json:"default_domains"`
	RoutedDomains       []string                       `json:"routed_domains"`
	Environments        []environmentDomainSetResource `json:"environments"`
}

// resolveDomainEnvironment accepts an environment ID, name, or kind.
// Project-scoped environments of another project are not eligible.
func (rt *Router) resolveDomainEnvironment(ctx context.Context, value, projectID string) (store.Environment, error) {
	envs, err := rt.environments.ListAllEnvironments(ctx)
	if err != nil {
		return store.Environment{}, fmt.Errorf("list environments: %w", err)
	}
	eligible := func(e store.Environment) bool {
		return e.Scope == store.EnvironmentScopeGlobal || e.ProjectID == projectID
	}
	want := strings.TrimSpace(value)
	for _, pass := range []func(store.Environment) bool{
		func(e store.Environment) bool { return e.ID == want },
		func(e store.Environment) bool { return strings.EqualFold(e.Name, want) },
		func(e store.Environment) bool {
			return e.Kind == strings.ToLower(want) && e.Scope == store.EnvironmentScopeGlobal
		},
	} {
		for _, e := range envs {
			if eligible(e.Environment) && pass(e.Environment) {
				return e.Environment, nil
			}
		}
	}
	return store.Environment{}, store.ErrEnvironmentNotFound
}

// editAppEnvironmentDomains applies a PATCH to one environment's domain set.
func (rt *Router) editAppEnvironmentDomains(w http.ResponseWriter, r *http.Request, name, envValue string, set *[]string, add, remove []string) {
	editor, ok := rt.apps.(serviceEnvironmentDomainStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "per-environment domains are not supported by this store")
		return
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: edit environment domains: load app failed", err, slog.String("name", name))
		return
	}
	env, err := rt.resolveDomainEnvironment(r.Context(), envValue, svc.ProjectID)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		writeError(w, http.StatusNotFound, "environment not found: "+envValue)
		return
	}
	if err != nil {
		rt.internalError(w, "api: edit environment domains: resolve environment failed", err, slog.String("name", name))
		return
	}

	var before []string
	next, changed, err := editor.EditServiceEnvironmentDomains(r.Context(), name, env.ID, func(current []string) ([]string, error) {
		before = slices.Clone(current)
		if set != nil {
			return normalizeDomainList(*set), nil
		}
		return applyDomainEdit(current, add, remove)
	})
	var taken *store.ErrDomainTaken
	switch {
	case errors.Is(err, errDomainNotSet):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.As(err, &taken):
		writeError(w, http.StatusConflict, taken.Error())
		return
	case err != nil:
		rt.internalError(w, "api: edit environment domains failed", err, slog.String("name", name), slog.String("environment", env.ID))
		return
	}
	if changed {
		if ev, ok := domainChangeEvent(name, before, next); ok {
			ev.Title = "Config changed: domains (" + env.Name + ")"
			rt.recordAppEvent(r, ev)
		}
		rt.nudgeReconciler()
	}
	writeJSON(w, http.StatusOK, editDomainsResponse{App: name, Domains: next, Changed: changed, EnvironmentID: env.ID})
}

// cloneDomainSets computes a clone's default domains (the source's routed set
// rewritten) and the rewritten sets of every other environment, keyed by
// environment ID.
func (rt *Router) cloneDomainSets(ctx context.Context, source store.DesiredService, rewrite store.DomainRewrite) (domains []string, others map[string][]string, err error) {
	var sets map[string][]string
	if lister, ok := rt.apps.(serviceEnvironmentDomainStore); ok {
		if sets, err = lister.ListServiceEnvironmentDomains(ctx, source.Name); err != nil {
			return nil, nil, fmt.Errorf("list environment domains: %w", err)
		}
	}
	domains = rewrite.ApplyAll(store.EffectiveServiceDomains(source.Domains, source.EnvironmentID, sets))
	others = map[string][]string{}
	for envID, hosts := range sets {
		if envID != source.EnvironmentID && len(hosts) > 0 {
			others[envID] = rewrite.ApplyAll(hosts)
		}
	}
	return domains, others, nil
}

// copyEnvironmentDomainSets writes sets onto app, one environment at a time.
func (rt *Router) copyEnvironmentDomainSets(ctx context.Context, app string, sets map[string][]string) error {
	if len(sets) == 0 {
		return nil
	}
	editor, ok := rt.apps.(serviceEnvironmentDomainStore)
	if !ok {
		return nil
	}
	for envID, hosts := range sets {
		if _, _, err := editor.EditServiceEnvironmentDomains(ctx, app, envID, func([]string) ([]string, error) { return hosts, nil }); err != nil {
			return fmt.Errorf("copy environment %q domains: %w", envID, err)
		}
	}
	return nil
}

// handleGetAppEnvironmentDomains handles
// GET /api/v1/apps/{name}/environment-domains.
func (rt *Router) handleGetAppEnvironmentDomains(w http.ResponseWriter, r *http.Request) {
	lister, ok := rt.apps.(serviceEnvironmentDomainStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "per-environment domains are not supported by this store")
		return
	}
	name := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get environment domains: load app failed", err, slog.String("name", name))
		return
	}
	sets, err := lister.ListServiceEnvironmentDomains(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get environment domains failed", err, slog.String("name", name))
		return
	}
	envs, err := rt.environments.ListAllEnvironments(r.Context())
	if err != nil {
		rt.internalError(w, "api: get environment domains: list environments failed", err, slog.String("name", name))
		return
	}
	out := appEnvironmentDomainsResource{
		App:                 name,
		ActiveEnvironmentID: svc.EnvironmentID,
		DefaultDomains:      nonNilStrings(svc.Domains),
		RoutedDomains:       nonNilStrings(store.EffectiveServiceDomains(svc.Domains, svc.EnvironmentID, sets)),
		Environments:        []environmentDomainSetResource{},
	}
	for _, e := range envs {
		if e.Kind == store.EnvironmentKindPreview || (e.Scope != store.EnvironmentScopeGlobal && e.ProjectID != svc.ProjectID) {
			continue
		}
		out.Environments = append(out.Environments, environmentDomainSetResource{
			EnvironmentID: e.ID, Name: e.Name, Kind: e.Kind,
			Active:  e.ID == svc.EnvironmentID,
			Domains: nonNilStrings(sets[e.ID]),
		})
	}
	writeJSON(w, http.StatusOK, out)
}
