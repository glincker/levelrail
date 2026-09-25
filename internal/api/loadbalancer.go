package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/loadbalancer/lbexport"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxLBImportBytes = 1 << 20

// LoadBalancerStore is the persistence surface for per-app load balancer
// config. *store.DB satisfies this.
type LoadBalancerStore interface {
	GetServiceLoadBalancer(ctx context.Context, service string) (string, bool, error)
	SetServiceLoadBalancer(ctx context.Context, service, configJSON string) error
	DeleteServiceLoadBalancer(ctx context.Context, service string) error
	ListLoadBalancerRows(ctx context.Context) ([]store.LoadBalancerRow, error)
}

type lbDeps struct {
	store    LoadBalancerStore
	registry *loadbalancer.Registry
	stats    loadbalancer.StatsSource
	prober   loadbalancer.Prober
}

// WithLoadBalancers enables the /api/v1/apps/{name}/loadbalancer routes.
// registry and stats feed the live status route and may be nil.
func WithLoadBalancers(s LoadBalancerStore, registry *loadbalancer.Registry, stats loadbalancer.StatsSource) Option {
	return func(rt *Router) {
		rt.lb = lbDeps{store: s, registry: registry, stats: stats, prober: loadbalancer.HTTPProber{}}
	}
}

// SetLoadBalancers enables the load balancer routes after construction, for
// wiring that needs the router before the reconciler exists.
func (rt *Router) SetLoadBalancers(s LoadBalancerStore, registry *loadbalancer.Registry, stats loadbalancer.StatsSource) {
	WithLoadBalancers(s, registry, stats)(rt)
}

type loadBalancerResource struct {
	AppName    string               `json:"app_name"`
	Configured bool                 `json:"configured"`
	Config     *loadbalancer.Config `json:"config,omitempty"`
	Algorithms []string             `json:"algorithms"`
	Formats    []string             `json:"export_formats"`
}

func lbResource(app string, cfg *loadbalancer.Config) loadBalancerResource {
	return loadBalancerResource{AppName: app, Configured: cfg != nil, Config: cfg, Algorithms: loadbalancer.Algorithms(), Formats: lbexport.ExportFormats()}
}

func (rt *Router) lbConfigured(w http.ResponseWriter) bool {
	if rt.lb.store == nil {
		writeError(w, http.StatusNotImplemented, "load balancer is not configured on this control plane")
		return false
	}
	return true
}

func (rt *Router) loadLB(w http.ResponseWriter, r *http.Request, name string) (*loadbalancer.Config, bool) {
	raw, found, err := rt.lb.store.GetServiceLoadBalancer(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get load balancer failed", err)
		return nil, false
	}
	if !found {
		return nil, true
	}
	var cfg loadbalancer.Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		rt.internalError(w, "api: decode stored load balancer failed", err)
		return nil, false
	}
	return &cfg, true
}

func (rt *Router) requireLBApp(w http.ResponseWriter, r *http.Request, name string) (*store.DesiredService, bool) {
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return nil, false
	}
	if err != nil {
		rt.internalError(w, "api: load app for load balancer failed", err)
		return nil, false
	}
	return svc, true
}

func (rt *Router) saveLB(w http.ResponseWriter, r *http.Request, name string, cfg loadbalancer.Config) {
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		rt.internalError(w, "api: encode load balancer failed", err)
		return
	}
	if err := rt.lb.store.SetServiceLoadBalancer(r.Context(), name, string(raw)); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set load balancer failed", err)
		return
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, lbResource(name, &cfg))
}

// handleGetLoadBalancer handles GET /api/v1/apps/{name}/loadbalancer.
func (rt *Router) handleGetLoadBalancer(w http.ResponseWriter, r *http.Request) {
	if !rt.lbConfigured(w) {
		return
	}
	name := r.PathValue("name")
	if _, ok := rt.requireLBApp(w, r, name); !ok {
		return
	}
	cfg, ok := rt.loadLB(w, r, name)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, lbResource(name, cfg))
}

// handleSetLoadBalancer handles PUT /api/v1/apps/{name}/loadbalancer.
func (rt *Router) handleSetLoadBalancer(w http.ResponseWriter, r *http.Request) {
	if !rt.lbConfigured(w) {
		return
	}
	name := r.PathValue("name")
	var cfg loadbalancer.Config
	dec := json.NewDecoder(io.LimitReader(r.Body, maxLBImportBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	rt.saveLB(w, r, name, cfg)
}

// handleDeleteLoadBalancer handles DELETE /api/v1/apps/{name}/loadbalancer.
func (rt *Router) handleDeleteLoadBalancer(w http.ResponseWriter, r *http.Request) {
	if !rt.lbConfigured(w) {
		return
	}
	name := r.PathValue("name")
	if _, ok := rt.requireLBApp(w, r, name); !ok {
		return
	}
	if err := rt.lb.store.DeleteServiceLoadBalancer(r.Context(), name); err != nil {
		rt.internalError(w, "api: delete load balancer failed", err)
		return
	}
	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}

// handleImportLoadBalancer handles POST /api/v1/apps/{name}/loadbalancer/import:
// the body is an app.yaml document, its loadbalancer: block for the service
// named by ?service= (default the app name) becomes this app's config.
func (rt *Router) handleImportLoadBalancer(w http.ResponseWriter, r *http.Request) {
	if !rt.lbConfigured(w) {
		return
	}
	name := r.PathValue("name")
	if _, ok := rt.requireLBApp(w, r, name); !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxLBImportBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read request body")
		return
	}
	parsed, err := spec.Parse(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	service := r.URL.Query().Get("service")
	if service == "" {
		service = name
	}
	cfg, err := parsed.LoadBalancerFor(service)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if cfg == nil {
		writeError(w, http.StatusBadRequest, "service "+service+" has no loadbalancer block")
		return
	}
	rt.saveLB(w, r, name, *cfg)
}

// handleLoadBalancerStatus handles GET /api/v1/apps/{name}/loadbalancer/status.
func (rt *Router) handleLoadBalancerStatus(w http.ResponseWriter, r *http.Request) {
	if !rt.lbConfigured(w) {
		return
	}
	name := r.PathValue("name")
	if _, ok := rt.requireLBApp(w, r, name); !ok {
		return
	}
	cfg, ok := rt.loadLB(w, r, name)
	if !ok {
		return
	}
	if cfg == nil {
		writeError(w, http.StatusNotFound, "load balancer not configured")
		return
	}
	var obs loadbalancer.Observation
	if rt.lb.registry != nil {
		obs, _ = rt.lb.registry.Get(name)
	}
	if obs.Service == "" {
		writeJSON(w, http.StatusOK, loadbalancer.Status{Service: name, Algorithm: cfg.EffectiveAlgorithm(), Reason: "Pending", Message: "no reconcile pass has observed this load balancer yet", Upstreams: []loadbalancer.UpstreamStatus{}})
		return
	}
	obs.Config = *cfg
	var stats map[string]loadbalancer.Stats
	if rt.lb.stats != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if s, err := rt.lb.stats.UpstreamStats(ctx); err == nil {
			stats = s
		}
	}
	writeJSON(w, http.StatusOK, loadbalancer.BuildStatus(r.Context(), obs, stats, rt.lb.prober))
}

// handleExportLoadBalancer handles GET /api/v1/apps/{name}/loadbalancer/export.
func (rt *Router) handleExportLoadBalancer(w http.ResponseWriter, r *http.Request) {
	if !rt.lbConfigured(w) {
		return
	}
	name := r.PathValue("name")
	svc, ok := rt.requireLBApp(w, r, name)
	if !ok {
		return
	}
	cfg, ok := rt.loadLB(w, r, name)
	if !ok {
		return
	}
	if cfg == nil {
		writeError(w, http.StatusNotFound, "load balancer not configured")
		return
	}
	in := lbexport.ExportInput{Service: name, Port: svc.Port, Domains: svc.Domains, Config: *cfg}
	if rt.lb.registry != nil {
		if obs, found := rt.lb.registry.Get(name); found {
			for _, u := range obs.Upstreams {
				if u.Running {
					in.Upstreams = append(in.Upstreams, u.Dial)
				}
			}
		}
	}
	art, err := lbexport.Export(r.URL.Query().Get("format"), in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.URL.Query().Get("raw") == "true" {
		w.Header().Set("Content-Type", art.ContentType+"; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(art.Filename, `"`, "")+`"`)
		_, _ = io.WriteString(w, art.Body)
		return
	}
	writeJSON(w, http.StatusOK, art)
}
